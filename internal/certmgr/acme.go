package certmgr

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-acme/lego/v5/acme"
	"github.com/go-acme/lego/v5/certcrypto"
	"github.com/go-acme/lego/v5/certificate"
	"github.com/go-acme/lego/v5/challenge/http01"
	"github.com/go-acme/lego/v5/lego"
	legolog "github.com/go-acme/lego/v5/log"
	"github.com/go-acme/lego/v5/providers/dns/cloudflare"
	"github.com/go-acme/lego/v5/registration"
)

func init() { legolog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil))) }

type account struct {
	email        string
	key          crypto.Signer
	registration *acme.ExtendedAccount
}

func (a *account) GetEmail() string                       { return a.email }
func (a *account) GetPrivateKey() crypto.Signer           { return a.key }
func (a *account) GetRegistration() *acme.ExtendedAccount { return a.registration }
func newAccountKey() ([]byte, error) {
	k, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		return nil, e
	}
	b, e := x509.MarshalECPrivateKey(k)
	if e != nil {
		return nil, e
	}
	return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: b}), nil
}
func issueACME(ctx context.Context, host, challengeAddr string, s Settings, key []byte, old *certificate.Resource) (*certificate.Resource, error) {
	block, _ := pem.Decode(key)
	if block == nil {
		return nil, errors.New("invalid account key")
	}
	k, e := x509.ParseECPrivateKey(block.Bytes)
	if e != nil {
		return nil, e
	}
	a := &account{email: s.Email, key: k}
	cfg := lego.NewConfig(a)
	if s.Staging {
		cfg.CADirURL = lego.DirectoryURLLetsEncryptStaging
	}
	cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 10 * time.Second}).DialContext, TLSHandshakeTimeout: 10 * time.Second}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	cfg.Certificate.Timeout = 3 * time.Minute
	client, e := lego.NewClient(cfg)
	if e != nil {
		return nil, e
	}
	a.registration, e = client.Registration.Register(ctx, registration.RegisterOptions{TermsOfServiceAgreed: true})
	if e != nil {
		return nil, e
	}
	switch s.Method {
	case "http":
		h, p, e := net.SplitHostPort(challengeAddr)
		if e != nil {
			return nil, e
		}
		if e = client.Challenge.SetHTTP01Provider(http01.NewProviderServer(h, p)); e != nil {
			return nil, e
		}
	case "cloudflare":
		cf := cloudflare.NewDefaultConfig()
		cf.AuthToken = s.DNSToken
		cf.ZoneToken = s.DNSToken
		provider, e := cloudflare.NewDNSProviderConfig(cf)
		if e != nil {
			return nil, e
		}
		if e = client.Challenge.SetDNS01Provider(provider); e != nil {
			return nil, e
		}
	default:
		return nil, errors.New("unsupported challenge")
	}
	profile := ""
	if net.ParseIP(host) != nil {
		profile = "shortlived"
	}
	if old != nil {
		return client.Certificate.Renew(ctx, *old, &certificate.RenewOptions{Profile: profile, Bundle: true, UseARICertID: true})
	}
	return client.Certificate.Obtain(ctx, certificate.ObtainRequest{Domains: []string{host}, Profile: profile, KeyType: certcrypto.EC256, Bundle: true})
}
