package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aluzed/grapi"
)

func TestGenerate(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	var out bytes.Buffer
	if code := run([]string{"new", "demo"}, &out, &out); code != 0 {
		t.Fatalf("new: %s", out.String())
	}
	project := filepath.Join(dir, "demo")
	gen := func(args ...string) (int, string) {
		out.Reset()
		code := run(append([]string{"generate"}, append(args, "-dir", project)...), &out, &out)
		return code, out.String()
	}
	if c, o := gen("component", "--name", "shared.seo", "--attr", "metaTitle:string:required"); c != 0 {
		t.Fatalf("component: %s", o)
	}
	if c, o := gen("content-type", "--name", "note", "--attr", "text:string:required", "--attr", "views:integer"); c != 0 {
		t.Fatalf("content type: %s", o)
	}
	if c, o := gen("content-type", "--name", "bad", "--attr", "locale:string"); c == 0 || !strings.Contains(o, "reserved") {
		t.Fatalf("reserved name: %d %s", c, o)
	}
	if _, err := os.Stat(filepath.Join(project, "src/api/bad/content-types/bad/schema.json")); err == nil {
		t.Fatal("nothing is written for a reserved name")
	}
	if c, o := gen("content-type", "--name", "note", "--plural", "notes-bis"); c == 0 || !strings.Contains(o, "already exists") {
		t.Fatalf("existing schema: %d %s", c, o)
	}
	if c, o := gen("migration", "--name", "seed-notes"); c != 0 || !strings.Contains(o, ".seed-notes.sql") {
		t.Fatalf("migration: %s", o)
	}
	srv, err := grapi.New(grapi.Options{Dir: project, Stderr: &out}).Setup()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	if code := run([]string{"permissions:grant", "public", "api::note.note.find", "-dir", project}, &out, &out); code != 0 {
		t.Fatalf("grant: %s", out.String())
	}
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()
	if code := run([]string{"db:check", "-dir", project}, &out, &out); code != 0 {
		t.Fatalf("db:check after the generated migration ran: %s", out.String())
	}
	res, err := http.Get(ts.URL + "/api/notes")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("the generated content type is served: %d", res.StatusCode)
	}
}
