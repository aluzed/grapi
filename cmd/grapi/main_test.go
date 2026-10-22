package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	version = "1.2.3"
	tests := []struct {
		args       []string
		code       int
		stdout     string
		stderrPart string
	}{
		{[]string{"version"}, 0, "1.2.3\n", ""},
		{[]string{"--version"}, 0, "1.2.3\n", ""},
		{nil, 2, "", "usage"},
		{[]string{"nope"}, 2, "", "unknown command"},
	}
	for _, tt := range tests {
		var out, errOut bytes.Buffer
		if code := run(tt.args, &out, &errOut); code != tt.code {
			t.Errorf("run(%v) = %d, want %d", tt.args, code, tt.code)
		}
		if out.String() != tt.stdout {
			t.Errorf("run(%v) stdout = %q, want %q", tt.args, out.String(), tt.stdout)
		}
		if !bytes.Contains(errOut.Bytes(), []byte(tt.stderrPart)) {
			t.Errorf("run(%v) stderr = %q, want it to contain %q", tt.args, errOut.String(), tt.stderrPart)
		}
	}
}

func TestNewProjectEnvAndReport(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	var out bytes.Buffer
	if code := run([]string{"new", "demo", "--db", "postgres", "--db-password", "s3cret"}, &out, &out); code != 0 {
		t.Fatalf("new: %d %s", code, out.String())
	}
	keys := func(file string) []string {
		b, err := os.ReadFile(filepath.Join(dir, "demo", file))
		if err != nil {
			t.Fatal(err)
		}
		var ks []string
		for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			k, _, _ := strings.Cut(line, "=")
			ks = append(ks, k)
		}
		return ks
	}
	if a, b := keys(".env"), keys(".env.example"); strings.Join(a, ",") != strings.Join(b, ",") {
		t.Fatalf(".env %v\n.env.example %v", a, b)
	}
	example, _ := os.ReadFile(filepath.Join(dir, "demo", ".env.example"))
	if strings.Contains(string(example), "s3cret") {
		t.Fatal(".env.example has a value")
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	out.Reset()
	if code := run([]string{"report", "-dir", filepath.Join(dir, "demo")}, &out, &out); code != 0 {
		t.Fatalf("report: %s", out.String())
	}
	env, _ := os.ReadFile(filepath.Join(dir, "demo", ".env"))
	for _, line := range strings.Split(string(env), "\n") {
		k, v, _ := strings.Cut(line, "=")
		secretKey := strings.Contains(k, "SECRET") || strings.Contains(k, "SALT") || strings.Contains(k, "KEY") || strings.Contains(k, "PASSWORD")
		if secretKey && v != "" && strings.Contains(out.String(), v) {
			t.Fatalf("report shows a secret: %s", out.String())
		}
	}
}
