package schemasync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/aluzed/grapi/internal/db/metadata"
)

type strapiHashes struct {
	Hashes map[string]struct{ Full, CE string } `json:"hashes"`
	Order  []string                             `json:"order"`
}

func loadHashes(t *testing.T) strapiHashes {
	t.Helper()
	b, err := os.ReadFile("testdata/strapi-hashes.json")
	if err != nil {
		t.Fatal(err)
	}
	var h strapiHashes
	if err := json.Unmarshal(b, &h); err != nil {
		t.Fatal(err)
	}
	return h
}

func TestCollateMatchesLocaleCompare(t *testing.T) {
	want := loadHashes(t).Order
	got := append([]string(nil), want...)
	sort.Slice(got, func(i, j int) bool { return got[i] > got[j] })
	sort.SliceStable(got, func(i, j int) bool { return collate(got[i], got[j]) < 0 })
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("got %v\nwant %v", got, want)
	}
}

// Hashes computed by Strapi's own code on the golden schemas.
func TestHashMatchesStrapi(t *testing.T) {
	h := loadHashes(t)
	if len(h.Hashes) == 0 {
		t.Fatal("no vectors")
	}
	files := map[string]struct{ Full, CE string }{}
	for file, want := range h.Hashes {
		files[filepath.Join("../../../compat", file)] = want
	}
	// A private fixture's vectors sit with it (GRAPI_PRIVATE_COMPAT, see internal/fixturetest).
	if dir := os.Getenv("GRAPI_PRIVATE_COMPAT"); dir != "" {
		if b, err := os.ReadFile(filepath.Join(dir, "testdata", "strapi-hashes.json")); err == nil {
			var p strapiHashes
			if err := json.Unmarshal(b, &p); err != nil {
				t.Fatal(err)
			}
			for file, want := range p.Hashes {
				files[filepath.Join(dir, file)] = want
			}
		}
	}
	for file, want := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var raw struct {
			Tables []json.RawMessage `json:"tables"`
		}
		if err := json.Unmarshal(b, &raw); err != nil {
			t.Fatal(err)
		}
		full := &metadata.Schema{}
		for _, r := range raw.Tables {
			table, err := metadata.ParseTable(r)
			if err != nil {
				t.Fatal(err)
			}
			full.Tables = append(full.Tables, table)
		}
		got, err := Hash(full)
		if err != nil {
			t.Fatal(err)
		}
		if got != want.Full {
			t.Errorf("%s: got %s, want %s", file, got, want.Full)
		}
	}
}
