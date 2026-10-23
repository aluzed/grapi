package grapi_test

import (
	"bytes"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/aluzed/grapi"
)

const ctbShowcase = `{"contentTypes":[{"action":"create","data":{"kind":"collectionType",
	"info":{"displayName":"Showcase","singularName":"showcase","pluralName":"showcases"},"options":{"draftAndPublish":true},
	"attributes":[
		{"name":"title","type":"string","required":true,"maxLength":120},
		{"name":"slug","type":"uid","targetField":"title"},
		{"name":"summary","type":"text"},
		{"name":"body","type":"richtext"},
		{"name":"content","type":"blocks"},
		{"name":"contact","type":"email"},
		{"name":"secret","type":"password","private":true},
		{"name":"stage","type":"enumeration","enum":["draft","review","done"],"default":"draft"},
		{"name":"views","type":"integer","min":0},
		{"name":"bigViews","type":"biginteger"},
		{"name":"rating","type":"float"},
		{"name":"price","type":"decimal"},
		{"name":"releaseDate","type":"date"},
		{"name":"publishAt","type":"datetime"},
		{"name":"opensAt","type":"time"},
		{"name":"featured","type":"boolean","default":false},
		{"name":"settings","type":"json"},
		{"name":"cover","type":"media","multiple":false,"allowedTypes":["images"]},
		{"name":"gallery","type":"media","multiple":true,"allowedTypes":["images","videos"]},
		{"name":"mainTag","type":"relation","relation":"oneToOne","target":"api::tag.tag"},
		{"name":"tags","type":"relation","relation":"manyToMany","target":"api::tag.tag","targetAttribute":"showcases"},
		{"name":"notes","type":"component","component":"shared.note","repeatable":true},
		{"name":"sections","type":"dynamiczone","components":["shared.note"]}
	]}}]}`

// The content type builder writes, under develop, a type with every kind
// of field, relations and a component; the routes come up without a
// restart, and the files are the ctb fixture the bench runs Strapi on.
func TestContentTypeBuilderFixture(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "src", "api"), 0o750); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	t.Setenv("ADMIN_JWT_SECRET", "admin-test")
	t.Setenv("HOST", "127.0.0.1")
	t.Setenv("PORT", "0")
	app := grapi.New(grapi.Options{Dir: dir, Stderr: io.Discard})
	addr := make(chan string, 1)
	done := make(chan error, 1)
	go func() { done <- app.Develop(func(a string) { addr <- a }) }()
	var base string
	select {
	case a := <-addr:
		base = "http://" + strings.TrimPrefix(a, "http://")
	case err := <-done:
		t.Fatalf("develop: %v", err)
	}
	defer func() {
		_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
		<-done
	}()
	routeUp := func(path string) bool {
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			if res, err := http.Get(base + path); err == nil {
				_ = res.Body.Close()
				if res.StatusCode != http.StatusNotFound && res.StatusCode != http.StatusServiceUnavailable {
					return true
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
		return false
	}
	c := adminLogin(t, base)
	batch := `{"components":[{"action":"create","data":{"category":"shared","info":{"displayName":"Note"},"attributes":[{"name":"heading","type":"string"},{"name":"text","type":"text"}]}}],
		"contentTypes":[{"action":"create","data":{"kind":"collectionType","info":{"displayName":"Tag","singularName":"tag","pluralName":"tags"},"options":{"draftAndPublish":false},
		"attributes":[{"name":"name","type":"string","required":true}]}}]}`
	if s, b, _ := c.do("POST", "/content-type-builder/update-schema", batch); s != 200 {
		t.Fatalf("tag and note: %d %v", s, b)
	}
	if !routeUp("/api/tags") {
		t.Fatal("/api/tags is not served after the reload")
	}
	if s, b, _ := c.do("POST", "/content-type-builder/update-schema", ctbShowcase); s != 200 {
		t.Fatalf("showcase: %d %v", s, b)
	}
	if !routeUp("/api/showcases") {
		t.Fatal("/api/showcases is not served after the reload")
	}

	fixture := filepath.Join("compat", "fixtures", "ctb", "src")
	written := map[string][]byte{}
	_ = filepath.WalkDir(filepath.Join(dir, "src"), func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(filepath.Join(dir, "src"), path)
			written[rel], _ = os.ReadFile(path) //nolint:gosec // the test's own directory
		}
		return nil
	})
	if os.Getenv("GRAPI_UPDATE_FIXTURE") == "1" {
		_ = os.RemoveAll(fixture)
		for rel, b := range written {
			_ = os.MkdirAll(filepath.Dir(filepath.Join(fixture, rel)), 0o750)
			if err := os.WriteFile(filepath.Join(fixture, rel), b, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	kept := 0
	_ = filepath.WalkDir(fixture, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			kept++
		}
		return nil
	})
	if kept != len(written) {
		t.Errorf("the builder wrote %d files, the fixture has %d (GRAPI_UPDATE_FIXTURE=1 rewrites it)", len(written), kept)
	}
	for rel, b := range written {
		want, err := os.ReadFile(filepath.Join(fixture, rel)) //nolint:gosec // the fixture
		if err != nil || !bytes.Equal(want, b) {
			t.Errorf("%s differs from the fixture (GRAPI_UPDATE_FIXTURE=1 rewrites it):\n%s", rel, b)
		}
	}
}
