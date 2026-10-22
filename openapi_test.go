package grapi_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/aluzed/grapi"
)

func TestOpenAPIRoute(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o750); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"config/server.yaml": "openapi:\n  content-api:\n    access: public\n",
		"package.json":       `{"name":"my-app","version":"1.2.3"}`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	app := grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}})
	app.Route("POST", "/accounts/:id/anonymize", "api::account.account.anonymize", func(*grapi.Request) error { return nil }).
		OpenAPI(map[string]any{"operationId": "account/post/accounts_by_id_anonymize", "tags": []any{"account"}, "responses": map[string]any{"200": map[string]any{"description": "OK"}}})
	srv, err := app.Setup()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()
	res, err := http.Get(ts.URL + "/api/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var doc struct {
		OpenAPI string                    `json:"openapi"`
		Info    map[string]string         `json:"info"`
		Paths   map[string]map[string]any `json:"paths"`
	}
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil || res.StatusCode != 200 {
		t.Fatalf("%d %v", res.StatusCode, err)
	}
	if doc.OpenAPI != "3.1.0" || doc.Info["title"] != "my-app" || doc.Info["version"] != "1.2.3" {
		t.Errorf("head: %v %v", doc.OpenAPI, doc.Info)
	}
	for _, p := range []string{"/articles", "/articles/{id}", "/auth/local", "/users/{id}", "/upload/files", "/content-type-builder/content-types"} {
		if doc.Paths[p] == nil {
			t.Errorf("missing %s", p)
		}
	}
	if op, ok := doc.Paths["/accounts/{id}/anonymize"]["post"].(map[string]any); !ok || op["operationId"] != "account/post/accounts_by_id_anonymize" {
		t.Errorf("custom route: %v", doc.Paths["/accounts/{id}/anonymize"])
	}
	// OAuth providers are not served by grapi yet.
	if doc.Paths["/auth/{provider}/callback"] != nil {
		t.Error("routes grapi does not serve are listed")
	}
}
