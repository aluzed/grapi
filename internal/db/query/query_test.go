package query

import (
	"context"
	"encoding/json"
	"math"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/aluzed/grapi/internal/db"
	"github.com/aluzed/grapi/internal/db/schemasync"
	"github.com/aluzed/grapi/internal/fixturetest"
)

func TestParseISO(t *testing.T) {
	utc := func(s string) time.Time { tm, _ := time.Parse(time.RFC3339Nano, s); return tm }
	for in, want := range map[string]time.Time{
		"2026-03-01T10:00:00.000Z":  utc("2026-03-01T10:00:00Z"),
		"2026-03-01T10:00:00+02:00": utc("2026-03-01T08:00:00Z"),
		"2026-03-01T10:00Z":         utc("2026-03-01T10:00:00Z"),
		"2026-03-01T10:00:00.5Z":    utc("2026-03-01T10:00:00.5Z"),
	} {
		got, err := parseISO(in)
		if err != nil || !got.Equal(want) {
			t.Errorf("%s: %v, %v", in, got, err)
		}
	}
	for _, bad := range []string{"15/01/2026", "2026-13-01", "2026-02-30", "now", ""} {
		if _, err := parseISO(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
	if got, err := parseDateTime("1769904000000"); err != nil || got.UnixMilli() != 1769904000000 {
		t.Errorf("epoch milliseconds: %v, %v", got, err)
	}
}

func TestValueConversions(t *testing.T) {
	if v, _ := toDB("boolean", "f", true); v != false {
		t.Error(`"f" is false`)
	}
	if v, _ := toDB("boolean", "yes", true); v != true {
		t.Error(`a non empty string is true`)
	}
	if _, err := toDB("integer", "abc", true); err == nil {
		t.Error("abc is not a number")
	}
	if v, _ := toDB("integer", " 0x1f ", true); v != float64(31) {
		t.Errorf("hex: %v", v)
	}
	if v, _ := toDB("time", "08:05:09.12", true); v != "08:05:09.120" {
		t.Errorf("time: %v", v)
	}
	if v, _ := toDB("date", "2026-01-15T10:00:00Z", true); v != "2026-01-15" {
		t.Errorf("date prefix: %v", v)
	}
	if v, _ := toDB("biginteger", "+0042", true); v != "42" {
		t.Errorf("biginteger: %v", v)
	}
	if v := fromDB("timestamp", int64(1769904000000)); v != "1769904000000" {
		t.Errorf("timestamp: %v", v)
	}
	if v := fromDB("datetime", int64(1769904000000)); v != "2026-02-01T00:00:00.000Z" {
		t.Errorf("datetime: %v", v)
	}
	if v := fromDB("json", `"{\"a\":1}"`); v.(map[string]any)["a"] != float64(1) {
		t.Errorf("json stringified twice: %v", v)
	}
	if v := fromDB("decimal", "10.50"); v != 10.5 {
		t.Errorf("decimal: %v", v)
	}
	if v := lodashToNumber("abc"); !math.IsNaN(v.(float64)) {
		t.Errorf("NaN: %v", v)
	}
	if escapeLike(`50%_\`) != `50\%\_\\` {
		t.Error(escapeLike(`50%_\`))
	}
}

func newEngine(t testing.TB, fixture string) *Engine {
	t.Helper()
	md, desired := fixturetest.Schema(t, fixture)
	d, err := db.Open(context.Background(), db.Config{Client: "sqlite", Filename: filepath.Join(t.TempDir(), "data.db")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if _, err := schemasync.Sync(context.Background(), d, desired, schemasync.Options{}); err != nil {
		t.Fatal(err)
	}
	return &Engine{DB: d, MD: md}
}

// Any filter tree either runs or is refused with an error: never a panic,
// never SQL the database rejects.
func FuzzWhere(f *testing.F) {
	for _, seed := range []string{
		`{"title":{"$eq":"x"}}`,
		`{"$or":[{"views":1},{"title":{"$containsi":["a","b"]}}]}`,
		`{"$not":{"featured":true}}`,
		`{"views":{"$between":[1,2]},"title":null}`,
		`{"createdBy":{"firstname":{"$in":[]}}}`,
		`{"$and":[{"$or":[]},{"title":{"$not":{"$null":"false"}}}]}`,
	} {
		f.Add(seed, "views")
	}
	e := newEngine(f, "basic")
	f.Fuzz(func(t *testing.T, where, sort string) {
		var w any
		if json.Unmarshal([]byte(where), &w) != nil {
			return
		}
		for _, uid := range []string{"api::article.article", "api::tag.tag"} {
			_, err := e.FindMany(context.Background(), uid, Params{Where: w, OrderBy: sort})
			if err != nil && strings.Contains(strings.ToLower(err.Error()), "syntax") {
				t.Fatalf("invalid SQL for %s: %v", where, err)
			}
			if _, err := e.Count(context.Background(), uid, Params{Where: w}); err != nil && strings.Contains(strings.ToLower(err.Error()), "syntax") {
				t.Fatalf("invalid SQL for count %s: %v", where, err)
			}
		}
	})
}

func TestNewDocumentID(t *testing.T) {
	re := regexp.MustCompile(`^[a-z][a-z0-9]{23}$`)
	n := 1_000_000
	if testing.Short() {
		n = 10_000
	}
	seen := make(map[string]struct{}, n)
	for range n {
		id := NewDocumentID()
		if !re.MatchString(id) {
			t.Fatalf("format: %q", id)
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate after %d: %q", len(seen), id)
		}
		seen[id] = struct{}{}
	}
}
