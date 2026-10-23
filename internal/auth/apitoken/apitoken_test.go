package apitoken

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

// The format is Strapi's encryption service: checked with a decryption
// written independently of Decrypt.
func TestEncryptFormat(t *testing.T) {
	enc, err := Encrypt("secret", "token")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(enc, ":")
	if len(parts) != 4 || parts[0] != "v1" || len(parts[1]) != 32 {
		t.Fatalf("format %s", enc)
	}
	key := sha256.Sum256([]byte("secret"))
	block, _ := aes.NewCipher(key[:])
	g, _ := cipher.NewGCMWithNonceSize(block, 16)
	iv, _ := hex.DecodeString(parts[1])
	data, _ := hex.DecodeString(parts[2])
	tag, _ := hex.DecodeString(parts[3])
	out, err := g.Open(nil, iv, append(data, tag...), nil)
	if err != nil || string(out) != "token" {
		t.Fatalf("decrypt: %q %v", out, err)
	}
}

func TestNewAndDecrypt(t *testing.T) {
	key, cols, err := New("salt", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != 256 || cols["accessKey"] != Hash("salt", key) {
		t.Fatalf("key %d, cols %v", len(key), cols)
	}
	got, err := Decrypt("secret", cols["encryptedKey"].(string))
	if err != nil || got != key {
		t.Fatalf("Decrypt = %q, %v", got, err)
	}
	if _, err := Decrypt("other", cols["encryptedKey"].(string)); err == nil {
		t.Error("a wrong secret fails")
	}
	if _, cols, _ := New("salt", ""); cols["encryptedKey"] != nil {
		t.Error("no encryption key, no encryptedKey")
	}
}
