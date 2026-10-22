package grapi_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aluzed/grapi"
)

func TestRequestLog(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	var logs bytes.Buffer
	srv, err := grapi.New(grapi.Options{Dir: dir, Stderr: &logs}).Setup()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/api/nope?x=1")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	generated := res.Header.Get("X-Request-Id")
	if len(generated) != 32 {
		t.Fatalf("generated request id %q", generated)
	}
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/_health", nil)
	req.Header.Set("X-Request-Id", "proxy-42")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if got := res.Header.Get("X-Request-Id"); got != "proxy-42" {
		t.Fatalf("request id %q is not kept", got)
	}
	out := logs.String()
	for _, want := range []string{
		`level=HTTP msg="GET /api/nope?x=1 (`, `ms) 404"`, "method=GET path=/api/nope status=404", "request_id=" + generated,
		`msg="GET /_health (`, "request_id=proxy-42",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
}
