package config

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func project(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func load(t *testing.T, files map[string]string, env Env) (*Config, error) {
	t.Helper()
	return Load(Options{Dir: project(t, files), Env: env})
}

func TestEnvHelpers(t *testing.T) {
	c, err := load(t, map[string]string{"config/custom.yaml": `
str: ${env:S}
empty: ${env:EMPTY:fallback}
missing: ${env:NOPE}
def: ${env:NOPE:fallback:with:colons}
port: ${env.int:PORT:1337}
portDefault: ${env.int:NOPE:1337}
badInt: ${env.int:WORD}
float: ${env.float:F}
boolOne: ${env.bool:ONE}
boolTrue: ${env.bool:TRUE}
json: ${env.json:J}
array: ${env.array:A}
choice: ${env.oneOf:DB:postgres|sqlite:sqlite}
choiceOut: ${env.oneOf:BAD:postgres|sqlite:sqlite}
url: http://${env:HOST}:${env.int:PORT}/x
`}, Env{
		"S": "v", "EMPTY": "", "PORT": "8080x", "WORD": "abc", "F": "1.5e2kg", "ONE": "1", "TRUE": "true",
		"J": `{"a":[1,2]}`, "A": `[a, "b" ,c]`, "DB": "postgres", "BAD": "mysql", "HOST": "h",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"str": "v", "empty": "", "missing": nil, "def": "fallback:with:colons",
		"port": 8080, "portDefault": 1337, "float": 150, "boolOne": false, "boolTrue": true,
		"json": map[string]any{"a": []any{float64(1), float64(2)}}, "array": []any{"a", "b", "c"},
		"choice": "postgres", "choiceOut": "sqlite", "url": "http://h:8080/x",
	}
	got := c.Get("custom").(map[string]any)
	for k, w := range want {
		if !reflect.DeepEqual(got[k], w) {
			t.Errorf("%s = %#v, want %#v", k, got[k], w)
		}
	}
	if f, ok := got["badInt"].(float64); !ok || !math.IsNaN(f) {
		t.Errorf("badInt = %#v, want NaN", got["badInt"])
	}
}

func TestOneOfDefaultOutsideChoices(t *testing.T) {
	_, err := load(t, map[string]string{"config/database.yaml": "client: ${env.oneOf:DB:postgres|sqlite:mysql}"}, Env{})
	if err == nil || !strings.Contains(err.Error(), "defaultValue to be included") {
		t.Fatalf("err = %v", err)
	}
}

func TestInvalidJSON(t *testing.T) {
	_, err := load(t, map[string]string{"config/x.yaml": "v: ${env.json:J}"}, Env{"J": "{nope"})
	if err == nil || !strings.Contains(err.Error(), "invalid json environment variable J") {
		t.Fatalf("err = %v", err)
	}
}

func TestFileDirective(t *testing.T) {
	c, err := load(t, map[string]string{
		"config/plugins.yaml": "email:\n  dkim: ${file:dkim/private.key}\n",
		"dkim/private.key":    "KEY\n",
	}, Env{})
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := c.String("plugins.email.dkim"); v != "KEY\n" {
		t.Fatalf("dkim = %q", v)
	}
	_, err = load(t, map[string]string{"config/plugins.yaml": "email:\n  dkim: ${file:dkim/missing.key}\n"}, Env{})
	if err == nil || !strings.Contains(err.Error(), "plugins.email.dkim") {
		t.Fatalf("err = %v", err)
	}
}

func TestEnvironmentOverride(t *testing.T) {
	files := map[string]string{
		"config/server.yaml":                "port: 1400\napp:\n  keys: [a, b]\n",
		"config/env/production/server.yaml": "port: 1500\napp:\n  keys: [c]\n",
		"config/env/production/api.json":    `{"rest": {"maxLimit": 50}}`,
	}
	dev, err := load(t, files, Env{})
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := dev.Int("server.port"); p != 1400 {
		t.Fatalf("development port = %d", p)
	}
	if _, ok := dev.Int("api.rest.maxLimit"); ok {
		t.Fatal("maxLimit set in development")
	}
	prod, err := load(t, files, Env{"NODE_ENV": "production"})
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := prod.Int("server.port"); p != 1500 {
		t.Fatalf("production port = %d", p)
	}
	if m, _ := prod.Int("api.rest.maxLimit"); m != 50 {
		t.Fatalf("maxLimit = %d", m)
	}
	if v, _ := prod.String("api.rest.prefix"); v != "/api" {
		t.Fatalf("prefix default lost: %q", v)
	}
	if keys := prod.Get("server.app.keys"); !reflect.DeepEqual(keys, []any{"c", "b"}) {
		t.Fatalf("keys = %#v (lodash merges arrays by index)", keys)
	}
	if h, _ := prod.String("server.host"); h != "0.0.0.0" {
		t.Fatalf("host = %q", h)
	}
}

func TestDotenvAndAliases(t *testing.T) {
	env := Env{"KEEP": "process", "GRAPI_ENV": "staging"}
	c, err := load(t, map[string]string{
		".env":                           "KEEP=dotenv\nNEW=\"from dotenv\" \n# comment\nexport OTHER=x # trailing\n",
		"config/env/staging/custom.yaml": "keep: ${env:KEEP}\nnew: ${env:NEW}\nother: ${env:OTHER}\n",
	}, env)
	if err != nil {
		t.Fatal(err)
	}
	if c.Environment != "staging" {
		t.Fatalf("environment = %q", c.Environment)
	}
	want := map[string]any{"keep": "process", "new": "from dotenv", "other": "x"}
	if got := c.Get("custom"); !reflect.DeepEqual(got, want) {
		t.Fatalf("custom = %#v", got)
	}
	if v, _ := (Env{"NODE_ENV": "a", "GRAPI_ENV": "b"}).Lookup("GRAPI_ENV", nil); v != "b" {
		t.Fatalf("GRAPI_ENV should win, got %q", v)
	}
}

func TestRestrictedAndTyped(t *testing.T) {
	c, err := load(t, map[string]string{"config/uuid.yaml": "a: 1", "config/middleware.json": "{}"}, Env{})
	if err != nil {
		t.Fatal(err)
	}
	if c.Get("uuid") != nil || c.Get("middleware") != nil {
		t.Fatal("restricted files loaded")
	}
	_, err = load(t, map[string]string{"config/server.yaml": "port: ${env:PORT}"}, Env{"PORT": "80"})
	if err == nil || !strings.Contains(err.Error(), "server.yaml: server.port must be an integer") {
		t.Fatalf("err = %v", err)
	}
}
