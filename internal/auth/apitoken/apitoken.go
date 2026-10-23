// Package apitoken holds the key scheme of Strapi's API tokens: a random
// key shown once, stored as its HMAC-SHA512 with admin.apiToken.salt, and
// encrypted with admin.secrets.encryptionKey when one is set.
package apitoken

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// Lifespans are Strapi's API_TOKEN_LIFESPANS, in days (0: unlimited).
var Lifespans = map[int]bool{0: true, 7: true, 30: true, 90: true}

// Day is the unit of the lifespans.
const Day = 24 * time.Hour

// Hash is the stored form of key.
func Hash(salt, key string) string {
	m := hmac.New(sha512.New, []byte(salt))
	m.Write([]byte(key))
	return hex.EncodeToString(m.Sum(nil))
}

// New returns a key (128 random bytes in hex) and the columns that store
// it: accessKey, and encryptedKey when encryptionKey is set.
func New(salt, encryptionKey string) (string, map[string]any, error) {
	raw := make([]byte, 128)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	key := hex.EncodeToString(raw)
	cols := map[string]any{"accessKey": Hash(salt, key)}
	if encryptionKey != "" {
		enc, err := Encrypt(encryptionKey, key)
		if err != nil {
			return "", nil, err
		}
		cols["encryptedKey"] = enc
	}
	return key, cols, nil
}

func gcm(secret string) (cipher.AEAD, error) {
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCMWithNonceSize(block, 16)
}

// Encrypt is the admin encryption service: AES-256-GCM with the SHA-256 of
// the secret, "v1:iv:data:tag" in hex.
func Encrypt(secret, value string) (string, error) {
	g, err := gcm(secret)
	if err != nil {
		return "", err
	}
	iv := make([]byte, 16)
	if _, err := rand.Read(iv); err != nil {
		return "", err
	}
	sealed := g.Seal(nil, iv, []byte(value), nil)
	data, tag := sealed[:len(sealed)-g.Overhead()], sealed[len(sealed)-g.Overhead():]
	return "v1:" + hex.EncodeToString(iv) + ":" + hex.EncodeToString(data) + ":" + hex.EncodeToString(tag), nil
}

// Decrypt reverses Encrypt.
func Decrypt(secret, value string) (string, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 4 || parts[0] != "v1" {
		return "", errors.New("apitoken: unknown encrypted key format")
	}
	var raw [3][]byte
	for i, p := range parts[1:] {
		b, err := hex.DecodeString(p)
		if err != nil {
			return "", err
		}
		raw[i] = b
	}
	g, err := gcm(secret)
	if err != nil {
		return "", err
	}
	out, err := g.Open(nil, raw[0], append(raw[1], raw[2]...), nil)
	return string(out), err
}
