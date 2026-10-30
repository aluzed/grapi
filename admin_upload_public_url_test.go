package grapi_test

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/aluzed/grapi"
	"github.com/aluzed/grapi/admin"
)

func TestAdminUploadPublicURL(t *testing.T) {
	prev := admin.FS
	admin.FS = fstest.MapFS{"index.html": {Data: []byte("<html><head></head></html>")}}
	defer func() { admin.FS = prev }()
	ts, _, stop, _ := newAdminAppWith(t, "basic", false, func(_ *grapi.App, dir string) {
		conf := "email:\n  config:\n    provider: capture\nupload:\n  config:\n    publicUrl: https://media.example.com/\n"
		if err := os.WriteFile(filepath.Join(dir, "config", "plugins.yaml"), []byte(conf), 0o600); err != nil {
			t.Fatal(err)
		}
	})
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)
	meta := func() string {
		res, err := http.Get(ts.URL + "/admin/content-manager")
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		_, after, ok := strings.Cut(string(body), `name="grapi-upload-url" content="`)
		if !ok {
			return ""
		}
		v, _, _ := strings.Cut(after, `"`)
		return v
	}
	if got := meta(); got != "https://media.example.com" {
		t.Fatalf("config default: %q", got)
	}
	if s, b, _ := c.do("GET", "/upload/settings", ""); s != 200 || dataOf(b)["publicUrl"] != "" || dataOf(b)["publicUrlDefault"] != "https://media.example.com" {
		t.Fatalf("settings: %d %v", s, b)
	}
	if s, b, _ := c.do("PUT", "/upload/settings", `{"sizeOptimization":true,"responsiveDimensions":true,"autoOrientation":false,"publicUrl":"https://cdn.example.com/"}`); s != 200 || dataOf(b)["publicUrl"] != "https://cdn.example.com" {
		t.Fatalf("set: %d %v", s, b)
	}
	if got := meta(); got != "https://cdn.example.com" {
		t.Fatalf("the admin setting wins: %q", got)
	}
	for _, bad := range []string{"cdn.example.com", "javascript:alert(1)", "https://cdn.example.com/?x=1", "https://u:p@cdn.example.com"} {
		if s, _, _ := c.do("PUT", "/upload/settings", `{"sizeOptimization":true,"responsiveDimensions":true,"autoOrientation":false,"publicUrl":"`+bad+`"}`); s != 400 {
			t.Fatalf("%s: %d", bad, s)
		}
	}
	if s, _, _ := c.do("PUT", "/upload/settings", `{"sizeOptimization":true,"responsiveDimensions":true,"autoOrientation":false}`); s != 200 || meta() != "https://cdn.example.com" {
		t.Fatalf("a body without publicUrl keeps it: %d %q", s, meta())
	}
	if s, _, _ := c.do("PUT", "/upload/settings", `{"sizeOptimization":true,"responsiveDimensions":true,"autoOrientation":false,"publicUrl":""}`); s != 200 || meta() != "https://media.example.com" {
		t.Fatalf("empty falls back to the config: %d %q", s, meta())
	}
}
