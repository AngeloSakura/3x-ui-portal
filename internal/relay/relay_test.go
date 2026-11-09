package relay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchPreservesSubscriptionWithoutRedirectOrCredentials(t *testing.T) {
	var calls int
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Error("不应转发凭据")
		}
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/subscription", 302)
			return
		}
		if r.Header.Get("User-Agent") != "fixture-client" {
			t.Error("客户端格式协商头丢失")
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Subscription-Userinfo", "upload=1; download=2; total=100")
		w.Header().Set("Set-Cookie", "upstream-secret=hidden")
		_, _ = w.Write([]byte("fixture-subscription"))
	}))
	defer origin.Close()
	allowed, err := Origins(origin.URL, origin.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := New()
	result, err := client.Fetch(context.Background(), origin.URL+"/subscription", "fixture-client", "text/plain", allowed)
	if err != nil || string(result.Body) != "fixture-subscription" || result.Headers.Get("Subscription-Userinfo") == "" || result.Headers.Get("Set-Cookie") != "" {
		t.Fatalf("订阅响应错误: %v", err)
	}
	if _, err = client.Fetch(context.Background(), origin.URL+"/redirect", "fixture-client", "", allowed); err == nil {
		t.Fatal("不应跟随重定向")
	}
	if calls != 2 {
		t.Fatalf("发生额外网络请求: %d", calls)
	}
	if _, err = client.Fetch(context.Background(), origin.URL+"/subscription", "", "", map[string]bool{}); err == nil || calls != 2 {
		t.Fatal("未授权来源不应产生网络请求")
	}
}

func TestOriginsRejectPathsAndUnsupportedSchemes(t *testing.T) {
	if _, err := Origins("http://example.com:8080", "https://panel.example.com"); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"ftp://example.com", "https://example.com/path", "https://user:pass@example.com", "https://example.com?url=other"} {
		if _, err := Origins(v, "https://panel.example.com"); err == nil {
			t.Errorf("不应接受来源 %s", v)
		}
	}
}
