package grapi_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aluzed/grapi"
	"github.com/aluzed/grapi/document"
)

func TestSanitizers(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	app := grapi.New(grapi.Options{Dir: dir, Stderr: io.Discard})
	app.AddSanitizer("content-api.input", func(uid string, entry map[string]any) map[string]any {
		if uid == "api::tag.tag" {
			if name, ok := entry["name"].(string); ok {
				entry["name"] = strings.TrimSpace(name)
			}
		}
		return entry
	})
	app.AddSanitizer("content-api.output", func(uid string, entry map[string]any) map[string]any {
		if uid == "api::tag.tag" {
			delete(entry, "createdAt")
			entry["source"] = "grapi"
		}
		return entry
	})
	app.OnBootstrap(func(ctx context.Context) error {
		role, err := app.Documents().FindFirst(ctx, "plugin::users-permissions.role", document.Params{"filters": map[string]any{"type": "public"}})
		if err != nil || role == nil {
			return err
		}
		id, _ := role.Get("id")
		for _, action := range []string{"api::tag.tag.find", "api::tag.tag.create"} {
			if _, err := app.Documents().Create(ctx, "plugin::users-permissions.permission", document.Params{"data": map[string]any{"action": action, "role": id}}); err != nil {
				return err
			}
		}
		return nil
	})
	srv, err := app.Setup()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()
	c := &upClient{t: t, base: ts.URL, http: ts.Client()}
	s, b := c.do("POST", "/api/tags", "", `{"data":{"name":"  retro  "}}`)
	data, _ := b["data"].(map[string]any)
	if s != 201 || data["name"] != "retro" || data["source"] != "grapi" || data["createdAt"] != nil {
		t.Fatalf("create: %d %v", s, b)
	}
	_, b = c.do("GET", "/api/tags", "", "")
	raw, _ := json.Marshal(b["data"])
	if !strings.Contains(string(raw), `"source":"grapi"`) || strings.Contains(string(raw), "createdAt") {
		t.Fatalf("each entry of a list goes through the output sanitizers: %s", raw)
	}
}
