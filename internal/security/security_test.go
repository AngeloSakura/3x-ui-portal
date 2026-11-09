package security

import (
	"bytes"
	"encoding/base64"
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

func TestEntrySuffixRejectsTraversalAndShortNames(t *testing.T) {
	for _, s := range []string{"admin", "../../private", "abcdefghijk/secret", "abcdefghijkl?x=1", "abcdefghijkl%2fsecret", "abcdefghijkl\\secret"} {
		if ValidSuffix(s) {
			t.Errorf("不应接受 %q", s)
		}
	}
	if !ValidSuffix("portal_7Kp-29xQ8m") {
		t.Fatal("有效后缀被拒绝")
	}
}
