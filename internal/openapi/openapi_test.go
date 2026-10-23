package openapi

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"testing"

	"github.com/aluzed/grapi/internal/builtin"
	"github.com/aluzed/grapi/internal/fixturetest"
)

var isoTime = regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{3}Z$`)

// canonical decodes v through JSON with the evaluated datetime defaults
// masked.
func canonical(v any) any {
	b, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(b, &out)
	var walk func(any) any
	walk = func(x any) any {
		switch t := x.(type) {
		case map[string]any:
			for k, e := range t {
				if s, ok := e.(string); ok && k == "default" && isoTime.MatchString(s) {
					t[k] = "<now>"
					continue
				}
				t[k] = walk(e)
			}
		case []any:
			for i, e := range t {
				t[i] = walk(e)
			}
		}
		return x
	}
	return walk(out)
}

func same(a, b any) bool {
	x, _ := json.Marshal(canonical(a))
	y, _ := json.Marshal(canonical(b))
	return string(x) == string(y)
}

// firstDiff is the first JSON path where a and b differ.
func firstDiff(a, b any, path string) string {
	am, aok := a.(map[string]any)
	bm, bok := b.(map[string]any)
	if aok && bok {
		keys := map[string]bool{}
		for k := range am {
			keys[k] = true
		}
		for k := range bm {
			keys[k] = true
		}
		var sorted []string
		for k := range keys {
			sorted = append(sorted, k)
		}
		sort.Strings(sorted)
		for _, k := range sorted {
			if d := firstDiff(am[k], bm[k], path+"."+k); d != "" {
				return d
			}
		}
		return ""
	}
	al, aok := a.([]any)
	bl, bok := b.([]any)
	if aok && bok && len(al) == len(bl) {
		for i := range al {
			if d := firstDiff(al[i], bl[i], fmt.Sprintf("%s[%d]", path, i)); d != "" {
				return d
			}
		}
		return ""
	}
	if same(a, b) {
		return ""
	}
	return fmt.Sprintf("%s: want %s got %s", path, short(a), short(b))
}

func short(v any) string {
	b, _ := json.Marshal(canonical(v))
	if len(b) > 1500 {
		b = append(b[:1500], "..."...)
	}
	return string(b)
}

func TestDocumentMatchesStrapi(t *testing.T) {
	for _, fx := range append([]string{"graphql"}, fixturetest.All()...) {
		t.Run(fx, func(t *testing.T) {
			raw, err := os.ReadFile(fixturetest.Testdata("oas-" + fx + ".json"))
			if err != nil {
				t.Fatal(err)
			}
			var want map[string]any
			if err := json.Unmarshal(raw, &want); err != nil {
				t.Fatal(err)
			}
			res, _, err := builtin.Load(os.DirFS(fixturetest.Dir(fx)))
			if err != nil {
				t.Fatal(err)
			}
			got := canonical(Generate(res.Set, Options{Info: Info{Name: "grapi-compat-reference", Version: "0.0.0"}})).(map[string]any)
			for _, k := range []string{"openapi", "info", "servers", "security"} {
				if !same(want[k], got[k]) {
					t.Errorf("%s: want %s got %s", k, short(want[k]), short(got[k]))
				}
			}
			wc := want["components"].(map[string]any)
			gc := got["components"].(map[string]any)
			if !same(wc["securitySchemes"], gc["securitySchemes"]) {
				t.Errorf("securitySchemes differ")
			}
			ws, gs := wc["schemas"].(map[string]any), gc["schemas"].(map[string]any)
			for name, s := range ws {
				if d := firstDiff(canonical(s), canonical(gs[name]), ""); d != "" {
					t.Errorf("schema %s %s", name, d)
				}
			}
			for name := range gs {
				if _, ok := ws[name]; !ok {
					t.Errorf("extra schema %s", name)
				}
			}
			wp, gp := want["paths"].(map[string]any), got["paths"].(map[string]any)
			var keys []string
			for p := range wp {
				keys = append(keys, p)
			}
			sort.Strings(keys)
			for _, p := range keys {
				for m, op := range wp[p].(map[string]any) {
					var g any
					if ops, ok := gp[p].(map[string]any); ok {
						g = ops[m]
					}
					if d := firstDiff(canonical(op), canonical(g), ""); d != "" {
						t.Errorf("%s %s %s", m, p, d)
					}
				}
			}
			for p, ops := range gp {
				for m := range ops.(map[string]any) {
					if w, ok := wp[p].(map[string]any); !ok || w[m] == nil {
						t.Errorf("extra operation %s %s", m, p)
					}
				}
			}
		})
	}
}
