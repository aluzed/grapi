package metadata_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/aluzed/grapi/internal/db/metadata"
	"github.com/aluzed/grapi/internal/fixturetest"
)

func build(t *testing.T, fixture string) *metadata.Schema {
	t.Helper()
	_, s := fixturetest.Schema(t, fixture)
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
		if fixturetest.Excluded(head.Name) {
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
	for _, fixture := range fixturetest.Names {
		t.Run(fixture, func(t *testing.T) {
			got := build(t, fixture)
			for _, db := range []string{"sqlite", "postgres"} {
				want := golden(t, filepath.Join(fixturetest.Compat(), "golden", fixture+"-"+db, "schema/database-schema.json"))
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
