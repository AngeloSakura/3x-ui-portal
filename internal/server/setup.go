package server

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AngeloSakura/3x-ui-portal/internal/panel"
	"github.com/AngeloSakura/3x-ui-portal/internal/relay"
	"github.com/AngeloSakura/3x-ui-portal/internal/security"
	"github.com/AngeloSakura/3x-ui-portal/internal/store"
	"github.com/gin-gonic/gin"
)

type setupInput struct {
	Name         string `json:"name"`
	Suffix       string `json:"suffix"`
	Username     string `json:"username"`
	Password     string `json:"password"`
	PanelURL     string `json:"panelUrl"`
	PanelToken   string `json:"panelToken"`
	SupportURL   string `json:"supportUrl"`
	RelayOrigins string `json:"relayOrigins"`
}

func (a *App) checkSetup(c *gin.Context) {
	if !a.limiter.Allow("install-check", 10, time.Minute) {
		fail(c, 429, "连接检查过于频繁，请稍后重试")
		return
	}
	var in struct {
		PanelURL   string `json:"panelUrl"`
		PanelToken string `json:"panelToken"`
	}
	if !decode(c, &in) {
		return
	}
	p, err := panel.New(in.PanelURL, in.PanelToken)
	if err == nil {
		err = p.Check(c.Request.Context())
	}
	if err != nil {
		fail(c, 400, err.Error())
		return
	}
	ok(c, gin.H{"message": "连接成功，客户端查询接口可用"})
}

func (a *App) setup(c *gin.Context) {
	a.setupMu.Lock()
	defer a.setupMu.Unlock()
	if site, _ := a.snapshot(); site != nil {
		c.Status(404)
		return
	}
	if !a.limiter.Allow("install-submit", 6, time.Minute) {
		fail(c, 429, "安装尝试过于频繁，请稍后重试")
		return
	}
	var in setupInput
	if !decode(c, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Username = strings.TrimSpace(in.Username)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > 40 {
		fail(c, 400, "站点名称需为 1–40 个字符")
		return
	}
	if !security.ValidSuffix(in.Suffix) {
		fail(c, 400, "登录后缀需为 1–64 位字母、数字、下划线或连字符，不能使用保留名称 sub")
		return
	}
	if !security.ValidUsername(in.Username) {
		fail(c, 400, "账号需为 3–32 位字母、数字、点、下划线或连字符")
		return
	}
	if !supportValid(in.SupportURL) {
		fail(c, 400, "联系方式需为 HTTPS 网址或 mailto 邮箱链接")
		return
	}
	hash, err := security.HashPassword(in.Password)
	if err != nil {
		fail(c, 400, err.Error())
		return
	}
	p, err := panel.New(in.PanelURL, in.PanelToken)
	if err == nil {
		err = p.Check(c.Request.Context())
	}
	if err != nil {
		fail(c, 400, err.Error())
		return
	}
	secret, err := a.vault.Seal(in.PanelToken)
	if err != nil {
		fail(c, 500, "密钥保存准备失败，请重试")
		return
	}
	if _, err = relay.Origins(in.RelayOrigins, in.PanelURL); err != nil {
		fail(c, 400, err.Error())
		return
	}
	site := &store.Site{Name: in.Name, Suffix: in.Suffix, PanelURL: strings.TrimRight(strings.TrimSpace(in.PanelURL), "/"), PanelToken: secret, SupportURL: in.SupportURL, RelayOrigins: strings.TrimSpace(in.RelayOrigins)}
	tx, err := a.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		fail(c, 500, "数据库暂不可用")
		return
	}
	defer tx.Rollback()
	_, err = tx.Exec("INSERT INTO site(id,name,suffix,panel_url,panel_token,support_url,relay_origins,installed_at) VALUES(1,?,?,?,?,?,?,?)", site.Name, site.Suffix, site.PanelURL, site.PanelToken, site.SupportURL, site.RelayOrigins, time.Now().Unix())
	if err == nil {
		_, err = tx.Exec("INSERT INTO users(username,password_hash,role,created_at) VALUES(?,?,'admin',?)", in.Username, hash, time.Now().Unix())
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		fail(c, 409, "安装没有完成或本站已被初始化，请刷新核实")
		return
	}
	a.mu.Lock()
	a.site = site
	a.panel = p
	a.mu.Unlock()
	// 安装状态优先于凭证文件；即使文件仍在，全部安装路由也立即失效。
	ok(c, gin.H{"loginPath": "/" + site.Suffix + "/"})
}
