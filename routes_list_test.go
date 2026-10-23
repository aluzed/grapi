package grapi_test

import (
	"io"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/aluzed/grapi"
)

func TestAppRoutesListsGoRoutes(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	app := grapi.New(grapi.Options{Dir: dir, Stderr: io.Discard})
	app.Policy("global::is-editor", func(*grapi.PolicyContext, any) (bool, error) { return true, nil })
	app.Route("GET", "/articles/popular", "api::article.article.popular", func(c *grapi.Request) error {
		return c.JSON(http.StatusOK, map[string]any{"data": nil})
	}).Policies("global::is-editor")
	app.CoreRouter("api::tag.tag").Only("find")
	routes, err := app.Routes()
	if err != nil {
		t.Fatal(err)
	}
	has := func(method, path string) bool {
		for _, r := range routes {
			if r.Method == method && r.Path == path {
				return true
			}
		}
		return false
	}
	if !has("GET", "/api/articles/popular") {
		t.Errorf("the Go route is listed: %v", routes)
	}
	for _, r := range routes {
		if r.Path == "/api/articles/popular" && (r.Action != "api::article.article.popular" || len(r.Policies) != 1 || r.Policies[0] != "global::is-editor") {
			t.Errorf("handler and policies: %+v", r)
		}
	}
	if !has("GET", "/api/tags") || has("POST", "/api/tags") {
		t.Errorf("the core router keeps find only: %v", routes)
	}
}
