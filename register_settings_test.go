package grapi_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/aluzed/grapi"
)

// setAdvanced changes users-permissions' advanced settings in the core
// store, as the admin page does.
func setAdvanced(t *testing.T, dbFile string, change func(map[string]any)) {
	t.Helper()
	d, err := sql.Open("sqlite", "file:"+dbFile+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	var raw string
	if err := d.QueryRow("select value from strapi_core_store_settings where key = 'plugin_users-permissions_advanced'").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(err)
	}
	change(v)
	out, _ := json.Marshal(v)
	if _, err := d.Exec("update strapi_core_store_settings set value = ? where key = 'plugin_users-permissions_advanced'", string(out)); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterSettings(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "basic", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	dbFile := filepath.Join(dir, "data.db")
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", dbFile)
	t.Setenv("JWT_SECRET", "test")
	srv, err := grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}}).Setup()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()
	c := &upClient{t: t, base: ts.URL, http: ts.Client()}
	register := func(username, email string) (int, string) {
		s, b := c.do("POST", "/api/auth/local/register", "", `{"username":"`+username+`","email":"`+email+`","password":"Passw0rd!"}`)
		return s, errorMessage(b)
	}

	if s, msg := register("ada", "ada@example.com"); s != 200 {
		t.Fatalf("register: %d %s", s, msg)
	}
	// unique_email is on by default: the same e-mail with another username.
	if s, msg := register("ada2", "ADA@example.com"); s != 400 || msg != "Email or Username are already taken" {
		t.Fatalf("unique_email: %d %q", s, msg)
	}
	// Strapi's first check matches the e-mail and the username either way,
	// so unique_email off does not free a taken e-mail.
	setAdvanced(t, dbFile, func(v map[string]any) { v["unique_email"] = false })
	if s, msg := register("ada3", "ada@example.com"); s != 400 || msg != "Email or Username are already taken" {
		t.Fatalf("unique_email off: %d %q", s, msg)
	}
	setAdvanced(t, dbFile, func(v map[string]any) { v["allow_register"] = false })
	// Strapi throws an ApplicationError: 400, not 403.
	if s, msg := register("bob", "bob@example.com"); s != 400 || msg != "Register action is currently disabled" {
		t.Fatalf("allow_register off: %d %q", s, msg)
	}
}
