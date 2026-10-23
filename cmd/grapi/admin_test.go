package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
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
	// The test logs in more often than admin.rateLimit allows.
	f, err := os.OpenFile(filepath.Join(project, "config", "admin.yaml"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("rateLimit:\n  enabled: false\n")
	_ = f.Close()
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
	cli := func(args ...string) (int, string) {
		var o bytes.Buffer
		code := run(append(args, "-dir", project), &o, &o)
		return code, o.String()
	}
	if code, msg := cli("admin:block-user", "--email", "ops@example.com"); code == 0 || !strings.Contains(msg, "last active Super Admin") {
		t.Fatalf("the last Super Admin cannot be blocked: %d %s", code, msg)
	}
	if code, msg := cli("admin:create-user", "--email", "ops2@example.com", "--firstname", "Ops", "--password", "Passw0rd"); code != 0 {
		t.Fatalf("second admin: %s", msg)
	}
	if code, msg := cli("admin:block-user", "--email", "ops@example.com"); code != 0 {
		t.Fatalf("block: %s", msg)
	}
	if login("N3wPassword") == 200 {
		t.Fatal("a blocked admin cannot log in")
	}
	if code, _ := cli("admin:active-user", "--email", "ops2@example.com", "--active=false"); code == 0 {
		t.Fatal("ops2 is now the last active Super Admin")
	}
	if code, msg := cli("admin:block-user", "--email", "ops@example.com", "--unblock"); code != 0 || login("N3wPassword") != 200 {
		t.Fatalf("unblock: %s", msg)
	}
	if code, msg := cli("admin:delete-user", "--email", "ops2@example.com"); code != 0 {
		t.Fatalf("delete: %s", msg)
	}
	out.Reset()
	run([]string{"admin:list-users", "-dir", project}, &out, &out)
	if strings.Contains(out.String(), "ops2@") {
		t.Fatalf("deleted: %s", out.String())
	}
	if !strings.Contains(out.String(), "ops@example.com") || !strings.Contains(out.String(), "Super Admin") {
		t.Fatalf("list-users: %s", out.String())
	}
}

func TestPasswordNeverPrinted(t *testing.T) {
	var prompt bytes.Buffer
	pw, err := readPassword("", false, strings.NewReader("S3cret-from-pipe\n"), &prompt)
	if err != nil || pw != "" || prompt.Len() != 0 {
		t.Fatalf("no terminal and no flag: no prompt, no password: %q %q %v", pw, prompt.String(), err)
	}
	if pw, _ := readPassword("", true, strings.NewReader("S3cret-from-pipe\r\n"), &prompt); pw != "S3cret-from-pipe" || prompt.Len() != 0 {
		t.Fatalf("--password-stdin: %q, prompt %q", pw, prompt.String())
	}

	dir := t.TempDir()
	t.Chdir(dir)
	var out bytes.Buffer
	if code := run([]string{"new", "demo"}, &out, &out); code != 0 {
		t.Fatalf("new: %s", out.String())
	}
	project := filepath.Join(dir, "demo")
	srv, err := grapi.New(grapi.Options{Dir: project, Stderr: &out}).Setup()
	if err != nil {
		t.Fatal(err)
	}
	_ = srv.Close()
	const secret = "Very-Secret-9"
	out.Reset()
	for _, args := range [][]string{
		{"admin:create-user", "--email", "ops@example.com", "--firstname", "Ops", "--password", secret},
		{"admin:reset-user-password", "--email", "ops@example.com", "--password", secret + "x"},
		{"admin:create-user", "--email", "ops@example.com", "--firstname", "Ops", "--password", secret},
	} {
		run(append(args, "-dir", project), &out, &out)
	}
	if strings.Contains(out.String(), secret) {
		t.Fatalf("the password is printed: %s", out.String())
	}
	files, _ := filepath.Glob(filepath.Join(project, ".tmp", "data.db*"))
	if len(files) == 0 {
		t.Fatal("no database file")
	}
	for _, f := range files {
		db, err := os.ReadFile(f) //nolint:gosec // the test's own database
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(db, []byte(secret)) {
			t.Fatalf("the password is stored in clear in %s (audit log or elsewhere)", f)
		}
	}
}
