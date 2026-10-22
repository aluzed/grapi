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
