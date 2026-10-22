// Package fixturetest loads the compat bench fixtures with grapi's built-in
// schemas, for tests that compare grapi with Strapi.
package fixturetest

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/aluzed/grapi/internal/builtin"
	"github.com/aluzed/grapi/internal/db/metadata"
)

// Names of the fixtures under compat/fixtures.
var Names = []string{"basic", "relations", "components", "i18n-dp", "devfreelance", "naming"}

// Tables of Enterprise features (clean-room: grapi designs its own) and of
// the AI features, present in the databases Strapi creates but not in grapi's.
var excludedPrefixes = []string{"strapi_release", "strapi_workflows", "strapi_history_", "strapi_ai_"}

// Excluded reports whether table belongs to a Strapi feature grapi does not
// reproduce.
func Excluded(table string) bool {
	for _, p := range excludedPrefixes {
		if strings.HasPrefix(table, p) {
			return true
		}
	}
	return false
}

// Compat is the path of the compat directory.
func Compat() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "../../compat")
}

// Schema returns the metadata and physical schema of a fixture.
func Schema(t testing.TB, fixture string) (*metadata.Metadata, *metadata.Schema) {
	t.Helper()
	_, md, err := builtin.Load(os.DirFS(filepath.Join(Compat(), "fixtures", fixture)))
	if err != nil {
		t.Fatal(err)
	}
	s, err := md.Schema()
	if err != nil {
		t.Fatal(err)
	}
	return md, s
}
