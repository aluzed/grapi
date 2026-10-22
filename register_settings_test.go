package grapi_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aluzed/grapi"
	"github.com/aluzed/grapi/internal/db/store"
)

func TestRegisterSettings(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	srv, err := grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}}).Setup()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()
	p, err := grapi.LoadProject(dir, "", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.DB.Close() }()
	st := store.Store{DB: p.DB}
	scope := store.Scope{Type: "plugin", Name: "users-permissions"}
	setAdvanced := func(k string, v any) {
		var adv map[string]any
		if _, err := st.Get(context.Background(), scope, "advanced", &adv); err != nil {
			t.Fatal(err)
		}
		adv[k] = v
		if err := st.Set(context.Background(), scope, "advanced", adv); err != nil {
			t.Fatal(err)
		}
	}
	register := func(user string) (int, string) {
		res, err := http.Post(ts.URL+"/api/auth/local/register", "application/json", strings.NewReader(`{"username":"`+user+`","email":"same@example.com","password":"Passw0rd!"}`))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}
	setAdvanced("allow_register", false)
	if s, b := register("ada"); s != 400 || !strings.Contains(b, "Register action is currently disabled") {
		t.Fatalf("allow_register false: %d %s", s, b)
	}
	setAdvanced("allow_register", true)
	setAdvanced("unique_email", false)
	if s, b := register("ada"); s != 200 {
		t.Fatalf("first: %d %s", s, b)
	}
	// As in Strapi, the same email is refused within one provider even
	// without unique_email, which only adds the check across providers.
	if s, b := register("bob"); s != 400 || !strings.Contains(b, "Email or Username are already taken") {
		t.Fatalf("same provider, same email: %d %s", s, b)
	}
}
