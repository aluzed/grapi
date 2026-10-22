package metadata_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/aluzed/grapi/internal/builtin"
	"github.com/aluzed/grapi/internal/db/metadata"
	"github.com/aluzed/grapi/internal/db/naming"
	"github.com/aluzed/grapi/internal/registry"
)

// Tables of Enterprise features (clean-room: grapi designs its own) and of
// the AI features, which grapi does not reproduce.
var excludedPrefixes = []string{"strapi_release", "strapi_workflows", "strapi_history_", "strapi_ai_"}

func excluded(table string) bool {
	for _, p := range excludedPrefixes {
		if strings.HasPrefix(table, p) {
			return true
		}
	}
	return false
}

func build(t *testing.T, fixture string) *metadata.Schema {
	t.Helper()
	res, err := registry.Load(os.DirFS(filepath.Join("../../../compat/fixtures", fixture)), registry.Options{
		Plugins: builtin.Plugins(),
		Admin:   builtin.Admin(),
	})
	if err != nil {
		t.Fatal(err)
	}
	raws, err := builtin.Models()
	if err != nil {
		t.Fatal(err)
	}
	var internal []*metadata.Model
	for _, raw := range raws {
		m, err := metadata.ParseModel(raw)
		if err != nil {
			t.Fatal(err)
		}
		internal = append(internal, m)
	}
	md, err := metadata.Build(res.Set, internal, naming.Default)
	if err != nil {
		t.Fatal(err)
	}
	s, err := md.Schema()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func golden(t *testing.T, path string) map[string]string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Tables []json.RawMessage `json:"tables"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, raw := range s.Tables {
		var head struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &head); err != nil {
			t.Fatal(err)
		}
		if excluded(head.Name) {
			continue
		}
		var buf bytes.Buffer
		if err := json.Compact(&buf, raw); err != nil {
			t.Fatal(err)
		}
		out[head.Name] = buf.String()
	}
	return out
}

// The schema grapi computes is byte for byte the one Strapi 5.59 stored for
// every fixture, so Strapi and grapi see no diff on each other's database.
func TestSchemaMatchesStrapi(t *testing.T) {
	for _, fixture := range []string{"basic", "relations", "components", "i18n-dp", "devfreelance", "naming"} {
		t.Run(fixture, func(t *testing.T) {
			got := build(t, fixture)
			for _, db := range []string{"sqlite", "postgres"} {
				want := golden(t, filepath.Join("../../../compat/golden", fixture+"-"+db, "schema/database-schema.json"))
				seen := map[string]bool{}
				for _, table := range got.Tables {
					seen[table.Name] = true
					b, err := metadata.Marshal(table)
					if err != nil {
						t.Fatal(err)
					}
					w, ok := want[table.Name]
					switch {
					case !ok:
						t.Errorf("%s: table %s is not in Strapi's schema", db, table.Name)
					case w != string(b):
						t.Errorf("%s: table %s differs\n got: %s\nwant: %s", db, table.Name, b, w)
					}
				}
				var missing []string
				for name := range want {
					if !seen[name] {
						missing = append(missing, name)
					}
				}
				sort.Strings(missing)
				if len(missing) > 0 {
					t.Errorf("%s: tables missing: %v", db, missing)
				}
			}
		})
	}
}
