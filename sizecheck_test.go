package grapi_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aluzed/grapi"
	"github.com/aluzed/grapi/internal/db/query"
)

type limitedStorage struct{ checked int64 }

func (l *limitedStorage) Upload(_ context.Context, f *grapi.UploadFile, r io.Reader) error {
	_, _ = io.Copy(io.Discard, r)
	f.URL = "https://cdn.example.com/" + f.Hash + f.Ext
	return nil
}
func (l *limitedStorage) Delete(context.Context, *grapi.UploadFile) error { return nil }
func (l *limitedStorage) IsPrivate() bool                                 { return false }
func (l *limitedStorage) SignedURL(_ context.Context, f *grapi.UploadFile) (string, error) {
	return f.URL, nil
}

func (l *limitedStorage) CheckFileSize(f *grapi.UploadFile, _ int64) error {
	l.checked = f.Size
	if f.Size > 10 {
		return errors.New("too big for this storage")
	}
	return nil
}

func TestProviderCheckFileSize(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config", "plugins.yaml"), []byte("upload:\n  config:\n    provider: limited\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	app := grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}})
	storage := &limitedStorage{}
	app.UseUploadProvider("limited", storage)
	srv, err := app.Setup()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	ctx := context.Background()
	e := app.Documents().E
	public, _ := e.FindOne(ctx, "plugin::users-permissions.role", query.Params{Where: map[string]any{"type": "public"}})
	roleID, _ := public.Get("id")
	if _, err := e.Create(ctx, "plugin::users-permissions.permission", query.Data{"action": "plugin::upload.content-api.upload", "role": roleID}, query.Params{}); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()
	body, ct := pngBody(t, 64, 64, "")
	res, err := http.Post(ts.URL+"/api/upload", ct, body)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if storage.checked == 0 || res.StatusCode != 500 || strings.Contains(string(b), "cdn.example.com") {
		t.Fatalf("the provider's check rejects the file: %d %s (checked %d)", res.StatusCode, b, storage.checked)
	}
}
