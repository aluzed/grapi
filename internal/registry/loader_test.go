package registry

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/aluzed/grapi/schema"
)

const postSchema = `{"kind":"collectionType","info":{"singularName":"post","pluralName":"posts","displayName":"Post"},"attributes":{"title":{"type":"string"},"seo":{"type":"component","component":"shared.seo"},"author":{"type":"relation","relation":"manyToOne","target":"plugin::users-permissions.user"}}}`

var usersPermissions = PluginSchemas{Name: "users-permissions", FS: fstest.MapFS{
	"content-types/user/schema.json": {Data: []byte(`{"collectionName":"up_users","info":{"singularName":"user","pluralName":"users","displayName":"User"},"attributes":{"username":{"type":"string"}}}`)},
	"content-types/role/schema.json": {Data: []byte(`{"collectionName":"up_roles","info":{"singularName":"role","pluralName":"roles","displayName":"Role"},"attributes":{"name":{"type":"string"},"users":{"type":"relation","relation":"oneToMany","target":"plugin::users-permissions.user","mappedBy":"role"}}}`)},
}}

func app(files map[string]string) fstest.MapFS {
	m := fstest.MapFS{}
	for p, s := range files {
		m[p] = &fstest.MapFile{Data: []byte(s)}
	}
	return m
}

func TestLoadProject(t *testing.T) {
	fsys := app(map[string]string{
		"src/api/post/content-types/post/schema.json":                     postSchema,
		"src/api/post/content-types/post/lifecycles.ts":                   "export default {}",
		"src/api/post/routes/post.ts":                                     "/** post router */\nimport { factories } from '@strapi/strapi';\n\nexport default factories.createCoreRouter('api::post.post');\n",
		"src/api/post/routes/custom.ts":                                   "export default { routes: [] }",
		"src/api/post/controllers/post.js":                                "module.exports = require('@strapi/strapi').factories.createCoreController(\"api::post.post\");",
		"src/api/post/services/post.ts":                                   "import { factories } from '@strapi/strapi';\nexport default factories.createCoreService('api::post.post', ({ strapi }) => ({}));",
		"src/components/shared/seo.json":                                  `{"collectionName":"components_shared_seos","info":{"displayName":"Seo"},"attributes":{"title":{"type":"string"}}}`,
		"src/extensions/users-permissions/content-types/user/schema.json": `{"attributes":{"username":{"type":"string"},"bio":{"type":"text"}}}`,
		"src/extensions/users-permissions/strapi-server.ts":               "export default (p) => p",
		"src/index.ts":                      "export default {}",
		"config/server.ts":                  "export default {}",
		"config/env/production/database.ts": "export default {}",
	})
	res, err := Load(fsys, Options{Plugins: []PluginSchemas{usersPermissions}})
	if err != nil {
		t.Fatal(err)
	}
	post, ok := res.Set.ContentType("api::post.post")
	if !ok || post.CollectionName != "post" {
		t.Fatalf("post not loaded or collectionName not defaulted to the singular name: %+v", post)
	}
	user, _ := res.Set.ContentType("plugin::users-permissions.user")
	if _, ok := user.Attributes.Get("bio"); !ok || user.CollectionName != "up_users" || user.GlobalID != "UsersPermissionsUser" {
		t.Errorf("extension not merged shallowly: %v %s %s", user.Attributes.Keys(), user.CollectionName, user.GlobalID)
	}
	got := map[string]string{}
	for _, c := range res.Code {
		got[c.Path] = c.Kind
	}
	want := map[string]string{
		"src/api/post/content-types/post/lifecycles.ts":     KindLifecycles,
		"src/api/post/routes/post.ts":                       KindDefaultFactory,
		"src/api/post/routes/custom.ts":                     KindCustomRoutes,
		"src/api/post/controllers/post.js":                  KindDefaultFactory,
		"src/api/post/services/post.ts":                     KindService,
		"src/extensions/users-permissions/strapi-server.ts": KindPluginExtension,
		"src/index.ts":                      KindAppIndex,
		"config/server.ts":                  KindConfig,
		"config/env/production/database.ts": KindConfig,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("code inventory:\n got %v\nwant %v", got, want)
	}
}

func TestLoadErrors(t *testing.T) {
	tests := map[string]map[string]string{
		"missing schema.json": {"src/api/post/content-types/post/lifecycles.js": "x"},
		"key differs from singularName": {
			"src/api/post/content-types/article/schema.json": postSchema,
		},
		"unknown relation target (plugin not loaded)": {
			"src/api/post/content-types/post/schema.json": postSchema,
			"src/components/shared/seo.json":              `{"collectionName":"components_shared_seos","info":{"displayName":"Seo"},"attributes":{}}`,
		},
		"component without collectionName": {"src/components/shared/seo.json": `{"info":{"displayName":"Seo"},"attributes":{}}`},
	}
	for name, files := range tests {
		if _, err := Load(app(files), Options{}); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestLoadIsDeterministic(t *testing.T) {
	fsys := app(map[string]string{
		"src/api/b/content-types/b/schema.json": `{"info":{"singularName":"b","pluralName":"bs","displayName":"B"},"attributes":{}}`,
		"src/api/a/content-types/a/schema.json": `{"info":{"singularName":"a","pluralName":"as","displayName":"A"},"attributes":{}}`,
	})
	first, err := Load(fsys, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		again, _ := Load(fsys, Options{})
		if !reflect.DeepEqual(first.Set.ContentTypes.Keys(), again.Set.ContentTypes.Keys()) {
			t.Fatal("load order changed between runs")
		}
	}
	if strings.Join(first.Set.ContentTypes.Keys(), ",") != "api::a.a,api::b.b" {
		t.Errorf("order = %v", first.Set.ContentTypes.Keys())
	}
}

func TestLoadFixtures(t *testing.T) {
	dirs, _ := filepath.Glob("../../compat/fixtures/*")
	for _, dir := range dirs {
		res, err := Load(os.DirFS(dir), Options{Plugins: []PluginSchemas{usersPermissions}})
		if err != nil {
			t.Errorf("%s: %v", filepath.Base(dir), err)
			continue
		}
		if res.Set.ContentTypes.Len() == 0 {
			t.Errorf("%s: no content type loaded", filepath.Base(dir))
		}
	}
}

// Reads the real dev-freelance backend when it is present on this machine.
func TestLoadDevFreelance(t *testing.T) {
	dir := os.Getenv("GRAPI_DEVFREELANCE_BACKEND")
	if dir == "" {
		t.Skip("GRAPI_DEVFREELANCE_BACKEND not set")
	}
	res, err := Load(os.DirFS(dir), Options{Plugins: []PluginSchemas{usersPermissions}})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d content types, %d components, %d code files", res.Set.ContentTypes.Len(), res.Set.Components.Len(), len(res.Code))
	for _, c := range res.Code {
		if c.Kind != KindDefaultFactory {
			t.Logf("%-70s %s", c.Path, c.Kind)
		}
	}
	for _, w := range res.Warnings {
		t.Logf("warning: %s", w)
	}
}

func TestDefaultFactoryUID(t *testing.T) {
	cases := map[string]string{
		"export default factories.createCoreRouter('api::a.a');":                                                     "api::a.a",
		"import { factories } from \"@strapi/strapi\";\nexport default factories.createCoreController(\"api::a.a\")": "api::a.a",
		"export default factories.createCoreRouter('api::a.a', { only: ['find'] });":                                 "",
		"export default factories.createCoreService('api::a.a', ({ strapi }) => ({}));":                              "",
	}
	for src, want := range cases {
		got, ok := defaultFactoryUID(src)
		if got != want || ok != (want != "") {
			t.Errorf("defaultFactoryUID(%q) = %q, %v", src, got, ok)
		}
	}
	_ = schema.UID("")
}
