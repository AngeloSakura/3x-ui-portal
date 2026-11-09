package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"regexp"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var suffixPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{2,31}$`)

func ValidSuffix(s string) bool   { return suffixPattern.MatchString(s) && s != "sub" }
func ValidUsername(s string) bool { return usernamePattern.MatchString(s) }

func Password(password string) error {
	if len(password) == 0 || len(password) > 72 {
		return errors.New("密码不能为空，且不能超过 72 个 UTF-8 字节")
	}
	return nil
}

func HashPassword(password string) (string, error) {
	if err := Password(password); err != nil {
		return "", err
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	return string(h), err
}

func Random(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func Digest(s string) string { b := sha256.Sum256([]byte(s)); return hex.EncodeToString(b[:]) }

type Vault struct {
	aead       cipher.AEAD
	signingKey [32]byte
}

func NewVault(key []byte) (*Vault, error) {
	if len(key) != 32 {
		return nil, errors.New("主密钥必须为 32 字节")
	}
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	a, err := cipher.NewGCM(b)
	if err != nil {
		return nil, err
	}
	derived := hmac.New(sha256.New, key)
	_, _ = derived.Write([]byte("portal-subscription-signing-v1"))
	var signingKey [32]byte
	copy(signingKey[:], derived.Sum(nil))
	return &Vault{aead: a, signingKey: signingKey}, nil
}

// 独立用途的派生密钥使中转地址稳定，且不能伪造用户或原站目标。
func (v *Vault) Sign(payload string) string {
	m := hmac.New(sha256.New, v.signingKey[:])
	_, _ = m.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}
func (v *Vault) Verify(payload, signature string) bool {
	return hmac.Equal([]byte(v.Sign(payload)), []byte(signature))
}

func (v *Vault) Seal(plain string) (string, error) {
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(v.aead.Seal(nonce, nonce, []byte(plain), []byte("portal-v1"))), nil
}

func (v *Vault) Open(secret string) (string, error) {
	b, err := base64.RawStdEncoding.DecodeString(secret)
	if err != nil || len(b) < v.aead.NonceSize() {
		return "", errors.New("密文格式错误")
	}
	n := v.aead.NonceSize()
	p, err := v.aead.Open(nil, b[:n], b[n:], []byte("portal-v1"))
	return string(p), err
}

type bucket struct {
	count int
	until time.Time
}
type Limiter struct {
	mu      sync.Mutex
	entries map[string]bucket
}

func NewLimiter() *Limiter { return &Limiter{entries: make(map[string]bucket)} }

// 固定窗口限制在密码计算之前执行；达到容量时拒绝新键，避免匿名请求撑爆内存。
func (l *Limiter) Allow(key string, limit int, window time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.entries) > 10000 {
		for k, b := range l.entries {
			if now.After(b.until) {
				delete(l.entries, k)
			}
		}
	}
	b, ok := l.entries[key]
	if !ok && len(l.entries) >= 20000 {
		return false
	}
	if !ok || now.After(b.until) {
		b = bucket{until: now.Add(window)}
	}
	if b.count >= limit {
		return false
	}
	b.count++
	l.entries[key] = b
	return true
}
