package server

import (
	"context"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AngeloSakura/3x-ui-portal/internal/certmgr"
	"github.com/AngeloSakura/3x-ui-portal/internal/panel"
	"github.com/AngeloSakura/3x-ui-portal/internal/relay"
	"github.com/AngeloSakura/3x-ui-portal/internal/security"
	"github.com/AngeloSakura/3x-ui-portal/internal/store"
	"github.com/gin-gonic/gin"
)

func (a *App) admin(c *gin.Context, actor store.User, route string) {
	switch {
	case route == "certificate" && c.Request.Method == "GET":
		if a.certificates == nil {
			ok(c, map[string]any{"managed": false})
			return
		}
		ok(c, map[string]any{"managed": true, "status": a.certificates.Status()})
	case route == "certificate" && c.Request.Method == "POST":
		if a.certificates == nil {
			fail(c, 409, "当前由外部反向代理管理证书")
			return
		}
		var s certmgr.Settings
		if !decode(c, &s) {
			return
		}
		if err := a.certificates.Configure(s); err != nil {
			fail(c, 400, err.Error())
			return
		}
		a.audit(actor.ID, "tls-settings", actor.ID, "success")
		ok(c, map[string]string{"message": "证书配置已保存"})
	case route == "certificate/issue" && c.Request.Method == "POST":
		if a.certificates == nil {
			fail(c, 409, "当前由外部反向代理管理证书")
			return
		}
		if err := a.certificates.Start(context.Background()); err != nil {
			fail(c, 409, err.Error())
			return
		}
		a.audit(actor.ID, "tls-issue", actor.ID, "started")
		ok(c, map[string]string{"message": "申请已开始，请稍后查看结果"})
	case route == "clients" && c.Request.Method == "GET":
		search := c.Query("search")
		if len(search) > 128 {
			fail(c, 400, "搜索内容过长")
			return
		}
		_, p := a.snapshot()
		rows, err := p.Search(c.Request.Context(), search)
		if err != nil {
			fail(c, 502, err.Error())
			return
		}
		ok(c, rows)
	case route == "users" && c.Request.Method == "GET":
		a.users(c)
	case route == "users" && c.Request.Method == "POST":
		a.createUser(c, actor)
	case route == "invitations" && c.Request.Method == "POST":
		a.createUser(c, actor)
	case route == "user-state" && c.Request.Method == "POST":
		a.userState(c, actor)
	case route == "invite" && c.Request.Method == "POST":
		a.invite(c, actor)
	case route == "settings" && c.Request.Method == "GET":
		site, _ := a.snapshot()
		ok(c, site)
	case route == "settings" && c.Request.Method == "POST":
		a.settings(c, actor)
	default:
		c.Status(404)
	}
}

func (a *App) users(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	if page < 1 {
		page = 1
	}
	if page > 10000 {
		fail(c, 400, "页码超出范围")
		return
	}
	var total int
	if err := a.db.QueryRow("SELECT COUNT(*) FROM users WHERE role='user'").Scan(&total); err != nil {
		fail(c, 500, "用户列表读取失败")
		return
	}
	rows, err := a.db.Query("SELECT "+store.UserColumns+" FROM users WHERE role='user' ORDER BY id DESC LIMIT 20 OFFSET ?", (page-1)*20)
	if err != nil {
		fail(c, 500, "用户列表读取失败")
		return
	}
	defer rows.Close()
	items := []store.User{}
	for rows.Next() {
		u, e := store.ScanUser(rows)
		if e != nil {
			fail(c, 500, "用户数据读取失败")
			return
		}
		items = append(items, u)
	}
	if rows.Err() != nil {
		fail(c, 500, "用户列表读取失败")
		return
	}
	ok(c, gin.H{"items": items, "total": total})
}

func (a *App) createUser(c *gin.Context, actor store.User) {
	if !a.limiter.Allow("create-user", 20, time.Minute) {
		fail(c, 429, "创建过于频繁，请稍后重试")
		return
	}
	var in struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		ClientID int64  `json:"clientId"`
	}
	if !decode(c, &in) {
		return
	}
	in.Username = strings.TrimSpace(in.Username)
	// 新邀请自动沿用面板名称；保留旧 API 的自定义账号以兼容已有客户端。
	if in.Username == "" {
		in.Username = in.Email
	}
	if !security.ValidLoginName(in.Username) || in.ClientID <= 0 {
		fail(c, 400, "请选择客户端；名称需为 1–128 个 UTF-8 字节，不含控制字符、斜杠或首尾空白")
		return
	}
	_, p := a.snapshot()
	detail, err := p.Detail(c.Request.Context(), in.Email)
	if err != nil {
		fail(c, 502, err.Error())
		return
	}
	if detail.Client.ID != in.ClientID {
		fail(c, 409, "面板客户端已变化，请重新选择")
		return
	}
	token, err := security.Random(32)
	if err != nil {
		fail(c, 500, "无法生成激活链接")
		return
	}
	tx, err := a.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		fail(c, 500, "用户创建失败")
		return
	}
	defer tx.Rollback()
	res, err := tx.Exec("INSERT INTO users(username,password_hash,role,client_id,client_email,created_at) VALUES(?,'','user',?,?,?)", in.Username, in.ClientID, in.Email, time.Now().Unix())
	if err != nil {
		fail(c, 409, "账号或客户端已有绑定；未激活用户请在列表重发邀请，账号重名请联系站长处理")
		return
	}
	id, err := res.LastInsertId()
	expires := time.Now().Add(24 * time.Hour).Unix()
	if err == nil {
		_, err = tx.Exec("INSERT INTO activations(user_id,token_hash,expires_at) VALUES(?,?,?)", id, security.Digest(token), expires)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		fail(c, 500, "用户与激活链接保存失败")
		return
	}
	a.audit(actor.ID, "user-create", id, "success")
	a.inviteResult(c, token, expires, in.Username)
}

func (a *App) userState(c *gin.Context, actor store.User) {
	var in struct {
		ID       int64 `json:"id"`
		Disabled bool  `json:"disabled"`
	}
	if !decode(c, &in) {
		return
	}
	tx, err := a.db.Begin()
	if err != nil {
		fail(c, 500, "账号更新失败")
		return
	}
	defer tx.Rollback()
	res, err := tx.Exec("UPDATE users SET disabled=? WHERE id=? AND role='user'", in.Disabled, in.ID)
	if err != nil {
		fail(c, 500, "账号更新失败")
		return
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		fail(c, 404, "用户不存在")
		return
	}
	_, err = tx.Exec("DELETE FROM sessions WHERE user_id=?", in.ID)
	if err == nil {
		_, err = tx.Exec("DELETE FROM activations WHERE user_id=?", in.ID)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		fail(c, 500, "账号更新失败")
		return
	}
	a.audit(actor.ID, "user-state", in.ID, "success")
	ok(c, gin.H{"message": "门户账号状态已更新，不改变面板客户端状态"})
}

func (a *App) settings(c *gin.Context, actor store.User) {
	a.writes.Lock()
	defer a.writes.Unlock()
	var in struct {
		Name          string `json:"name"`
		SupportURL    string `json:"supportUrl"`
		PanelToken    string `json:"panelToken"`
		AllowRotation bool   `json:"allowRotation"`
		RelayOrigins  string `json:"relayOrigins"`
	}
	if !decode(c, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > 40 || !supportValid(in.SupportURL) {
		fail(c, 400, "请检查站点名称与联系方式")
		return
	}
	current, _ := a.snapshot()
	next := *current
	next.Name = in.Name
	next.SupportURL = in.SupportURL
	next.AllowRotation = in.AllowRotation
	if _, err := relay.Origins(in.RelayOrigins, next.PanelURL); err != nil {
		fail(c, 400, err.Error())
		return
	}
	next.RelayOrigins = strings.TrimSpace(in.RelayOrigins)
	if in.PanelToken != "" {
		p, err := panel.New(next.PanelURL, in.PanelToken)
		if err == nil {
			err = p.Check(c.Request.Context())
		}
		if err != nil {
			fail(c, 400, err.Error())
			return
		}
		secret, err := a.vault.Seal(in.PanelToken)
		if err != nil {
			fail(c, 500, "密钥加密失败")
			return
		}
		next.PanelToken = secret
	}
	// 面板地址固定，避免将现有绑定静默指向另一台面板的同名客户端。
	_, err := a.db.Exec("UPDATE site SET name=?,support_url=?,panel_token=?,allow_rotation=?,relay_origins=? WHERE id=1", next.Name, next.SupportURL, next.PanelToken, next.AllowRotation, next.RelayOrigins)
	if err != nil {
		fail(c, 500, "设置保存失败")
		return
	}
	if err = a.configure(&next); err != nil {
		fail(c, 500, "设置读取失败，请重启服务核实")
		return
	}
	a.audit(actor.ID, "settings", actor.ID, "success")
	ok(c, gin.H{"message": "设置已保存"})
}
