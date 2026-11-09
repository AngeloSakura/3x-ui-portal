// Package relay 只获取已配置来源的订阅，不接受浏览器指定任意目标。
package relay

import (
	"context"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func Origin(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || len(raw) > 4096 {
		return "", errors.New("订阅源地址无效")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return "", errors.New("订阅源须使用 HTTP 或 HTTPS")
	}
	return u.Scheme + "://" + strings.ToLower(u.Host), nil
}

// 空配置默认只允许面板同源；独立订阅域名/端口须由站长明确加入。
func Origins(config, panelURL string) (map[string]bool, error) {
	result := map[string]bool{}
	if strings.TrimSpace(config) == "" {
		origin, err := Origin(panelURL)
		if err != nil {
			return nil, err
		}
		result[origin] = true
		return result, nil
	}
	if len(config) > 2048 {
		return nil, errors.New("订阅来源配置过长")
	}
	lines := strings.Fields(config)
	if len(lines) > 8 {
		return nil, errors.New("最多配置 8 个订阅来源")
	}
	for _, line := range lines {
		u, err := url.Parse(line)
		if err != nil || u.RawQuery != "" || (u.Path != "" && u.Path != "/") {
			return nil, errors.New("订阅来源只填写协议、域名或 IP 和端口，不含路径")
		}
		origin, err := Origin(line)
		if err != nil {
			return nil, err
		}
		result[origin] = true
	}
	return result, nil
}

type Client struct{ http *http.Client }

func New() *Client {
	return &Client{http: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 12 * time.Second, MaxIdleConns: 16, MaxIdleConnsPerHost: 4, IdleConnTimeout: 60 * time.Second}}}
}

type Result struct {
	Body        []byte
	ContentType string
	Headers     http.Header
}

func (r *Client) Fetch(ctx context.Context, target, agent, accept string, allowed map[string]bool) (Result, error) {
	var result Result
	origin, err := Origin(target)
	if err != nil || !allowed[origin] {
		return result, errors.New("此订阅源尚未获站长允许中转")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", target, nil)
	if err != nil {
		return result, errors.New("订阅地址无效")
	}
	// 仅保留影响订阅格式的协商头；绝不转发管理 Token、Cookie、来源 IP 或任意浏览器头。
	if len(agent) <= 512 && !strings.ContainsAny(agent, "\r\n") {
		req.Header.Set("User-Agent", agent)
	}
	if len(accept) <= 512 && !strings.ContainsAny(accept, "\r\n") {
		req.Header.Set("Accept", accept)
	}
	resp, err := r.http.Do(req)
	if err != nil {
		return result, errors.New("原站订阅连接失败，请稍后重试或使用备用地址")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return result, errors.New("原站暂未返回有效订阅；未跟随重定向")
	}
	contentType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return result, errors.New("原站订阅内容类型无效")
	}
	switch contentType {
	case "text/plain", "text/yaml", "text/x-yaml", "application/yaml", "application/x-yaml", "application/json", "application/octet-stream":
	default:
		return result, errors.New("原站返回了不支持的内容类型，无法作为订阅中转")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if err != nil || len(body) == 0 || len(body) > 8<<20 {
		return result, errors.New("订阅内容为空、过大或传输失败")
	}
	prefix := strings.ToLower(strings.TrimSpace(string(body[:min(len(body), 256)])))
	if strings.HasPrefix(prefix, "<!doctype html") || strings.HasPrefix(prefix, "<html") {
		return result, errors.New("原站返回网页而非订阅内容")
	}
	result = Result{Body: body, ContentType: contentType, Headers: make(http.Header)}
	for _, name := range []string{"Subscription-Userinfo", "Profile-Update-Interval", "Profile-Title"} {
		if value := resp.Header.Get(name); len(value) <= 2048 {
			result.Headers.Set(name, value)
		}
	}
	return result, nil
}
