// Package certmgr 管理单一站点的证书；私钥、ACME 账户和 DNS 凭据整体加密存储。
package certmgr

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"net"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/AngeloSakura/3x-ui-portal/internal/security"
	"github.com/go-acme/lego/v5/certificate"
)

type Settings struct {
	Email      string `json:"email"`
	Method     string `json:"method"`
	Staging    bool   `json:"staging"`
	DNSToken   string `json:"dnsToken,omitempty"`
	AgreeTerms bool   `json:"agreeTerms"`
}
type state struct {
	CertificatePEM []byte                `json:"certificatePem,omitempty"`
	PrivateKeyPEM  []byte                `json:"privateKeyPem,omitempty"`
	IssuerPEM      []byte                `json:"issuerPem,omitempty"`
	Settings       Settings              `json:"settings"`
	AccountKey     []byte                `json:"accountKey"`
	Resource       *certificate.Resource `json:"resource,omitempty"`
	Source         string                `json:"source"`
	LastAttempt    time.Time             `json:"lastAttempt"`
	LastError      string                `json:"lastError"`
}
type Status struct {
	Host        string    `json:"host"`
	Settings    Settings  `json:"settings"`
	HasDNSToken bool      `json:"hasDnsToken"`
	Busy        bool      `json:"busy"`
	Source      string    `json:"source"`
	NotAfter    time.Time `json:"notAfter"`
	RenewAt     time.Time `json:"renewAt"`
	LastAttempt time.Time `json:"lastAttempt"`
	LastError   string    `json:"lastError"`
}
type issuer func(context.Context, string, string, Settings, []byte, *certificate.Resource) (*certificate.Resource, error)
type Manager struct {
	mu                        sync.Mutex
	path, host, challengeAddr string
	vault                     *security.Vault
	state                     state
	cert                      *tls.Certificate
	busy                      bool
	issue                     issuer
}

func New(dir, host, challengeAddr string, vault *security.Vault) (*Manager, error) {
	m := &Manager{path: filepath.Join(dir, "tls-state.enc"), host: host, challengeAddr: challengeAddr, vault: vault, issue: issueACME}
	b, err := os.ReadFile(m.path)
	if err == nil {
		plain, e := vault.Open(string(b))
		if e != nil {
			return nil, errors.New("证书状态解密失败，请恢复正确主密钥")
		}
		if e = json.Unmarshal([]byte(plain), &m.state); e != nil {
			return nil, errors.New("证书状态格式错误")
		}
		if m.state.Resource != nil {
			m.state.Resource.Certificate = m.state.CertificatePEM
			m.state.Resource.PrivateKey = m.state.PrivateKeyPEM
			m.state.Resource.IssuerCertificate = m.state.IssuerPEM
			c, e := parse(m.state.Resource, host, false)
			if e != nil {
				return nil, e
			}
			m.cert = c
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return m, nil
}

func parse(r *certificate.Resource, host string, checkTime bool) (*tls.Certificate, error) {
	c, err := tls.X509KeyPair(r.Certificate, r.PrivateKey)
	if err != nil {
		return nil, errors.New("证书与私钥不匹配")
	}
	c.Leaf, err = x509.ParseCertificate(c.Certificate[0])
	if err != nil {
		return nil, err
	}
	if err = c.Leaf.VerifyHostname(host); err != nil {
		return nil, errors.New("证书不包含当前 PORTAL_ORIGIN 的主机名或 IP")
	}
	if checkTime && (time.Now().Before(c.Leaf.NotBefore) || !time.Now().Before(c.Leaf.NotAfter)) {
		return nil, errors.New("证书尚未生效或已经过期")
	}
	return &c, nil
}

// 同一密文文件原子替换，避免重启时证书和私钥属于不同批次。
func (m *Manager) persist(s state) error {
	// lego v5 的 Resource 故意不序列化 PEM；此处在整体加密前显式保存。
	if s.Resource != nil {
		s.CertificatePEM = s.Resource.Certificate
		s.PrivateKeyPEM = s.Resource.PrivateKey
		s.IssuerPEM = s.Resource.IssuerCertificate
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	enc, err := m.vault.Seal(string(b))
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(m.path), ".tls-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.WriteString(enc)
	}
	if err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err == nil {
		err = ce
	}
	if err == nil {
		err = os.Rename(name, m.path)
	}
	return err
}

func (m *Manager) Configure(s Settings) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.busy {
		return errors.New("证书任务正在运行，请稍后修改")
	}
	email, err := mail.ParseAddress(s.Email)
	if err != nil || email.Address != s.Email || len(s.Email) > 254 {
		return errors.New("请输入有效的 ACME 联系邮箱")
	}
	if !s.AgreeTerms {
		return errors.New("请先同意 Let's Encrypt 订阅者协议")
	}
	if s.Method != "http" && s.Method != "cloudflare" {
		return errors.New("请选择 HTTP-01 或 Cloudflare DNS-01")
	}
	if s.Method == "cloudflare" && net.ParseIP(m.host) != nil {
		return errors.New("IP 证书不能使用 DNS 验证，需要公网 80 验证入口")
	}
	if s.DNSToken == "" {
		s.DNSToken = m.state.Settings.DNSToken
	}
	if s.Method == "cloudflare" && (len(s.DNSToken) < 16 || len(s.DNSToken) > 4096 || strings.ContainsAny(s.DNSToken, "\r\n")) {
		return errors.New("请填写 Cloudflare API Token")
	}
	if s.Method != "cloudflare" {
		s.DNSToken = ""
	}
	next := m.state
	next.Settings = s
	if err = m.persist(next); err != nil {
		return errors.New("证书配置保存失败")
	}
	m.state = next
	return nil
}

func renewAt(c *tls.Certificate) time.Time {
	// 以实际寿命的 2/3 为续期点，同时兼容 160 小时 IP 证书与常规域名证书。
	return c.Leaf.NotBefore.Add(c.Leaf.NotAfter.Sub(c.Leaf.NotBefore) * 2 / 3)
}
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := Status{Host: m.host, Settings: m.state.Settings, HasDNSToken: m.state.Settings.DNSToken != "", Busy: m.busy, Source: m.state.Source, LastAttempt: m.state.LastAttempt, LastError: m.state.LastError}
	s.Settings.DNSToken = ""
	if m.cert != nil {
		s.NotAfter = m.cert.Leaf.NotAfter
		if m.state.Settings.Method != "" {
			s.RenewAt = renewAt(m.cert)
		}
	}
	return s
}
func (m *Manager) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cert == nil {
		return nil, errors.New("尚未安装证书，请先运行 tls-issue 或 tls-import")
	}
	return m.cert, nil
}
func (m *Manager) Import(certPEM, keyPEM []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.busy {
		return errors.New("证书任务正在运行")
	}
	r := &certificate.Resource{Domains: []string{m.host}, Certificate: certPEM, PrivateKey: keyPEM}
	c, err := parse(r, m.host, true)
	if err != nil {
		return err
	}
	next := m.state
	next.Resource = r
	next.Source = "imported"
	next.LastError = ""
	if err = m.persist(next); err != nil {
		return err
	}
	m.state = next
	m.cert = c
	return nil
}

// Start 先同步占用任务槽，再异步申请，避免重复点击建立多个 ACME 订单。
func (m *Manager) Start(ctx context.Context) error {
	if err := m.begin(); err != nil {
		return err
	}
	go func() { _ = m.finish(ctx) }()
	return nil
}
func (m *Manager) Issue(ctx context.Context) error {
	if err := m.begin(); err != nil {
		return err
	}
	return m.finish(ctx)
}
func (m *Manager) begin() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.busy {
		return errors.New("证书任务正在运行")
	}
	if m.state.Settings.Method == "" {
		return errors.New("请先保存证书配置")
	}
	if time.Since(m.state.LastAttempt) < time.Hour {
		return errors.New("两次申请至少间隔一小时，请先检查验证入口或 DNS 配置")
	}
	next := m.state
	if len(next.AccountKey) == 0 {
		b, err := newAccountKey()
		if err != nil {
			return err
		}
		next.AccountKey = b
	}
	next.LastAttempt = time.Now()
	next.LastError = ""
	if err := m.persist(next); err != nil {
		return errors.New("证书任务保存失败")
	}
	m.state = next
	m.busy = true
	return nil
}
func (m *Manager) finish(parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, 10*time.Minute)
	defer cancel()
	m.mu.Lock()
	s := m.state
	m.mu.Unlock()
	old := s.Resource
	if s.Source == "imported" || (s.Source == "staging") != s.Settings.Staging {
		old = nil
	}
	r, err := m.issue(ctx, m.host, m.challengeAddr, s.Settings, s.AccountKey, old)
	var c *tls.Certificate
	if err == nil {
		c, err = parse(r, m.host, true)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	defer func() { m.busy = false }()
	next := m.state
	if err != nil {
		// 底层错误可能包含 DNS token、账户地址或验证秘密，不返回原始错误。
		next.LastError = "申请失败：检查公网 80 验证映射，或 Cloudflare DNS 权限、传播和 CA 限额；旧证书继续使用。"
		m.state = next
		_ = m.persist(next)
		return errors.New(next.LastError)
	}
	next.Resource = r
	next.Source = "letsencrypt"
	if s.Settings.Staging {
		next.Source = "staging"
	}
	next.LastError = ""
	if err = m.persist(next); err != nil {
		m.state.LastError = "证书保存失败，旧证书继续使用"
		return errors.New(m.state.LastError)
	}
	m.state = next
	m.cert = c
	return nil
}
func (m *Manager) Run(ctx context.Context) {
	tick := time.NewTicker(15 * time.Minute)
	defer tick.Stop()
	for {
		s := m.Status()
		if !s.Busy && s.Settings.Method != "" && time.Since(s.LastAttempt) >= time.Hour && (s.RenewAt.IsZero() || !time.Now().Before(s.RenewAt)) {
			_ = m.Start(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
