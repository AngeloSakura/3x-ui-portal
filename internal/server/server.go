package server

import (
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/AngeloSakura/3x-ui-portal/internal/certmgr"
	"github.com/AngeloSakura/3x-ui-portal/internal/config"
	"github.com/AngeloSakura/3x-ui-portal/internal/panel"
	"github.com/AngeloSakura/3x-ui-portal/internal/relay"
	"github.com/AngeloSakura/3x-ui-portal/internal/security"
	"github.com/AngeloSakura/3x-ui-portal/internal/store"
	"github.com/gin-gonic/gin"
)

type App struct {
	certificates *certmgr.Manager
	cfg          config.Config
	db           *sql.DB
	vault        *security.Vault
	mu           sync.RWMutex
	site         *store.Site
	panel        *panel.Client
	relay        *relay.Client
	relaySlots   chan struct{}
	setupToken   string
	setupMu      sync.Mutex
	writes       sync.Mutex
	limiter      *security.Limiter
	router       *gin.Engine
}

func New(cfg config.Config, db *sql.DB, vault *security.Vault) (*App, error) {
	a := &App{cfg: cfg, db: db, vault: vault, limiter: security.NewLimiter(), relay: relay.New(), relaySlots: make(chan struct{}, 8)}
	site, err := store.LoadSite(db)
	if err != nil {
		return nil, err
	}
	if site != nil {
		if err = a.configure(site); err != nil {
			return nil, err
		}
	} else {
		token, e := os.ReadFile(filepath.Join(cfg.DataDir, "setup.token"))
		if e != nil || len(token) < 32 {
			return nil, errors.New("缺少安装凭证，请执行 init")
		}
		a.setupToken = strings.TrimSpace(string(token))
	}
	if _, err = os.Stat(filepath.Join(cfg.WebDir, "index.html")); err != nil {
		return nil, errors.New("找不到前端产物，请先按部署文档准备 frontend/dist")
	}
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.Use(func(c *gin.Context) {
		defer func() {
			if recover() != nil {
				fail(c, 500, "服务暂时不可用，请重试")
			}
		}()
		c.Next()
	}, a.headers)
	r.NoRoute(a.dispatch)
	a.router = r
	return a, nil
}

func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) { a.router.ServeHTTP(w, r) }
func (a *App) SetCertificates(m *certmgr.Manager)               { a.certificates = m }
func (a *App) snapshot() (*store.Site, *panel.Client) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.site, a.panel
}
func (a *App) configure(s *store.Site) error {
	token, err := a.vault.Open(s.PanelToken)
	if err != nil {
		return errors.New("面板密钥解密失败，请检查主密钥")
	}
	p, err := panel.New(s.PanelURL, token)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.site = s
	a.panel = p
	a.mu.Unlock()
	return nil
}

func (a *App) headers(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("X-Frame-Options", "DENY")
	c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	c.Header("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
	if a.cfg.Secure {
		c.Header("Strict-Transport-Security", "max-age=31536000")
	}
	u, _ := url.Parse(a.cfg.Origin)
	if !strings.EqualFold(c.Request.Host, u.Host) {
		c.AbortWithStatus(404)
		return
	}
	if c.Request.Method != "GET" && c.Request.Method != "HEAD" {
		if c.GetHeader("Origin") != a.cfg.Origin {
			fail(c, 403, "请求来源无效，请从本站页面重试")
			return
		}
		if !strings.HasPrefix(c.GetHeader("Content-Type"), "application/json") {
			fail(c, 415, "请求必须使用 JSON")
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	}
	c.Next()
}

func (a *App) dispatch(c *gin.Context) {
	site, _ := a.snapshot()
	path := c.Request.URL.Path
	if strings.Contains(c.Request.URL.EscapedPath(), "%") {
		c.Status(404)
		return
	}
	if site == nil {
		prefix := "/install/" + a.setupToken + "/"
		if !strings.HasPrefix(path, prefix) {
			c.Status(404)
			return
		}
		rel := strings.TrimPrefix(path, prefix)
		switch {
		case rel == "api/setup" && c.Request.Method == "POST":
			a.setup(c)
		case rel == "api/check" && c.Request.Method == "POST":
			a.checkSetup(c)
		case rel == "api/bootstrap" && c.Request.Method == "GET":
			ok(c, gin.H{"mode": "setup", "name": "Portal"})
		default:
			a.asset(c, rel)
		}
		return
	}
	prefix := "/" + site.Suffix + "/"
	if strings.HasPrefix(path, "/sub/") && c.Request.Method == "GET" {
		a.subscription(c, strings.TrimPrefix(path, "/sub/"))
		return
	}
	if path == strings.TrimSuffix(prefix, "/") && c.Request.Method == "GET" {
		c.Redirect(308, prefix)
		return
	}
	if !strings.HasPrefix(path, prefix) {
		c.Status(404)
		return
	}
	rel := strings.TrimPrefix(path, prefix)
	if !strings.HasPrefix(rel, "api/") {
		a.asset(c, rel)
		return
	}
	if rel == "api/bootstrap" && c.Request.Method == "GET" {
		ok(c, gin.H{"mode": "login", "name": site.Name})
		return
	}
	if rel == "api/login" && c.Request.Method == "POST" {
		a.login(c)
		return
	}
	if rel == "api/activate" && c.Request.Method == "POST" {
		a.activate(c)
		return
	}
	u, csrf, valid := a.session(c)
	if !valid {
		return
	}
	if c.Request.Method != "GET" && subtle.ConstantTimeCompare([]byte(c.GetHeader("X-CSRF-Token")), []byte(csrf)) != 1 {
		fail(c, 403, "会话校验失败，请刷新页面")
		return
	}
	if !a.limiter.Allow("user:"+u.Username, 120, time.Minute) {
		fail(c, 429, "操作较频繁，请稍后再试")
		return
	}
	if u.MustSetPassword && rel != "api/me" && rel != "api/logout" && rel != "api/first-password" {
		fail(c, 403, "请先设置本站登录密码")
		return
	}
	switch {
	case rel == "api/first-password" && c.Request.Method == "POST":
		a.firstPassword(c, u)
	case rel == "api/me" && c.Request.Method == "GET":
		ok(c, gin.H{"user": u, "csrf": csrf, "name": site.Name, "supportUrl": site.SupportURL})
	case rel == "api/logout" && c.Request.Method == "POST":
		a.logout(c)
	case rel == "api/password" && c.Request.Method == "POST":
		a.password(c, u)
	case rel == "api/summary" && c.Request.Method == "GET" && u.Role == "user":
		a.summary(c, u)
	case rel == "api/rotate" && c.Request.Method == "POST" && u.Role == "user":
		a.rotate(c, u)
	case strings.HasPrefix(rel, "api/admin/") && u.Role == "admin":
		a.admin(c, u, strings.TrimPrefix(rel, "api/admin/"))
	default:
		c.Status(404)
	}
}

func (a *App) asset(c *gin.Context, rel string) {
	if c.Request.Method != "GET" && c.Request.Method != "HEAD" {
		c.Status(404)
		return
	}
	if rel == "" {
		c.Status(200)
		c.File(filepath.Join(a.cfg.WebDir, "index.html"))
		return
	}
	if !strings.HasPrefix(rel, "assets/") || strings.Contains(rel, "..") || strings.Contains(rel, "\\") {
		c.Status(404)
		return
	}
	file := filepath.Join(a.cfg.WebDir, filepath.FromSlash(rel))
	stat, err := os.Stat(file)
	if err != nil || stat.IsDir() {
		c.Status(404)
		return
	}
	c.Status(200)
	c.File(file)
}

func decode(c *gin.Context, v any) bool {
	d := json.NewDecoder(c.Request.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		fail(c, 400, "输入格式不正确，请检查字段")
		return false
	}
	if d.Decode(new(any)) != io.EOF {
		fail(c, 400, "请求包含多余内容")
		return false
	}
	return true
}
func ok(c *gin.Context, v any)                    { c.JSON(200, v) }
func fail(c *gin.Context, status int, msg string) { c.AbortWithStatusJSON(status, gin.H{"error": msg}) }
func supportValid(s string) bool {
	if s == "" {
		return true
	}
	u, e := url.Parse(s)
	return e == nil && u.User == nil && ((u.Scheme == "https" && u.Host != "") || (u.Scheme == "mailto" && u.Opaque != ""))
}
func (a *App) audit(actor int64, action string, target int64, result string) {
	_, _ = a.db.Exec("INSERT INTO audits(actor_id,action,target_id,result,created_at) VALUES(?,?,?,?,?)", actor, action, target, result, time.Now().Unix())
}
