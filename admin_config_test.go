package grapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/aluzed/grapi"
	"github.com/aluzed/grapi/internal/project"
)

// A schema change through the content type builder keeps the view
// configuration: a new field goes at the end of the edit layout, a renamed
// one keeps its label and description, the other settings stay.
func TestAdminConfigurationFollowsSchema(t *testing.T) {
	ts, _, stop, dir := newAdminApp(t, "basic", true)
	defer stop()
	c := adminLogin(t, ts.URL)
	const path = "/content-manager/configuration/api::article.article"
	_, b, _ := c.do("GET", path, "")
	cfg := dataOf(b)
	cfg["settings"].(map[string]any)["pageSize"] = 50
	views := cfg["metadatas"].(map[string]any)["views"].(map[string]any)
	views["edit"].(map[string]any)["label"] = "Readers"
	views["edit"].(map[string]any)["description"] = "How many read it"
	views["list"].(map[string]any)["label"] = "Readers"
	body, _ := json.Marshal(cfg)
	if s, b, _ := c.do("PUT", path, string(body)); s != 200 {
		t.Fatalf("configure: %d %v", s, b)
	}

	_, b, _ = c.do("GET", "/content-type-builder/schema", "")
	var article map[string]any
	for _, x := range dataOf(b)["contentTypes"].([]any) {
		if x.(map[string]any)["uid"] == "api::article.article" {
			article = x.(map[string]any)
		}
	}
	attrs := article["attributes"].([]any)
	for _, a := range attrs {
		if m := a.(map[string]any); m["name"] == "views" {
			m["name"], m["renamedFrom"] = "readers", "views"
		}
	}
	article["attributes"] = append(attrs, map[string]any{"name": "subtitle", "type": "string"})
	batch, _ := json.Marshal(map[string]any{"contentTypes": []any{map[string]any{"action": "update", "uid": "api::article.article", "data": article}}})
	if s, b, _ := c.do("POST", "/content-type-builder/update-schema", string(batch)); s != 200 {
		t.Fatalf("update schema: %d %v", s, b)
	}
	app := grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}, AutoReload: true})
	srv, err := app.Setup()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	ts2 := httptest.NewServer(srv.Handler)
	defer func() { ts2.Close(); _ = srv.Close() }()
	c2 := newAdminClient(t, ts2.URL)
	_, b, _ = c2.do("POST", "/auth/login", `{"email":"ada@example.com","password":"Passw0rd"}`)
	c2.token = dataOf(b)["token"].(string)
	_, b, _ = c2.do("GET", path, "")
	cfg = dataOf(b)
	if cfg["settings"].(map[string]any)["pageSize"] != float64(50) {
		t.Fatalf("settings kept: %v", cfg["settings"])
	}
	readers, _ := cfg["metadatas"].(map[string]any)["readers"].(map[string]any)
	if readers == nil || readers["edit"].(map[string]any)["label"] != "Readers" || readers["edit"].(map[string]any)["description"] != "How many read it" {
		t.Fatalf("the renamed field keeps its metadata: %v", readers)
	}
	edit := cfg["layouts"].(map[string]any)["edit"].([]any)
	last := edit[len(edit)-1].([]any)
	if last[len(last)-1].(map[string]any)["name"] != "subtitle" {
		t.Fatalf("the new field ends the edit layout: %v", edit)
	}
	raw, _ := json.Marshal(cfg)
	if bytes.Contains(raw, []byte(`"views"`)) {
		t.Fatalf("the old name is gone: %s", raw)
	}
}

func TestAdminConfigurationValidation(t *testing.T) {
	ts, _, stop := newAdminProject(t)
	defer stop()
	c := adminLogin(t, ts.URL)
	const path = "/content-manager/configuration/api::article.article"
	for name, edit := range map[string]any{
		"unknown field": []any{[]any{map[string]any{"name": "nope", "size": 6}}},
		"size 5":        []any{[]any{map[string]any{"name": "title", "size": 5}}},
		"size 13":       []any{[]any{map[string]any{"name": "title", "size": 13}}},
		"row over 12":   []any{[]any{map[string]any{"name": "title", "size": 8}, map[string]any{"name": "views", "size": 6}}},
	} {
		_, b, _ := c.do("GET", path, "")
		cfg := dataOf(b)
		cfg["layouts"].(map[string]any)["edit"] = edit
		body, _ := json.Marshal(cfg)
		if s, b, _ := c.do("PUT", path, string(body)); s != 400 || b["error"].(map[string]any)["name"] != "ValidationError" {
			t.Errorf("%s: %d %v", name, s, b)
		}
	}
	_, b, _ := c.do("GET", path, "")
	cfg := dataOf(b)
	cfg["layouts"].(map[string]any)["list"] = []any{"id", "nope"}
	body, _ := json.Marshal(cfg)
	if s, _, _ := c.do("PUT", path, string(body)); s != 400 {
		t.Errorf("unknown list column: %d", s)
	}
}

// A view configuration as Strapi stores it (its own export, then edited the
// way its admin saves) is served as is, and grapi does not rewrite it.
func TestAdminReadsStrapiConfiguration(t *testing.T) {
	ts, _, stop, dir := newAdminApp(t, "basic", false)
	adminLogin(t, ts.URL)
	stop()
	cfg := strapiViewConfigurations(t)["api::article.article"]
	// Edited the way Strapi's admin saves a configuration.
	settings := cfg["settings"].(map[string]any)
	settings["pageSize"], settings["mainField"], settings["defaultSortBy"], settings["defaultSortOrder"] = 37, "slug", "views", "DESC"
	layouts := cfg["layouts"].(map[string]any)
	layouts["list"] = []any{"id", "views", "title"}
	layouts["edit"] = []any{[]any{map[string]any{"name": "views", "size": 4}, map[string]any{"name": "title", "size": 8}}, []any{map[string]any{"name": "summary", "size": 6}}}
	metas := cfg["metadatas"].(map[string]any)
	metas["title"].(map[string]any)["edit"].(map[string]any)["label"] = "Headline"
	metas["views"].(map[string]any)["edit"].(map[string]any)["description"] = "Counted by the front"
	stored, _ := json.Marshal(cfg)
	var compact bytes.Buffer
	if err := json.Compact(&compact, stored); err != nil {
		t.Fatal(err)
	}
	const key = "plugin_content_manager_configuration_content_types::api::article.article"
	p, err := project.Load(dir, "", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	q, ph := p.DB.Dialect.Quote, p.DB.Dialect.Placeholder
	if _, err := p.DB.Exec(ctx, fmt.Sprintf("insert into %s (%s, %s, %s) values (%s, %s, %s)", q("strapi_core_store_settings"), q("key"), q("value"), q("type"), ph(1), ph(2), ph(3)), key, compact.String(), "object"); err != nil {
		t.Fatal(err)
	}
	_ = p.DB.Close()

	app := grapi.New(grapi.Options{Dir: dir, Stderr: io.Discard})
	srv, err := app.Setup()
	if err != nil {
		t.Fatal(err)
	}
	ts2 := httptest.NewServer(srv.Handler)
	defer func() { ts2.Close(); _ = srv.Close() }()
	c := newAdminClient(t, ts2.URL)
	_, b, _ := c.do("POST", "/auth/login", `{"email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	_, b, _ = c.do("GET", "/content-manager/configuration/api::article.article", "")
	var want, got map[string]any
	_ = json.Unmarshal(stored, &want)
	raw, _ := json.Marshal(dataOf(b))
	_ = json.Unmarshal(raw, &got)
	for _, part := range []string{"settings", "layouts", "metadatas"} {
		w, _ := json.Marshal(want[part])
		g, _ := json.Marshal(got[part])
		if !bytes.Equal(w, g) {
			t.Errorf("%s: stored %s, served %s", part, w, g)
		}
	}

	p, err = project.Load(dir, "", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.DB.Close() }()
	var after string
	rows, err := p.DB.Query(ctx, fmt.Sprintf("select %s from %s where %s = %s", q("value"), q("strapi_core_store_settings"), q("key"), ph(1)), key)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		_ = rows.Scan(&after)
	}
	_ = rows.Close()
	if after != compact.String() {
		t.Errorf("the stored configuration was rewritten:\n%s\n%s", compact.String(), after)
	}
}

// strapiViewConfigurations are the configurations Strapi 5.59 stored for
// the basic fixture's types, from its core store export.
func strapiViewConfigurations(t *testing.T) map[string]map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("compat", "fixtures", "basic", "strapi-view-configurations.json"))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// The configuration grapi creates for a type is the one Strapi creates.
func TestAdminDefaultConfigurationIsStrapis(t *testing.T) {
	ts, _, stop := newAdminProject(t)
	defer stop()
	c := adminLogin(t, ts.URL)
	for uid, want := range strapiViewConfigurations(t) {
		_, b, _ := c.do("GET", "/content-manager/configuration/"+uid, "")
		for _, part := range []string{"settings", "layouts", "metadatas"} {
			w, _ := json.Marshal(want[part])
			g, _ := json.Marshal(dataOf(b)[part])
			if !bytes.Equal(w, g) {
				t.Errorf("%s %s:\nStrapi %s\ngrapi  %s", uid, part, w, g)
			}
		}
	}
}
