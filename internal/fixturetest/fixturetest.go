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

// Names of the public fixtures, under compat/fixtures.
var Names = []string{"basic", "relations", "components", "i18n-dp", "naming"}

// PrivateEnv names the directory of fixtures that stay out of this
// repository (a real project's schemas and rules), laid out as compat/:
// fixtures/, golden/, corpus/, querycases/, testdata/.
const PrivateEnv = "GRAPI_PRIVATE_COMPAT"

// All are the public fixtures and those of PrivateEnv, if set.
func All() []string {
	out := append([]string{}, Names...)
	if dir := os.Getenv(PrivateEnv); dir != "" {
		entries, _ := os.ReadDir(filepath.Join(dir, "fixtures"))
		for _, e := range entries {
			if e.IsDir() {
				out = append(out, e.Name())
			}
		}
	}
	return out
}

// root is the compat directory holding fixture: this repository's, or
// the private one.
func root(fixture string) string {
	if _, err := os.Stat(filepath.Join(Compat(), "fixtures", fixture)); err == nil {
		return Compat()
	}
	return os.Getenv(PrivateEnv)
}

// Dir is the directory of fixture.
func Dir(fixture string) string { return filepath.Join(root(fixture), "fixtures", fixture) }

// Require skips the test when fixture is not available (a private fixture
// without PrivateEnv).
func Require(t testing.TB, fixture string) {
	t.Helper()
	if _, err := os.Stat(Dir(fixture)); err != nil {
		t.Skipf("fixture %s not available (private fixtures need %s)", fixture, PrivateEnv)
	}
}

// Golden is a file of fixture's goldens for db (sqlite or postgres).
func Golden(fixture, db string, elem ...string) string {
	return filepath.Join(append([]string{root(fixture), "golden", fixture + "-" + db}, elem...)...)
}

// Testdata is name in the calling package's testdata/, or in the private
// testdata/ when it is not there.
func Testdata(name string) string {
	local := filepath.Join("testdata", name)
	if _, err := os.Stat(local); err == nil {
		return local
	}
	if dir := os.Getenv(PrivateEnv); dir != "" {
		return filepath.Join(dir, "testdata", name)
	}
	return local
}

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
	Require(t, fixture)
	_, md, err := builtin.Load(os.DirFS(Dir(fixture)))
	if err != nil {
		t.Fatal(err)
	}
	s, err := md.Schema()
	if err != nil {
		t.Fatal(err)
	}
	return md, s
}
