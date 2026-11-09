package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/AngeloSakura/3x-ui-portal/internal/panel"
	"github.com/AngeloSakura/3x-ui-portal/internal/relay"
	"github.com/AngeloSakura/3x-ui-portal/internal/security"
	"github.com/AngeloSakura/3x-ui-portal/internal/store"
	"github.com/gin-gonic/gin"
)

type relayClaim struct {
	User   int64  `json:"u"`
	Source string `json:"s"`
	State  string `json:"v"`
}
type subscriptionRoute struct {
	Primary string `json:"primary"`
	Backup  string `json:"backup"`
	Reason  string `json:"reason"`
}

func relayState(d panel.Detail) string {
	return security.Digest(fmt.Sprintf("%d:%s", d.Client.ID, d.Client.SubID))
}

func (a *App) subscriptionRoutes(site *store.Site, u store.User, d panel.Detail, links []string) []subscriptionRoute {
	allowed, err := relay.Origins(site.RelayOrigins, site.PanelURL)
	result := []subscriptionRoute{}
	for _, link := range links {
		route := subscriptionRoute{Backup: link}
		origin, e := relay.Origin(link)
		if err != nil || e != nil || !allowed[origin] {
			route.Reason = "本站中转尚未配置此订阅来源，请先使用原站备用或联系站长"
		} else {
			data, _ := json.Marshal(relayClaim{User: u.ID, Source: security.Digest(link), State: relayState(d)})
			payload := base64.RawURLEncoding.EncodeToString(data)
			route.Primary = a.cfg.Origin + "/sub/" + payload + "." + a.vault.Sign(payload)
		}
		result = append(result, route)
	}
	return result
}

func (a *App) subscription(c *gin.Context, token string) {
	// 导入客户端没有浏览器会话，签名链接本身是只用于取订阅的凭证。
	if len(token) > 512 || c.Request.URL.RawQuery != "" {
		c.Status(404)
		return
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 || !a.vault.Verify(parts[0], parts[1]) {
		c.Status(404)
		return
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	var claim relayClaim
	if err != nil || json.Unmarshal(raw, &claim) != nil || claim.User <= 0 {
		c.Status(404)
		return
	}
	if !a.limiter.Allow("relay-global", 300, time.Minute) || !a.limiter.Allow(fmt.Sprintf("relay:%d", claim.User), 12, time.Minute) {
		fail(c, 429, "订阅更新过于频繁，请稍后重试")
		return
	}
	select {
	case a.relaySlots <- struct{}{}:
		defer func() { <-a.relaySlots }()
	default:
		fail(c, 503, "订阅更新繁忙，请稍后重试")
		return
	}
	u, err := store.ScanUser(a.db.QueryRow("SELECT "+store.UserColumns+" FROM users WHERE id=?", claim.User))
	if err != nil || u.Disabled || u.Role != "user" || u.MustSetPassword {
		c.Status(404)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 45*time.Second)
	defer cancel()
	site, p := a.snapshot()
	d, t, err := bound(ctx, p, u)
	if err != nil {
		fail(c, 502, "面板暂不可达，无法核实订阅，请稍后重试")
		return
	}
	if claim.State != relayState(d) {
		c.Status(404)
		return
	}
	if !d.Client.Enable || !t.Enable || (d.Client.Expiry > 0 && d.Client.Expiry <= time.Now().UnixMilli()) || (t.Total > 0 && t.Up+t.Down >= t.Total) {
		fail(c, 403, "服务已停用、到期或流量耗尽，请联系站长")
		return
	}
	shared, err := p.Shared(ctx, d.Client.SubID)
	if err != nil {
		fail(c, 502, "订阅归属暂不可核实")
		return
	}
	if len(shared) != 1 || shared[0].Client.ID != u.ClientID {
		c.Status(404)
		return
	}
	links, err := p.Links(ctx, d.Client.SubID)
	if err != nil {
		fail(c, 502, "原站订阅地址暂不可用")
		return
	}
	target := ""
	for _, link := range links {
		if security.Digest(link) == claim.Source {
			target = link
			break
		}
	}
	if target == "" {
		c.Status(404)
		return
	}
	allowed, err := relay.Origins(site.RelayOrigins, site.PanelURL)
	if err != nil {
		fail(c, 503, "站长尚未完成订阅中转配置")
		return
	}
	result, err := a.relay.Fetch(ctx, target, c.GetHeader("User-Agent"), c.GetHeader("Accept"), allowed)
	if err != nil {
		fail(c, 502, err.Error())
		return
	}
	// 网络往返后重新核实停用状态；失效时不输出已取到的内容。
	var enabled bool
	err = a.db.QueryRow("SELECT disabled=0 AND password_hash<>'' FROM users WHERE id=?", u.ID).Scan(&enabled)
	if err != nil || !enabled {
		c.Status(404)
		return
	}
	for key, values := range result.Headers {
		if len(values) > 0 && values[0] != "" {
			c.Header(key, values[0])
		}
	}
	c.Header("Content-Disposition", "attachment; filename=subscription.txt")
	c.Data(200, result.ContentType, result.Body)
}
