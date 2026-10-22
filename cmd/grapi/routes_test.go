package main

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoutesList(t *testing.T) {
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(t.TempDir(), "data.db"))
	var out bytes.Buffer
	if err := routesList([]string{"-dir", filepath.Join("..", "..", "compat", "fixtures", "basic")}, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	for _, want := range []string{"│ Method   │ Path", "│ HEAD|GET │ /api/articles/:id", "│ DELETE   │ /api/users/:id", "│ POST     │ /api/auth/local "} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in\n%s", want, s)
		}
	}
}

func TestListCommands(t *testing.T) {
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(t.TempDir(), "data.db"))
	var out bytes.Buffer
	dir := filepath.Join("..", "..", "compat", "fixtures", "components")
	if err := listCommand("content-types", []string{"-dir", dir}, &out, io.Discard); err != nil || !strings.Contains(out.String(), "│ api::page.page ") {
		t.Fatalf("content-types: %v %s", err, out.String())
	}
	out.Reset()
	if err := listCommand("components", []string{"-dir", dir}, &out, io.Discard); err != nil || !strings.Contains(out.String(), "│ shared.seo ") || !strings.Contains(out.String(), "│ Name ") {
		t.Fatalf("components: %v %s", err, out.String())
	}
}

func TestStaticLists(t *testing.T) {
	var out bytes.Buffer
	if err := staticList("middlewares", &out); err != nil || !strings.Contains(out.String(), "│ strapi::body ") {
		t.Fatalf("%v %s", err, out.String())
	}
	out.Reset()
	if err := staticList("hooks", &out); err != nil || !strings.Contains(out.String(), "│ strapi::content-types.afterSync ") {
		t.Fatalf("%v %s", err, out.String())
	}
}
