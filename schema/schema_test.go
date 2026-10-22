package schema

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Expected values come from lodash 4 run in the reference Strapi image.
func TestWordsAndCases(t *testing.T) {
	tests := []struct{ in, pascal, snake string }{
		{"article", "Article", "article"},
		{"theme-page", "ThemePage", "theme_page"},
		{"users-permissions-user", "UsersPermissionsUser", "users_permissions_user"},
		{"component_shared.seo", "ComponentSharedSeo", "component_shared_seo"},
		{"createdAt", "CreatedAt", "created_at"},
		{"documentId", "DocumentId", "document_id"},
		{"XMLHttpRequest", "XmlHttpRequest", "xml_http_request"},
		{"item2go", "Item2Go", "item_2_go"},
		{"__component", "Component", "component"},
		{"v2Api", "V2Api", "v_2_api"},
		{"foo2Bar", "Foo2Bar", "foo_2_bar"},
		{"ABC", "Abc", "abc"},
		{"aBC", "ABc", "a_bc"},
		{"2nd-place", "2ndPlace", "2nd_place"},
		{"élan-vital", "ElanVital", "elan_vital"},
	}
	for _, tt := range tests {
		if got := pascalCase(tt.in); got != tt.pascal {
			t.Errorf("pascalCase(%q) = %q, want %q", tt.in, got, tt.pascal)
		}
		if got := snakeCase(tt.in); got != tt.snake {
			t.Errorf("snakeCase(%q) = %q, want %q", tt.in, got, tt.snake)
		}
	}
}

func TestUID(t *testing.T) {
	u := APIUID("article", "article")
	if u != "api::article.article" || u.Namespace() != "api" || u.Origin() != "article" || u.IsComponent() {
		t.Errorf("unexpected api uid parts for %s", u)
	}
	if c := ComponentUID("shared", "seo"); !c.IsComponent() || c.Namespace() != "" {
		t.Errorf("unexpected component uid parts for %s", c)
	}
}

func TestOrderedMapKeepsOrder(t *testing.T) {
	var m OrderedMap[int]
	if err := json.Unmarshal([]byte(`{"z":1,"a":2,"m":3}`), &m); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(m.Keys(), ","); got != "z,a,m" {
		t.Fatalf("keys = %s", got)
	}
	m.Delete("a")
	m.Set("b", 4)
	b, _ := json.Marshal(m)
	if string(b) != `{"z":1,"m":3,"b":4}` {
		t.Errorf("marshal = %s", b)
	}
}

const article = `{
  "kind": "collectionType",
  "collectionName": "articles",
  "info": {"singularName": "article", "pluralName": "articles", "displayName": "Article", "custom": 1},
  "options": {"draftAndPublish": true, "futureOption": "x"},
  "pluginOptions": {"i18n": {"localized": true}},
  "attributes": {
    "title": {"type": "string", "required": true, "maxLength": 120, "pluginOptions": {"i18n": {"localized": true}}},
    "status": {"type": "string"},
    "slug": {"type": "uid", "targetField": "title", "x-unknown": [1, 2]},
    "views": {"type": "integer", "default": 0, "min": 0, "required": false}
  },
  "topLevelUnknown": true
}`

func TestContentTypeRoundTripIsLossless(t *testing.T) {
	ct, err := ParseContentType([]byte(article))
	if err != nil {
		t.Fatal(err)
	}
	if err := ct.Prepare(APIUID("article", "article"), ""); err != nil {
		t.Fatal(err)
	}
	assertSameJSON(t, "article", []byte(article), ct)
}

func TestPrepareDerivesAndAddsAutoAttributes(t *testing.T) {
	ct, _ := ParseContentType([]byte(article))
	if err := ct.Prepare(PluginUID("users-permissions", "article"), "users-permissions"); err != nil {
		t.Fatal(err)
	}
	if ct.UID != "plugin::users-permissions.article" || ct.ModelName != "article" || ct.GlobalID != "UsersPermissionsArticle" || ct.Kind != CollectionType {
		t.Errorf("derived = %s %s %s %s", ct.UID, ct.ModelName, ct.GlobalID, ct.Kind)
	}
	want := "title,status,slug,views,createdAt,updatedAt,publishedAt,createdBy,updatedBy"
	if got := strings.Join(ct.Attributes.Keys(), ","); got != want {
		t.Errorf("attributes = %s, want %s", got, want)
	}
	cb, _ := ct.Attributes.Get(AttrCreatedBy)
	if !cb.IsPrivate() || cb.Target != "admin::user" || cb.IsWritable() {
		t.Errorf("createdBy = %+v", cb)
	}
	if len(ct.Warnings) != 1 || !strings.Contains(ct.Warnings[0], `"status"`) {
		t.Errorf("warnings = %q", ct.Warnings)
	}
}

func TestPrepareCreatorFieldsPublicWithPopulateCreatorFields(t *testing.T) {
	ct, _ := ParseContentType([]byte(`{"collectionName":"a","info":{"singularName":"a","pluralName":"as","displayName":"A"},"options":{"populateCreatorFields":true},"attributes":{}}`))
	if err := ct.Prepare(APIUID("a", "a"), ""); err != nil {
		t.Fatal(err)
	}
	cb, _ := ct.Attributes.Get(AttrUpdatedBy)
	if cb.IsPrivate() {
		t.Error("updatedBy must be public with populateCreatorFields")
	}
	if ct.DraftAndPublish() {
		t.Error("draftAndPublish defaults to false")
	}
	pa, _ := ct.Attributes.Get(AttrPublishedAt)
	if pa == nil {
		t.Error("publishedAt is added even without draft and publish")
	}
}

func TestPrepareRejects(t *testing.T) {
	tests := map[string]string{
		"documentId attribute": `{"collectionName":"a","info":{"singularName":"a","pluralName":"as","displayName":"A"},"attributes":{"documentId":{"type":"string"}}}`,
		"id attribute":         `{"collectionName":"a","info":{"singularName":"a","pluralName":"as","displayName":"A"},"attributes":{"id":{"type":"integer"}}}`,
		"no collectionName":    `{"info":{"singularName":"a","pluralName":"as","displayName":"A"},"attributes":{}}`,
		"not kebab case":       `{"collectionName":"a","info":{"singularName":"myArticle","pluralName":"as","displayName":"A"},"attributes":{}}`,
		"no type":              `{"collectionName":"a","info":{"singularName":"a","pluralName":"as","displayName":"A"},"attributes":{"x":{"required":true}}}`,
	}
	for name, src := range tests {
		ct, err := ParseContentType([]byte(src))
		if err == nil {
			err = ct.Prepare(APIUID("a", "a"), "")
		}
		if err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	// Strapi loads these: only the content-type builder refuses them.
	ok := `{"collectionName":"a","info":{"singularName":"a","pluralName":"as","displayName":"A"},"attributes":{"locale":{"type":"string"},"strapiFoo":{"type":"string"}}}`
	ct, _ := ParseContentType([]byte(ok))
	if err := ct.Prepare(APIUID("a", "a"), ""); err != nil {
		t.Errorf("locale and strapiFoo must load: %v", err)
	}
}

func TestBuilderReservedNames(t *testing.T) {
	for _, n := range []string{"locale", "strapiFoo", "createdAt", "created_by", "meta"} {
		if !IsReservedAttributeName(n, false) {
			t.Errorf("%s must be reserved", n)
		}
	}
	if IsReservedAttributeName("status", false) || !IsReservedAttributeName("status", true) {
		t.Error("status is reserved only with draft and publish")
	}
	if IsReservedAttributeName("title", true) {
		t.Error("title is not reserved")
	}
	if !IsReservedModelName("upload") || !IsReservedModelName("dateTime") || IsReservedModelName("article") {
		t.Error("model names")
	}
}

func TestComponentPrepare(t *testing.T) {
	c, err := ParseComponent([]byte(`{"collectionName":"components_shared_seos","info":{"displayName":"Seo","icon":"search"},"attributes":{"title":{"type":"string"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Prepare("shared", "seo"); err != nil {
		t.Fatal(err)
	}
	if c.UID != "shared.seo" || c.GlobalID != "ComponentSharedSeo" || c.ModelName != "seo" {
		t.Errorf("derived = %s %s %s", c.UID, c.GlobalID, c.ModelName)
	}
	dz, _ := ParseComponent([]byte(`{"collectionName":"c","info":{"displayName":"C"},"attributes":{"blocks":{"type":"dynamiczone","components":["shared.seo"]}}}`))
	if err := dz.Prepare("shared", "c"); err == nil {
		t.Error("dynamic zone in a component must be rejected")
	}
	missing, _ := ParseComponent([]byte(`{"info":{"displayName":"C"},"attributes":{}}`))
	if err := missing.Prepare("shared", "c"); err == nil {
		t.Error("missing collectionName must be rejected")
	}
}

// Every schema of the compat fixtures must survive a parse and a marshal.
func TestFixtureSchemasRoundTrip(t *testing.T) {
	files, _ := filepath.Glob("../compat/fixtures/*/src/api/*/content-types/*/schema.json")
	comps, _ := filepath.Glob("../compat/fixtures/*/src/components/*/*.json")
	if len(files) == 0 {
		t.Fatal("no fixture schema found")
	}
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		ct, err := ParseContentType(src)
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		ctDir := filepath.Dir(f)
		api := filepath.Base(filepath.Dir(filepath.Dir(ctDir)))
		if err := ct.Prepare(APIUID(api, filepath.Base(ctDir)), ""); err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		assertSameJSON(t, f, src, ct)
	}
	for _, f := range comps {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		c, err := ParseComponent(src)
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		if err := c.Prepare(filepath.Base(filepath.Dir(f)), strings.TrimSuffix(filepath.Base(f), ".json")); err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		assertSameJSON(t, f, src, c)
	}
}

func assertSameJSON(t *testing.T, name string, src []byte, v any) {
	t.Helper()
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("%s: marshal: %v", name, err)
	}
	var a, b any
	_ = json.Unmarshal(src, &a)
	_ = json.Unmarshal(out, &b)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("%s: round trip changed the schema\nsource: %s\nresult: %s", name, src, out)
	}
	var srcAttrs, outAttrs struct {
		Attributes OrderedMap[json.RawMessage] `json:"attributes"`
	}
	_ = json.Unmarshal(src, &srcAttrs)
	_ = json.Unmarshal(out, &outAttrs)
	if s, o := strings.Join(srcAttrs.Attributes.Keys(), ","), strings.Join(outAttrs.Attributes.Keys(), ","); s != o {
		t.Errorf("%s: attribute order %s became %s", name, s, o)
	}
}
