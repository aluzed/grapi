package typegen

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/aluzed/grapi/internal/builtin"
	"github.com/aluzed/grapi/internal/fixturetest"
)

// Enterprise content types (clean-room) are left out of the reference.
var enterpriseBlock = regexp.MustCompile(`(?s)export interface Plugin(ContentReleases|ReviewWorkflows)\w*\s+extends .*?\n}\n`)

var enterpriseEntry = regexp.MustCompile(`(?m)^\s*'plugin::(content-releases|review-workflows)\.[^']*': \w+;\n`)

var spaces = regexp.MustCompile(`\s+`)

// compact drops the formatting prettier adds.
func compact(s string) string { return spaces.ReplaceAllString(s, "") }

func firstDiff(want, got string) string {
	for i := 0; i < len(want) && i < len(got); i++ {
		if want[i] != got[i] {
			lo := max(0, i-120)
			return "\n want …" + want[lo:min(len(want), i+120)] + "\n  got …" + got[lo:min(len(got), i+120)]
		}
	}
	return "\n lengths differ"
}

func TestMatchesStrapi(t *testing.T) {
	for _, fx := range append([]string{"graphql"}, fixturetest.All()...) {
		t.Run(fx, func(t *testing.T) {
			res, _, err := builtin.Load(os.DirFS(fixturetest.Dir(fx)))
			if err != nil {
				t.Fatal(err)
			}
			got := Generate(res.Set)
			for name, out := range map[string]string{"contentTypes.d.ts": got.ContentTypes, "components.d.ts": got.Components} {
				raw, err := os.ReadFile(filepath.Join(fixturetest.Testdata(fx), name))
				if err != nil {
					t.Fatal(err)
				}
				want := enterpriseEntry.ReplaceAllString(enterpriseBlock.ReplaceAllString(string(raw), ""), "")
				if w, g := compact(want), compact(out); w != g {
					t.Errorf("%s:%s", name, firstDiff(w, g))
				}
			}
		})
	}
}

func TestDeterministic(t *testing.T) {
	res, _, err := builtin.Load(os.DirFS(fixturetest.Dir("components")))
	if err != nil {
		t.Fatal(err)
	}
	a, b := Generate(res.Set), Generate(res.Set)
	if a != b || !strings.Contains(a.ContentTypes, "export interface ApiPostPost extends Struct.CollectionTypeSchema") {
		t.Fatal("two generations differ")
	}
}

func TestClient(t *testing.T) {
	res, _, err := builtin.Load(os.DirFS(fixturetest.Dir("components")))
	if err != nil {
		t.Fatal(err)
	}
	got := Client(res.Set)
	if got != Client(res.Set) {
		t.Fatal("two generations differ")
	}
	for _, want := range []string{
		"  seo?: SharedSeo | null;\n",
		"  links?: SharedLink[];\n",
		"(BlocksHero & { __component: 'blocks.hero' }) | (BlocksRichText & { __component: 'blocks.rich-text' })",
		"  title: string;\n  updatedAt: string;\n",
		"  'api::post.post': ApiPostPost;\n",
		"export interface SharedSeoInput {\n  id?: number;\n",
		"  links?: SharedLinkInput[];\n",
		"  'api::post.post': ApiPostPostInput;\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q", want)
		}
	}
	user := got[strings.Index(got, "export interface PluginUsersPermissionsUser {"):]
	user = user[:strings.Index(user, "\n}\n")]
	if strings.Contains(got, "AdminUser") || strings.Contains(user, "password") {
		t.Error("admin types or private attributes leaked into the client types")
	}
	if !strings.Contains(got, "export interface PluginUsersPermissionsUserInput {") {
		t.Error("no input type for users")
	}
}
