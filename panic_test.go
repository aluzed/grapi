package grapi_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aluzed/grapi"
)

func TestPanicGivesA500(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	var logs bytes.Buffer
	app := grapi.New(grapi.Options{Dir: dir, Stderr: &logs})
	app.PublicRoute("GET", "/boom", func(*grapi.Request) error { panic("secret internal state") })
	srv, err := app.Setup()
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler)
	defer func() { ts.Close(); _ = srv.Close() }()
	res, err := http.Get(ts.URL + "/api/boom")
	if err != nil {
		t.Fatalf("the connection must not be cut: %v", err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	want := `{"data":null,"error":{"message":"Internal Server Error","name":"InternalServerError","status":500}}`
	if res.StatusCode != 500 || strings.TrimSpace(string(body)) != want {
		t.Fatalf("%d %s", res.StatusCode, body)
	}
	if strings.Contains(string(body), "secret") || strings.Contains(string(body), "goroutine") {
		t.Fatal("no panic value or stack in the response")
	}
	if !strings.Contains(logs.String(), "secret internal state") {
		t.Fatal("the panic is logged")
	}
	if res, _ := http.Get(ts.URL + "/api/articles"); res == nil || res.StatusCode == 500 {
		t.Fatal("the server keeps serving")
	}
}
