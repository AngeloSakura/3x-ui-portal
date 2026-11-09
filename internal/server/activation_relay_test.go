package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AngeloSakura/3x-ui-portal/internal/config"
	"github.com/AngeloSakura/3x-ui-portal/internal/panel"
	"github.com/AngeloSakura/3x-ui-portal/internal/security"
	"github.com/AngeloSakura/3x-ui-portal/internal/store"
)

func accountFixture(t *testing.T, panelURL string) (*App, store.User) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	vault, err := security.NewVault(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	secret, err := vault.Seal(strings.Repeat("t", 32))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("INSERT INTO site(id,name,suffix,panel_url,panel_token,installed_at) VALUES(1,'fixture','private-entry-123',?,?,0)", panelURL, secret)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("INSERT INTO users(id,username,password_hash,role,client_id,client_email,created_at) VALUES(1,'member','','user',9,'client-a',0)")
	if err != nil {
		t.Fatal(err)
	}
	app, err := New(config.Config{Origin: "https://portal.test", Secure: true, DataDir: dir, WebDir: dir}, db, vault)
	if err != nil {
		t.Fatal(err)
	}
	u, err := store.ScanUser(db.QueryRow("SELECT " + store.UserColumns + " FROM users WHERE id=1"))
	if err != nil {
		t.Fatal(err)
	}
	return app, u
}

func requestAccount(a *App, method, path, body string, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "https://portal.test"+path, strings.NewReader(body))
	req.Header.Set("Origin", "https://portal.test")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", csrf)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	a.ServeHTTP(w, req)
	return w
}

func TestActivationCannotReadSubscriptionBeforePassword(t *testing.T) {
	a, _ := accountFixture(t, "https://panel.example.com")
	invite := strings.Repeat("a", 43)
	if _, err := a.db.Exec("INSERT INTO activations VALUES(1,?,?)", security.Digest(invite), time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	prefix := "/private-entry-123/api/"
	w := requestAccount(a, "POST", prefix+"activate", `{"token":"`+invite+`"}`, nil, "")
	if w.Code != 200 {
		t.Fatalf("激活失败 %d", w.Code)
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatal("缺少有限会话")
	}
	cookie := cookies[0]
	if cookie.MaxAge != 600 {
		t.Fatal("激活会话应只有 10 分钟")
	}
	w = requestAccount(a, "GET", prefix+"me", "", cookie, "")
	var me struct {
		CSRF string     `json:"csrf"`
		User store.User `json:"user"`
	}
	if json.Unmarshal(w.Body.Bytes(), &me) != nil || !me.User.MustSetPassword {
		t.Fatal("首次会话未标记设置密码")
	}
	for _, path := range []string{"summary", "admin/users"} {
		if r := requestAccount(a, "GET", prefix+path, "", cookie, ""); r.Code != 403 {
			t.Fatalf("未设密码可读取 %s", path)
		}
	}
	if r := requestAccount(a, "POST", prefix+"first-password", `{"password":"fixture-long-password"}`, cookie, ""); r.Code != 403 {
		t.Fatal("设置密码必须校验 CSRF")
	}
	w = requestAccount(a, "POST", prefix+"first-password", `{"password":"fixture-long-password"}`, cookie, me.CSRF)
	if w.Code != 200 {
		t.Fatalf("设置密码失败 %d", w.Code)
	}
	if r := requestAccount(a, "GET", prefix+"me", "", cookie, ""); r.Code != 401 {
		t.Fatal("有限会话未撤销")
	}
	if r := requestAccount(a, "POST", prefix+"activate", `{"token":"`+invite+`"}`, nil, ""); r.Code != 401 {
		t.Fatal("邀请被重复使用")
	}
	if r := requestAccount(a, "POST", prefix+"login", `{"username":"member","password":"fixture-long-password"}`, nil, ""); r.Code != 200 {
		t.Fatalf("新密码登录失败 %d", r.Code)
	}
}

func TestRelayRejectsTamperingDisabledAccountAndOldSubscription(t *testing.T) {
	currentSub := "original-sub"
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("fixture-subscription"))
	}))
	defer source.Close()
	d := func() panel.Detail {
		return panel.Detail{Client: panel.Record{ID: 9, Email: "client-a", SubID: currentSub, Enable: true}, InboundIDs: []int64{1}}
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var obj any
		switch {
		case strings.HasPrefix(r.URL.Path, "/panel/api/clients/get/subId/"):
			obj = []panel.Detail{d()}
		case r.URL.Path == "/panel/api/clients/get":
			obj = d()
		case strings.HasPrefix(r.URL.Path, "/panel/api/clients/traffic/"):
			obj = panel.Traffic{Email: "client-a", Enable: true}
		case strings.HasPrefix(r.URL.Path, "/panel/api/clients/subLinks/"):
			obj = []string{source.URL + "/s"}
		default:
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "obj": obj})
	}))
	defer upstream.Close()
	a, u := accountFixture(t, upstream.URL)
	if _, err := a.db.Exec("UPDATE users SET password_hash='activated-fixture' WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	site, _ := a.snapshot()
	site.RelayOrigins = source.URL
	routes := a.subscriptionRoutes(site, u, d(), []string{source.URL + "/s"})
	parsed, err := url.Parse(routes[0].Primary)
	if err != nil {
		t.Fatal(err)
	}
	path := parsed.Path
	if w := requestAccount(a, "GET", path+"x", "", nil, ""); w.Code != 404 {
		t.Fatal("被篡改的签名应拒绝")
	}
	if w := requestAccount(a, "GET", path, "", nil, ""); w.Code != 200 || w.Body.String() != "fixture-subscription" {
		t.Fatalf("中转失败 %d %s", w.Code, w.Body.String())
	}
	currentSub = "rotated-sub"
	if w := requestAccount(a, "GET", path, "", nil, ""); w.Code != 404 {
		t.Fatal("旧 subId 的中转地址未失效")
	}
	currentSub = "original-sub"
	if _, err := a.db.Exec("UPDATE users SET disabled=1 WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	if w := requestAccount(a, "GET", path, "", nil, ""); w.Code != 404 {
		t.Fatal("停用用户仍能中转")
	}
}
