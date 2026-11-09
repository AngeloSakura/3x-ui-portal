package security

import (
	"bytes"
	"encoding/base64"
	"golang.org/x/crypto/bcrypt"
	"strings"
	"testing"
)

func TestVaultRejectsChangedCiphertextAndWrongKey(t *testing.T) {
	v, err := NewVault(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	secret, err := v.Seal("test-panel-token")
	if err != nil {
		t.Fatal(err)
	}
	plain, err := v.Open(secret)
	if err != nil || plain != "test-panel-token" {
		t.Fatalf("解密不符: %q %v", plain, err)
	}
	data, err := base64.RawStdEncoding.DecodeString(secret)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-1] ^= 1
	if _, err = v.Open(base64.RawStdEncoding.EncodeToString(data)); err == nil {
		t.Fatal("被修改的密文应拒绝")
	}
	other, err := NewVault(bytes.Repeat([]byte{2}, 32))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = other.Open(secret); err == nil {
		t.Fatal("错误主密钥应拒绝")
	}
}

func TestEntrySuffixAllowsShortNamesAndRejectsUnsafePaths(t *testing.T) {
	for _, s := range []string{"", "sub", strings.Repeat("a", 65), "../../private", "abcdefghijk/secret", "abcdefghijkl?x=1", "abcdefghijkl%2fsecret", "abcdefghijkl\\secret"} {
		if ValidSuffix(s) {
			t.Errorf("不应接受 %q", s)
		}
	}
	for _, s := range []string{"a", "admin", "portal_7Kp-29xQ8m", strings.Repeat("a", 64)} {
		if !ValidSuffix(s) {
			t.Errorf("有效后缀被拒绝: %q", s)
		}
	}
}

func TestPasswordMatchesPanelNonemptyBcryptLimit(t *testing.T) {
	for _, p := range []string{"a", "123456", " admin ", " ", strings.Repeat("密", 24)} {
		if err := Password(p); err != nil {
			t.Errorf("valid password rejected: %v", err)
		}
	}
	for _, p := range []string{"", strings.Repeat("a", 73), strings.Repeat("密", 25)} {
		if Password(p) == nil {
			t.Error("empty or overlong password accepted")
		}
	}
	hash, err := HashPassword(" a ")
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(" a ")) != nil {
		t.Fatal("password did not round trip")
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte("a")) == nil {
		t.Fatal("password whitespace was discarded")
	}
}
