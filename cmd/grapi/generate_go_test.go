package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateGoSkeletons(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a Go program")
	}
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	t.Chdir(dir)
	var out bytes.Buffer
	if code := run([]string{"new", "demo", "--framework"}, &out, &out); code != 0 {
		t.Fatalf("new: %s", out.String())
	}
	project := filepath.Join(dir, "demo")
	if !isFramework(project) {
		t.Fatal("grapi new --framework writes go.mod and main.go")
	}
	gen := func(args ...string) (int, string) {
		out.Reset()
		code := run(append([]string{"generate"}, append(args, "-dir", project)...), &out, &out)
		return code, out.String()
	}
	for _, args := range [][]string{
		{"api", "--name", "newsletter-signup"},
		{"policy", "--name", "is-owner"},
		{"route", "--method", "post", "--path", "/account/anonymize", "--action", "api::account.account.anonymize"},
		{"route", "--path", "/ping", "--public"},
		{"document-middleware", "--name", "audit", "--uid", "api::post.post"},
		{"lifecycle", "--model", "api::post.post", "--event", "update"},
	} {
		if c, o := gen(args...); c != 0 {
			t.Fatalf("%v: %s", args, o)
		}
	}
	if c, o := gen("policy", "--name", "is-owner"); c == 0 || !strings.Contains(o, "already exists") {
		t.Fatalf("no overwrite: %d %s", c, o)
	}
	for _, f := range []string{"policy_global_is_owner.go", "route_post_account_anonymize.go", "route_get_ping.go", "middleware_audit.go", "lifecycle_api_post_post_update.go"} {
		if _, err := os.Stat(filepath.Join(project, f)); err != nil {
			t.Fatal(err)
		}
	}
	mod := "module demo\n\ngo 1.26\n\nrequire github.com/aluzed/grapi v0.0.0\n\nreplace github.com/aluzed/grapi => " + repo + "\n"
	if err := os.WriteFile(filepath.Join(project, "go.mod"), []byte(mod), 0o600); err != nil {
		t.Fatal(err)
	}
	gocmd := func(args ...string) {
		cmd := exec.Command("go", args...)
		cmd.Dir = project
		cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off")
		if o, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("go %v: %v\n%s", args, err, o)
		}
	}
	// Offline: grapi's go.sum covers what the build needs.
	sum, err := os.ReadFile(filepath.Join(repo, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "go.sum"), sum, 0o600); err != nil {
		t.Fatal(err)
	}
	if o, err := exec.Command("gofmt", "-l", project).CombinedOutput(); err != nil || len(bytes.TrimSpace(o)) > 0 {
		t.Fatalf("gofmt: %v %s", err, o)
	}
	gocmd("vet", ".")
	gocmd("build", "-o", filepath.Join(dir, "demo-bin"), ".")
}

func TestGenerateGoNeedsFramework(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	var out bytes.Buffer
	run([]string{"new", "plain"}, &out, &out)
	out.Reset()
	if code := run([]string{"generate", "policy", "--name", "x", "-dir", filepath.Join(dir, "plain")}, &out, &out); code == 0 || !strings.Contains(out.String(), "framework project") {
		t.Fatalf("a plain project has no Go skeletons: %d %s", code, out.String())
	}
}
