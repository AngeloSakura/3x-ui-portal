package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/AngeloSakura/3x-ui-portal/internal/panel"
	"github.com/AngeloSakura/3x-ui-portal/internal/security"
	"github.com/AngeloSakura/3x-ui-portal/internal/store"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

func bound(ctx context.Context, p *panel.Client, u store.User) (panel.Detail, panel.Traffic, error) {
	d, err := p.Detail(ctx, u.ClientEmail)
	if err != nil {
		return d, panel.Traffic{}, err
	}
	if d.Client.ID != u.ClientID {
		return d, panel.Traffic{}, errors.New("客户端绑定已变化，请联系站长重新核实")
	}
	t, err := p.Traffic(ctx, u.ClientEmail)
	return d, t, err
}

func usable(d panel.Detail, t panel.Traffic) bool {
	r := d.Client
	now := time.Now().UnixMilli()
	return r.Enable && t.Enable && r.Expiry == t.Expiry && r.Total == t.Total && r.Expiry >= 0 && (r.Expiry == 0 || r.Expiry > now) && (r.Total == 0 || t.Up+t.Down < r.Total)
}

func fingerprint(d panel.Detail) string {
	d.Client.SubID = ""
	d.InboundIDs = slices.Clone(d.InboundIDs)
	slices.Sort(d.InboundIDs)
	b, _ := json.Marshal(d)
	return security.Digest(string(b))
}

func (a *App) summary(c *gin.Context, u store.User) {
	site, p := a.snapshot()
	d, t, err := bound(c.Request.Context(), p, u)
	if err != nil {
		fail(c, 502, err.Error())
		return
	}
	status := "active"
	switch {
	case d.Client.Expiry > 0 && d.Client.Expiry <= time.Now().UnixMilli():
		status = "expired"
	case t.Total > 0 && t.Up+t.Down >= t.Total:
		status = "depleted"
	case !d.Client.Enable || !t.Enable:
		status = "disabled"
	case d.Client.Expiry < 0:
		status = "pending"
	case d.Client.Expiry == 0:
		status = "unlimited"
	}
	links := []string{}
	linkError := ""
	unique := false
	shared, e := p.Shared(c.Request.Context(), d.Client.SubID)
	if e != nil {
		linkError = "订阅信息暂不可用，请稍后重试"
	} else if len(shared) != 1 || shared[0].Client.ID != u.ClientID {
		linkError = "此订阅关联多个客户端，请联系站长处理"
	} else {
		unique = true
		links, e = p.Links(c.Request.Context(), d.Client.SubID)
		if e != nil {
			links = []string{}
			linkError = e.Error()
		}
	}
	var rotationState string
	err = a.db.QueryRow("SELECT status FROM rotations WHERE user_id=?", u.ID).Scan(&rotationState)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		fail(c, 500, "操作状态读取失败")
		return
	}
	reason := ""
	switch {
	case rotationState == "pending" || rotationState == "review":
		reason = "上次重置仍需站长核实，暂不可再次提交"
	case !site.AllowRotation:
		reason = "站长尚未开放自助重置"
	case !unique:
		reason = "共享或异常订阅需联系站长处理"
	case !usable(d, t):
		reason = "当前账号状态不适合自助重置，请联系站长"
	}
	ok(c, gin.H{"clientName": d.Client.Email, "status": status, "expiryTime": d.Client.Expiry, "upload": t.Up, "download": t.Down, "total": t.Total, "serverTime": time.Now().UnixMilli(), "links": links, "routes": a.subscriptionRoutes(site, u, d, links), "linkError": linkError, "canRotate": reason == "", "rotateReason": reason, "supportUrl": site.SupportURL})
}

func (a *App) rotate(c *gin.Context, u store.User) {
	if !a.limiter.Allow("rotate:"+u.Username, 2, 10*time.Minute) {
		fail(c, 429, "请勿频繁重置，请 10 分钟后重试")
		return
	}
	var in struct {
		Password string `json:"password"`
	}
	if !decode(c, &in) {
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(in.Password)) != nil {
		fail(c, 400, "当前密码不正确")
		return
	}
	a.writes.Lock()
	defer a.writes.Unlock()
	site, p := a.snapshot()
	if !site.AllowRotation {
		fail(c, 403, "站长尚未开放自助重置")
		return
	}
	var state string
	var previous int64
	err := a.db.QueryRow("SELECT status,created_at FROM rotations WHERE user_id=?", u.ID).Scan(&state, &previous)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		fail(c, 500, "操作状态读取失败")
		return
	}
	if state == "pending" || state == "review" {
		fail(c, 409, "上次操作需站长核实，未再次写入面板")
		return
	}
	if previous > time.Now().Add(-10*time.Minute).Unix() {
		fail(c, 429, "上次重置距今不足 10 分钟")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 45*time.Second)
	defer cancel()
	d, t, err := bound(ctx, p, u)
	if err != nil {
		fail(c, 502, err.Error())
		return
	}
	if !usable(d, t) {
		fail(c, 409, "客户端与流量状态不一致或账号不可用，请联系站长")
		return
	}
	shared, err := p.Shared(ctx, d.Client.SubID)
	if err != nil {
		fail(c, 502, err.Error())
		return
	}
	if len(shared) != 1 || shared[0].Client.ID != u.ClientID {
		fail(c, 409, "共享订阅不能自助重置")
		return
	}
	if err = p.CheckRotation(ctx); err == nil {
		err = p.LocalInbounds(ctx, d.InboundIDs)
	}
	if err != nil {
		fail(c, 409, err.Error())
		return
	}
	// 网络预检后再次读取，缩短前置检查与写入之间的状态漂移窗口。
	fresh, freshTraffic, err := bound(ctx, p, u)
	if err != nil || d.Client.SubID != fresh.Client.SubID || fingerprint(d) != fingerprint(fresh) || !usable(fresh, freshTraffic) {
		fail(c, 409, "账号状态发生变化，请稍后重试")
		return
	}
	target, err := security.Random(24)
	if err != nil {
		fail(c, 500, "无法生成订阅标识")
		return
	}
	sealed, err := a.vault.Seal(target)
	if err != nil {
		fail(c, 500, "无法保存操作记录")
		return
	}
	_, err = a.db.Exec("INSERT INTO rotations(user_id,target_secret,protected_hash,status,created_at) VALUES(?,?,?,'pending',?) ON CONFLICT(user_id) DO UPDATE SET target_secret=excluded.target_secret,protected_hash=excluded.protected_hash,status='pending',created_at=excluded.created_at", u.ID, sealed, fingerprint(d), time.Now().Unix())
	if err != nil {
		fail(c, 500, "无法保存操作记录，未修改面板")
		return
	}
	writeErr := p.Rotate(ctx, u.ClientEmail, target)
	// 使用独立短时上下文核实超时/浏览器断开后的结果；错误时绝不再次写入。
	verifyCtx, stop := context.WithTimeout(context.Background(), 15*time.Second)
	defer stop()
	after, afterTraffic, readErr := bound(verifyCtx, p, u)
	confirmed := writeErr == nil && readErr == nil && after.Client.SubID == target && fingerprint(after) == fingerprint(d) && usable(after, afterTraffic)
	result := "review"
	if confirmed {
		result = "confirmed"
	}
	_, err = a.db.Exec("UPDATE rotations SET status=? WHERE user_id=?", result, u.ID)
	a.audit(u.ID, "subscription-rotate", u.ID, result)
	if !confirmed || err != nil {
		fail(c, 409, "操作结果需要站长核实，请勿重复重置；重新刷新可查看当前订阅")
		return
	}
	ok(c, gin.H{"message": "新订阅标识已保存并读回确认，请复制最新地址更新客户端"})
}
