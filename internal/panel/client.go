package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	base, token string
	http        *http.Client
}
type Record struct {
	ID       int64  `json:"id"`
	Email    string `json:"email"`
	SubID    string `json:"subId"`
	UUID     string `json:"uuid"`
	Password string `json:"password"`
	Auth     string `json:"auth"`
	Secret   string `json:"secret"`
	Enable   bool   `json:"enable"`
	Expiry   int64  `json:"expiryTime"`
	Total    int64  `json:"totalGB"`
}
type Detail struct {
	Client     Record  `json:"client"`
	InboundIDs []int64 `json:"inboundIds"`
}
type Traffic struct {
	Email  string `json:"email"`
	Enable bool   `json:"enable"`
	Up     int64  `json:"up"`
	Down   int64  `json:"down"`
	Total  int64  `json:"total"`
	Expiry int64  `json:"expiryTime"`
}
type Choice struct {
	ID     int64  `json:"id"`
	Email  string `json:"email"`
	Enable bool   `json:"enable"`
}
type Page struct {
	Items []Choice `json:"items"`
	Total int      `json:"total"`
}

func New(base, token string) (*Client, error) {
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("请输入完整面板地址，不能包含账号、查询参数或片段")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, errors.New("面板地址必须使用 HTTP 或 HTTPS")
	}
	if strings.Contains(u.EscapedPath(), "%") || strings.Contains(u.Path, "..") || strings.Contains(u.Path, "\\") {
		return nil, errors.New("面板路径格式不受支持")
	}
	if len(token) < 16 || len(token) > 4096 || strings.ContainsAny(token, "\r\n") {
		return nil, errors.New("API 密钥格式不正确")
	}
	tr := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second, MaxIdleConnsPerHost: 4}
	return &Client{base: strings.TrimRight(u.String(), "/"), token: token, http: &http.Client{Transport: tr, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// 仅站长能够设置目标；关闭环境代理和重定向，避免把 Bearer 发到另一个地址。
func (p *Client) request(ctx context.Context, method, path string, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}
	r, err := http.NewRequestWithContext(ctx, method, p.base+"/panel/api"+path, reader)
	if err != nil {
		return nil, errors.New("面板请求地址无效")
	}
	r.Header.Set("Authorization", "Bearer "+p.token)
	r.Header.Set("Accept", "application/json")
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	resp, err := p.http.Do(r)
	if err != nil {
		return nil, errors.New("面板连接失败，请检查网络、HTTPS 证书和地址")
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case 401:
		return nil, errors.New("面板密钥无效或已失效")
	case 403:
		return nil, errors.New("面板密钥权限不足，需要 admin API Token")
	case 404:
		return nil, errors.New("面板路径或 API 不匹配，请检查路径后缀与版本")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("面板返回 HTTP %d，请稍后重试", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil || len(data) > 4<<20 {
		return nil, errors.New("面板响应过大或读取失败")
	}
	return data, nil
}

func (p *Client) call(ctx context.Context, method, path string, body, out any) error {
	data, err := p.request(ctx, method, path, body)
	if err != nil {
		return err
	}
	var result struct {
		Success bool            `json:"success"`
		Obj     json.RawMessage `json:"obj"`
	}
	if json.Unmarshal(data, &result) != nil {
		return errors.New("面板响应不是预期的 JSON")
	}
	if !result.Success {
		return errors.New("面板拒绝此操作，请检查客户端状态或联系站长")
	}
	if out == nil {
		return nil
	}
	if len(result.Obj) == 0 || string(result.Obj) == "null" {
		return errors.New("面板没有返回对应数据")
	}
	if json.Unmarshal(result.Obj, out) != nil {
		return errors.New("面板字段不兼容")
	}
	return nil
}

func (p *Client) Check(ctx context.Context) error {
	_, err := p.Search(ctx, "")
	return err
}

func (p *Client) Search(ctx context.Context, search string) (Page, error) {
	var page Page
	err := p.call(ctx, "GET", "/clients/list/paged?page=1&pageSize=20&search="+url.QueryEscape(search), nil, &page)
	if page.Items == nil {
		page.Items = []Choice{}
	}
	return page, err
}

func (p *Client) Detail(ctx context.Context, email string) (Detail, error) {
	var d Detail
	err := p.call(ctx, "GET", "/clients/get?email="+url.QueryEscape(email), nil, &d)
	if err == nil && (d.Client.ID <= 0 || d.Client.Email != email) {
		err = errors.New("面板客户端身份不匹配")
	}
	return d, err
}

func (p *Client) Traffic(ctx context.Context, email string) (Traffic, error) {
	var t Traffic
	if email == "." || email == ".." || strings.ContainsAny(email, "/\\") {
		return t, errors.New("此客户端名称暂不支持流量查询，请站长重命名")
	}
	err := p.call(ctx, "GET", "/clients/traffic/"+url.PathEscape(email), nil, &t)
	if err == nil && t.Email != email {
		err = errors.New("面板流量归属不匹配")
	}
	return t, err
}

func validSub(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, "/\\")
}
func (p *Client) Shared(ctx context.Context, sub string) ([]Detail, error) {
	var ds []Detail
	if !validSub(sub) {
		return nil, errors.New("订阅标识不可用")
	}
	err := p.call(ctx, "GET", "/clients/get/subId/"+url.PathEscape(sub), nil, &ds)
	return ds, err
}
func (p *Client) Links(ctx context.Context, sub string) ([]string, error) {
	if !validSub(sub) {
		return nil, errors.New("尚未配置订阅")
	}
	var links []string
	err := p.call(ctx, "GET", "/clients/subLinks/"+url.PathEscape(sub), nil, &links)
	if err != nil {
		return nil, err
	}
	safe := []string{}
	for _, link := range links {
		u, e := url.Parse(link)
		if e == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil {
			safe = append(safe, link)
		}
	}
	if len(safe) == 0 {
		return nil, errors.New("面板未返回可用订阅地址，请检查订阅服务")
	}
	return safe, nil
}

func (p *Client) LocalInbounds(ctx context.Context, ids []int64) error {
	if len(ids) == 0 || len(ids) > 8 {
		return errors.New("该绑定暂不支持自助重置，请联系站长")
	}
	for _, id := range ids {
		var row struct {
			ID     int64  `json:"id"`
			Enable bool   `json:"enable"`
			NodeID *int64 `json:"nodeId"`
		}
		if err := p.call(ctx, "GET", fmt.Sprintf("/inbounds/get/%d", id), nil, &row); err != nil {
			return err
		}
		if row.ID != id || !row.Enable || (row.NodeID != nil && *row.NodeID != 0) {
			return errors.New("仅状态正常的本地入站支持本版自助重置")
		}
	}
	return nil
}

func (p *Client) Rotate(ctx context.Context, email, target string) error {
	// 成功响应可能没有 obj；只检查 success，完整状态由调用方读回。
	err := p.call(ctx, "POST", "/clients/update?email="+url.QueryEscape(email), map[string]string{"subId": target}, nil)
	return err
}

func (p *Client) CheckRotation(ctx context.Context) error {
	data, err := p.request(ctx, "GET", "/openapi.json", nil)
	if err != nil {
		return err
	}
	var spec struct {
		Paths map[string]map[string]struct {
			Summary     string `json:"summary"`
			Description string `json:"description"`
		} `json:"paths"`
	}
	if json.Unmarshal(data, &spec) != nil {
		return errors.New("无法确认面板的更新契约，暂不启用重置")
	}
	for path, ops := range spec.Paths {
		if strings.Contains(path, "/clients/update/") {
			op := ops["post"]
			if strings.Contains(op.Summary+op.Description, "omitted fields keep their current values") {
				return nil
			}
		}
	}
	return errors.New("该面板未声明兼容的局部更新契约，请联系站长核实版本")
}

func SameProtected(a, b Record) bool {
	return a.ID == b.ID && a.Email == b.Email && a.UUID == b.UUID && a.Password == b.Password && a.Auth == b.Auth && a.Secret == b.Secret && a.Enable == b.Enable && a.Expiry == b.Expiry && a.Total == b.Total
}
