package grapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"testing"

	"github.com/aluzed/grapi"
	"github.com/aluzed/grapi/query"
)

func uploadSecurityServer(t *testing.T, conf string) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config", "plugins.yaml"), []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	app := grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}})
	srv, err := app.Setup()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	ctx := context.Background()
	public, _ := app.Query("plugin::users-permissions.role").FindOne(ctx, query.Params{Where: map[string]any{"type": "public"}})
	roleID, _ := public.Get("id")
	if _, err := app.Query("plugin::users-permissions.permission").Create(ctx, query.Data{"action": "plugin::upload.content-api.upload", "role": roleID}, query.Params{}); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler)
	t.Cleanup(ts.Close)
	return ts
}

type upFile struct {
	name, mime string
	body       []byte
}

func securityPNG() []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 4, 4)))
	return b.Bytes()
}

func postFiles(t *testing.T, ts *httptest.Server, files ...upFile) (int, any) {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	for _, f := range files {
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", `form-data; name="files"; filename="`+f.name+`"`)
		if f.mime != "" {
			h.Set("Content-Type", f.mime)
		}
		part, _ := mw.CreatePart(h)
		_, _ = part.Write(f.body)
	}
	_ = mw.Close()
	res, err := http.Post(ts.URL+"/api/upload", mw.FormDataContentType(), &b)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var out any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

func securityMessage(v any) string {
	m, _ := v.(map[string]any)
	e, _ := m["error"].(map[string]any)
	s, _ := e["message"].(string)
	return s
}

func TestUploadSecurity(t *testing.T) {
	ts := uploadSecurityServer(t, "upload:\n  config:\n    security:\n      allowedTypes: ['image/*']\n      deniedTypes: ['image/svg+xml']\n")
	img := securityPNG()

	if s, out := postFiles(t, ts, upFile{"a.png", "image/png", img}); s != 201 {
		t.Fatalf("png: %d %v", s, out)
	}
	// The content wins over the declared type, both ways.
	if s, out := postFiles(t, ts, upFile{"looks.txt", "text/plain", img}); s != 201 {
		t.Fatalf("a png declared as text: %d %v", s, out)
	}
	if s, out := postFiles(t, ts, upFile{"run.sh", "text/x-sh", []byte("#!/bin/sh\necho hi\n")}); s != 400 || securityMessage(out) != "File type 'text/x-sh' is not allowed" {
		t.Fatalf("shell script: %d %v", s, out)
	}
	if s, out := postFiles(t, ts, upFile{"blob.bin", "application/octet-stream", []byte{0x01, 0x02, 0x03, 0xfe}}); s != 400 || securityMessage(out) != "Cannot verify file type for security reasons" {
		t.Fatalf("unknown binary: %d %v", s, out)
	}
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	if s, out := postFiles(t, ts, upFile{"x.svg", "image/svg+xml", svg}); s != 400 || securityMessage(out) != "File type 'image/svg+xml' is not allowed" {
		t.Fatalf("denied svg: %d %v", s, out)
	}
	// Parameters do not hide a denied type, and an image claim needs an image.
	ts2 := uploadSecurityServer(t, "upload:\n  config:\n    security:\n      deniedTypes: ['text/html']\n")
	if s, out := postFiles(t, ts2, upFile{"x.html", "text/html; charset=utf-8", []byte("<script>alert(1)</script>")}); s != 400 || securityMessage(out) != "File type 'text/html' is not allowed" {
		t.Fatalf("declared type with parameters: %d %v", s, out)
	}
	if s, out := postFiles(t, ts, upFile{"fake.png", "image/png", svg}); s != 400 || securityMessage(out) != "Cannot verify file type for security reasons" {
		t.Fatalf("an svg declared as png: %d %v", s, out)
	}
	// Refused files are left out of a batch that has allowed ones.
	s, out := postFiles(t, ts, upFile{"b.png", "image/png", img}, upFile{"run.sh", "text/x-sh", []byte("echo")})
	if list, _ := out.([]any); s != 201 || len(list) != 1 || list[0].(map[string]any)["name"] != "b.png" {
		t.Fatalf("mixed batch: %d %v", s, out)
	}
}

func TestUploadSecurityInvalidConfig(t *testing.T) {
	ts := uploadSecurityServer(t, "upload:\n  config:\n    security:\n      allowedTypes: 'image/*'\n")
	if s, out := postFiles(t, ts, upFile{"a.png", "image/png", securityPNG()}); s != 400 || securityMessage(out) != "Invalid configuration: allowedTypes must be an array of strings." {
		t.Fatalf("invalid config: %d %v", s, out)
	}
}

func TestUploadSecurityChecksActiveFileExtensions(t *testing.T) {
	ts := uploadSecurityServer(t, "upload:\n  config:\n    security:\n      deniedTypes: ['text/html', 'image/svg+xml', 'application/javascript', 'text/javascript']\n")
	for _, file := range []upFile{
		{"attack.html", "text/plain", []byte(`<script>alert(document.domain)</script>`)},
		{"attack.SVG", "text/plain", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)},
		{"attack.js", "text/plain", []byte(`alert(document.domain)`)},
	} {
		if status, out := postFiles(t, ts, file); status != 400 {
			t.Errorf("a false declared MIME type cannot allow %s: %d %v", file.name, status, out)
		}
	}
	allowed := uploadSecurityServer(t, "upload:\n  config:\n    security:\n      allowedTypes: ['application/javascript']\n")
	if status, out := postFiles(t, allowed, upFile{"allowed.js", "application/javascript", []byte("export const value = 1")}); status != 201 {
		t.Fatalf("an explicitly allowed JavaScript MIME alias remains usable: %d %v", status, out)
	}
}

func TestUploadWithoutSecurity(t *testing.T) {
	ts := uploadSecurityServer(t, "upload:\n  config:\n    sizeLimit: 1000000\n")
	if s, out := postFiles(t, ts, upFile{"run.sh", "text/x-sh", []byte("echo")}); s != 201 {
		t.Fatalf("no security config, no restriction: %d %v", s, out)
	}
}
