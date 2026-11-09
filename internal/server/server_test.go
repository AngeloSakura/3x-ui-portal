package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AngeloSakura/3x-ui-portal/internal/config"
	"github.com/AngeloSakura/3x-ui-portal/internal/security"
	"github.com/AngeloSakura/3x-ui-portal/internal/store"
)

func TestSetupClosesBootstrapAndProtectsSessions(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("portal-shell"), 0600); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("x", 43)
	if err := os.WriteFile(filepath.Join(dir, "setup.token"), []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	v, err := security.NewVault(bytes.Repeat([]byte{3}, 32))
	if err != nil {
		t.Fatal(err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/panel/api/clients/list/paged" {
			t.Errorf("安装只应查询客户端: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"obj":{"items":[],"total":0}}`))
	}))
	defer upstream.Close()
	cfg := config.Config{Origin: "https://portal.test", Secure: true, DataDir: dir, WebDir: dir}
	a, err := New(cfg, db, v)
	if err != nil {
		t.Fatal(err)
	}
	send := func(method, path, body, origin string, cookie *http.Cookie) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "https://portal.test"+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		return w
	}
	installPath := "/install/" + token + "/"
	input := setupInput{Name: "测试站点", Suffix: "private-entry-92847", Username: "owner", Password: "fixture-password-long", PanelURL: upstream.URL, PanelToken: strings.Repeat("t", 32)}
	body, _ := json.Marshal(input)
	if w := send("POST", installPath+"api/setup", string(body), "https://attacker.test", nil); w.Code != 403 {
		t.Fatalf("跨站安装应拒绝，得到 %d", w.Code)
	}
	w := send("POST", installPath+"api/setup", string(body), cfg.Origin, nil)
	if w.Code != 200 {
		t.Fatalf("安装失败: %d %s", w.Code, w.Body.String())
	}
	if w := send("GET", installPath, "", "", nil); w.Code != 404 {
		t.Fatalf("已安装后的安装入口应关闭: %d", w.Code)
	}
	for _, path := range []string{"/", "/login", "/wrong/api/bootstrap"} {
		if w := send("GET", path, "", "", nil); w.Code != 404 {
			t.Errorf("非入口路径 %s 泄露站点: %d", path, w.Code)
		}
	}
	prefix := "/" + input.Suffix + "/"
	if w := send("GET", prefix+"api/me", "", "", nil); w.Code != 401 {
		t.Fatalf("匿名账户读取应拒绝: %d", w.Code)
	}
	w = send("POST", prefix+"api/login", `{"username":"owner","password":"fixture-password-long"}`, cfg.Origin, nil)
	if w.Code != 200 {
		t.Fatalf("登录失败: %d %s", w.Code, w.Body.String())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("会话 cookie 数量: %d", len(cookies))
	}
	cookie := cookies[0]
	if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != prefix {
		t.Fatalf("会话 cookie 属性不符: %+v", cookie)
	}
	if w := send("POST", prefix+"api/logout", `{}`, cfg.Origin, cookie); w.Code != 403 {
		t.Fatalf("无 CSRF 不应退出: %d", w.Code)
	}
	w = send("GET", prefix+"api/me", "", "", cookie)
	if w.Code != 200 {
		t.Fatalf("已登录读取失败: %d", w.Code)
	}
	if strings.Contains(w.Body.String(), input.PanelToken) || strings.Contains(w.Body.String(), "password_hash") {
		t.Fatal("账户响应泄露凭据")
	}
	var stored string
	if err := db.QueryRow("SELECT panel_token FROM site").Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == input.PanelToken {
		t.Fatal("数据库保存了明文密钥")
	}
}
