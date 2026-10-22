package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aluzed/grapi"
)

func TestEncryptKeyRoundTrip(t *testing.T) {
	enc, err := encryptKey("secret", "token")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(enc, ":")
	if len(parts) != 4 || parts[0] != "v1" || len(parts[1]) != 32 {
		t.Fatalf("format %s", enc)
	}
	key := sha256.Sum256([]byte("secret"))
	block, _ := aes.NewCipher(key[:])
	gcm, _ := cipher.NewGCMWithNonceSize(block, 16)
	iv, _ := hex.DecodeString(parts[1])
	data, _ := hex.DecodeString(parts[2])
	tag, _ := hex.DecodeString(parts[3])
	out, err := gcm.Open(nil, iv, append(data, tag...), nil)
	if err != nil || string(out) != "token" {
		t.Fatalf("decrypt: %q %v", out, err)
	}
}

// A project managed from the CLI only: grant a permission, create a
// token, use it.
func TestTokensAndPermissions(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	var out bytes.Buffer
	if code := run([]string{"new", "demo"}, &out, &out); code != 0 {
		t.Fatalf("new: %s", out.String())
	}
	project := filepath.Join(dir, "demo")
	schema := `{"kind":"collectionType","collectionName":"notes","info":{"singularName":"note","pluralName":"notes","displayName":"Note"},"options":{},"attributes":{"text":{"type":"string"}}}`
	if err := os.MkdirAll(filepath.Join(project, "src/api/note/content-types/note"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "src/api/note/content-types/note/schema.json"), []byte(schema), 0o600); err != nil {
		t.Fatal(err)
	}
	srv, err := grapi.New(grapi.Options{Dir: project, Stderr: &out}).Setup()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()
	get := func(token string) int {
		req, _ := http.NewRequest("GET", ts.URL+"/api/notes", nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		return res.StatusCode
	}
	if c := get(""); c != 403 {
		t.Fatalf("public before grant: %d", c)
	}
	out.Reset()
	if code := run([]string{"permissions:grant", "public", "api::note.note.find", "-dir", project}, &out, &out); code != 0 {
		t.Fatalf("grant: %s", out.String())
	}
	if c := get(""); c != 200 {
		t.Fatalf("public after grant: %d", c)
	}
	out.Reset()
	if code := run([]string{"api-token:create", "-dir", project, "--name", "ci", "--type", "custom", "--permissions", "api::note.note.find"}, &out, &out); code != 0 {
		t.Fatalf("token: %s", out.String())
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	token := lines[len(lines)-1]
	if len(token) != 256 {
		t.Fatalf("token %q", token)
	}
	run([]string{"permissions:revoke", "public", "api::note.note.find", "-dir", project}, &out, &out)
	if c := get(""); c != 403 {
		t.Fatalf("public after revoke: %d", c)
	}
	if c := get(token); c != 200 {
		t.Fatalf("custom token: %d", c)
	}
	p, err := grapi.LoadProject(project, "", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	var last sql.NullString
	err = p.DB.QueryRow(context.Background(), `select cast("last_used_at" as varchar) from "strapi_api_tokens"`).Scan(&last)
	_ = p.DB.Close()
	if err != nil || !last.Valid || last.String == "" {
		t.Fatalf("lastUsedAt is set on use: %v %v", last, err)
	}
	out.Reset()
	if code := run([]string{"api-token:regenerate", "-dir", project, "--id", "1"}, &out, &out); code != 0 {
		t.Fatalf("regenerate: %s", out.String())
	}
	lines = strings.Split(strings.TrimSpace(out.String()), "\n")
	fresh := lines[len(lines)-1]
	if c := get(token); c != 401 {
		t.Fatalf("the old key after regenerate: %d", c)
	}
	if c := get(fresh); c != 200 {
		t.Fatalf("the new key: %d", c)
	}
	out.Reset()
	run([]string{"api-token:list", "-dir", project}, &out, &out)
	if !strings.Contains(out.String(), "ci") || strings.Contains(out.String(), token) {
		t.Fatalf("list: %s", out.String())
	}
}
