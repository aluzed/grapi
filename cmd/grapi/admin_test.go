package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aluzed/grapi"
)

func TestAdminCommands(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	var out bytes.Buffer
	if code := run([]string{"new", "demo"}, &out, &out); code != 0 {
		t.Fatalf("new: %s", out.String())
	}
	project := filepath.Join(dir, "demo")
	out.Reset()
	if code := run([]string{"admin:create-user", "-dir", project, "--email", "ops@example.com", "--firstname", "Ops", "--password", "Passw0rd"}, &out, &out); code == 0 {
		t.Fatal("the roles do not exist before the first start")
	}
	srv, err := grapi.New(grapi.Options{Dir: project, Stderr: &out}).Setup()
	if err != nil {
		t.Fatal(err)
	}
	_ = srv.Close()
	out.Reset()
	if code := run([]string{"admin:create-user", "-dir", project, "--email", "Ops@Example.com", "--firstname", "Ops", "--password", "weak"}, &out, &out); code == 0 {
		t.Fatal("the password policy applies")
	}
	if code := run([]string{"admin:create-user", "-dir", project, "--email", "Ops@Example.com", "--firstname", "Ops", "--password", "Passw0rd"}, &out, &out); code != 0 {
		t.Fatalf("create-user: %s", out.String())
	}
	srv, err = grapi.New(grapi.Options{Dir: project, Stderr: &out}).Setup()
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.Handler)
	defer func() { ts.Close(); _ = srv.Close() }()
	login := func(pw string) int {
		res, err := http.Post(ts.URL+"/admin/api/v1/auth/login", "application/json", strings.NewReader(`{"email":"ops@example.com","password":"`+pw+`"}`))
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		return res.StatusCode
	}
	if c := login("Passw0rd"); c != 200 {
		t.Fatalf("login: %d", c)
	}
	out.Reset()
	if code := run([]string{"admin:reset-user-password", "-dir", project, "--email", "ops@example.com", "--password", "N3wPassword"}, &out, &out); code != 0 {
		t.Fatalf("reset: %s", out.String())
	}
	if login("Passw0rd") != 400 || login("N3wPassword") != 200 {
		t.Fatal("the new password replaces the old one")
	}
	if strings.Contains(out.String(), "warning") {
		t.Fatalf("the CLI writes the audit log: %s", out.String())
	}
	res, err := http.Post(ts.URL+"/admin/api/v1/auth/login", "application/json", strings.NewReader(`{"email":"ops@example.com","password":"N3wPassword"}`))
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Data struct{ Token string } `json:"data"`
	}
	_ = json.NewDecoder(res.Body).Decode(&body)
	_ = res.Body.Close()
	req, _ := http.NewRequest("GET", ts.URL+"/admin/api/v1/audit-logs?origin=cli", nil)
	req.Header.Set("Authorization", "Bearer "+body.Data.Token)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var logs struct {
		Data []struct{ Action, Origin string } `json:"data"`
	}
	_ = json.NewDecoder(res.Body).Decode(&logs)
	_ = res.Body.Close()
	if len(logs.Data) != 2 || logs.Data[0].Action != "user.password.update" || logs.Data[1].Action != "user.create" {
		t.Fatalf("CLI audit: %+v", logs.Data)
	}
	out.Reset()
	run([]string{"admin:list-users", "-dir", project}, &out, &out)
	if !strings.Contains(out.String(), "ops@example.com") || !strings.Contains(out.String(), "Super Admin") {
		t.Fatalf("list-users: %s", out.String())
	}
}
