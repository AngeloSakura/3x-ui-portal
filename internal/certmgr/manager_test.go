package certmgr

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"testing"
	"time"

	"github.com/AngeloSakura/3x-ui-portal/internal/security"
	"github.com/go-acme/lego/v5/certificate"
)

func fixture(t *testing.T, host string, serial int64) *certificate.Resource {
	t.Helper()
	k, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	c := &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: host}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(159 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	if ip := net.ParseIP(host); ip != nil {
		c.IPAddresses = []net.IP{ip}
	} else {
		c.DNSNames = []string{host}
	}
	der, e := x509.CreateCertificate(rand.Reader, c, c, &k.PublicKey, k)
	if e != nil {
		t.Fatal(e)
	}
	key, e := x509.MarshalECPrivateKey(k)
	if e != nil {
		t.Fatal(e)
	}
	return &certificate.Resource{Domains: []string{host}, Certificate: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), PrivateKey: pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: key})}
}
func TestTLSReloadPersistenceAndFailedRenewal(t *testing.T) {
	dir := t.TempDir()
	vault, _ := security.NewVault(bytes.Repeat([]byte{7}, 32))
	m, e := New(dir, "127.0.0.1", "127.0.0.1:8080", vault)
	if e != nil {
		t.Fatal(e)
	}
	first := fixture(t, "127.0.0.1", 1)
	if e = m.Import(first.Certificate, first.PrivateKey); e != nil {
		t.Fatal(e)
	}
	wrong := fixture(t, "other.example", 3)
	if m.Import(wrong.Certificate, wrong.PrivateKey) == nil {
		t.Fatal("accepted wrong SAN")
	}
	listener, e := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: m.GetCertificate})
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	go func() {
		for {
			c, e := listener.Accept()
			if e != nil {
				return
			}
			go func() { defer c.Close(); _ = c.(*tls.Conn).Handshake() }()
		}
	}()
	check := func(r *certificate.Resource, serial int64) {
		t.Helper()
		roots := x509.NewCertPool()
		roots.AppendCertsFromPEM(r.Certificate)
		c, e := tls.Dial("tcp", listener.Addr().String(), &tls.Config{RootCAs: roots, ServerName: "127.0.0.1", MinVersion: tls.VersionTLS12})
		if e != nil {
			t.Fatal(e)
		}
		defer c.Close()
		if c.ConnectionState().PeerCertificates[0].SerialNumber.Int64() != serial {
			t.Fatal("old certificate served")
		}
	}
	check(first, 1)
	if e = m.Configure(Settings{Email: "admin@example.com", Method: "http", AgreeTerms: true}); e != nil {
		t.Fatal(e)
	}
	second := fixture(t, "127.0.0.1", 2)
	m.issue = func(context.Context, string, string, Settings, []byte, *certificate.Resource) (*certificate.Resource, error) {
		return second, nil
	}
	if e = m.Issue(context.Background()); e != nil {
		t.Fatal(e)
	}
	check(second, 2)
	if m.Status().RenewAt.Sub(secondTime(t, second)) > 108*time.Hour {
		t.Fatal("short-lived renewal scheduled too late")
	}
	restored, e := New(dir, "127.0.0.1", "127.0.0.1:8080", vault)
	if e != nil {
		t.Fatal(e)
	}
	cert, e := restored.GetCertificate(nil)
	if e != nil || cert.Leaf.SerialNumber.Int64() != 2 {
		t.Fatal("restart did not restore certificate")
	}
	m.mu.Lock()
	m.state.LastAttempt = time.Now().Add(-2 * time.Hour)
	m.mu.Unlock()
	m.issue = func(context.Context, string, string, Settings, []byte, *certificate.Resource) (*certificate.Resource, error) {
		return nil, errors.New("secret-token-do-not-leak")
	}
	if e = m.Issue(context.Background()); e == nil {
		t.Fatal("expected failure")
	}
	check(second, 2)
	b, _ := json.Marshal(m.Status())
	if bytes.Contains(b, []byte("secret-token")) {
		t.Fatal("raw issuer error leaked")
	}
	ciphertext, _ := os.ReadFile(m.path)
	if bytes.Contains(ciphertext, []byte("PRIVATE KEY")) {
		t.Fatal("private key stored in plaintext")
	}
}
func secondTime(t *testing.T, r *certificate.Resource) time.Time {
	t.Helper()
	c, e := parse(r, "127.0.0.1", true)
	if e != nil {
		t.Fatal(e)
	}
	return c.Leaf.NotBefore
}
func TestDNSSecretAndSingleFlight(t *testing.T) {
	vault, _ := security.NewVault(bytes.Repeat([]byte{4}, 32))
	m, _ := New(t.TempDir(), "portal.example.com", ":8080", vault)
	settings := Settings{Email: "admin@example.com", Method: "cloudflare", DNSToken: "fixture-secret-token", AgreeTerms: true}
	if e := m.Configure(settings); e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(m.Status())
	if bytes.Contains(b, []byte(settings.DNSToken)) {
		t.Fatal("DNS secret in status")
	}
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	m.issue = func(context.Context, string, string, Settings, []byte, *certificate.Resource) (*certificate.Resource, error) {
		close(started)
		<-release
		return nil, errors.New("fixture failure")
	}
	go func() { done <- m.Issue(context.Background()) }()
	<-started
	if m.Configure(settings) == nil {
		t.Fatal("configuration changed during issuance")
	}
	if m.Issue(context.Background()) == nil {
		t.Fatal("concurrent issuance allowed")
	}
	close(release)
	<-done
	if m.Issue(context.Background()) == nil {
		t.Fatal("retry backoff bypassed")
	}
	ip, _ := New(t.TempDir(), "192.0.2.10", ":8080", vault)
	if ip.Configure(settings) == nil {
		t.Fatal("IP accepted DNS challenge")
	}
}
