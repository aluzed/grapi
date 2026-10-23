package config

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// dropNil removes the nil values: Strapi's undefined, absent from its JSON.
func dropNil(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, e := range x {
			if e != nil {
				out[k] = dropNil(e)
			}
		}
		return out
	case []any:
		for i, e := range x {
			x[i] = dropNil(e)
		}
	}
	return v
}

// dev-freelance's config, transcribed in YAML, gives Strapi's merged
// configuration (testdata/devfreelance-config.json: loadConfiguration on its
// TypeScript files with the same environment, plugins evaluated with
// @strapi/utils' env), leaf by leaf.
func TestDevFreelanceConfigTranscription(t *testing.T) {
	raw, err := os.ReadFile("testdata/devfreelance-config.json")
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]any
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open("testdata/devfreelance-config.env")
	if err != nil {
		t.Fatal(err)
	}
	env := Env{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if k, v, ok := strings.Cut(sc.Text(), "="); ok {
			env[k] = v
		}
	}
	_ = f.Close()
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, "dkim"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "dkim", "key.pem"), []byte("FAKE-DKIM-KEY\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(Options{Dir: project, ConfigDir: filepath.Join("..", "..", "compat", "fixtures", "devfreelance", "config-yaml"), Env: env})
	if err != nil {
		t.Fatal(err)
	}
	for path, w := range leaves(want, "") {
		if computedElsewhere(path) {
			continue
		}
		got, err := json.Marshal(c.Get(path))
		if err != nil {
			t.Fatal(err)
		}
		var g any
		_ = json.Unmarshal(got, &g)
		if !reflect.DeepEqual(dropNil(g), w) {
			wb, _ := json.Marshal(w)
			t.Errorf("%s: got %s, want %s", path, got, wb)
		}
	}
}

// computedElsewhere are keys of Strapi's merged object that grapi derives
// where it uses them, not in the config: the absolute URLs and the admin
// path (absoluteURLs), and the defaults of features it reads with their own
// fallbacks (server.openapi, server.transfer).
func computedElsewhere(path string) bool {
	switch path {
	case "server.absoluteUrl", "admin.url", "admin.absoluteUrl", "admin.path", "admin.serveAdminPanel":
		return true
	}
	return strings.HasPrefix(path, "server.openapi.") || strings.HasPrefix(path, "server.transfer.")
}

// leaves flattens objects to dotted paths; lists stay whole.
func leaves(v any, prefix string) map[string]any {
	out := map[string]any{}
	m, ok := v.(map[string]any)
	if !ok || prefix != "" && len(m) == 0 {
		out[prefix] = v
		return out
	}
	for k, e := range m {
		p := k
		if prefix != "" {
			p = prefix + "." + k
		}
		for kk, ee := range leaves(e, p) {
			out[kk] = ee
		}
	}
	return out
}
