package metadata_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/aluzed/grapi/internal/db/metadata"
	"github.com/aluzed/grapi/internal/db/naming"
	"github.com/aluzed/grapi/schema"
)

func set(t *testing.T, schemas map[string]string) *schema.Set {
	t.Helper()
	s := &schema.Set{}
	names := make([]string, 0, len(schemas))
	for name := range schemas {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		raw := schemas[name]
		ct, err := schema.ParseContentType([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		if err := ct.Prepare(schema.APIUID(name, name), ""); err != nil {
			t.Fatal(err)
		}
		s.AddContentType(ct)
	}
	return s
}

const adminUser = `{"collectionName":"admin_users","info":{"singularName":"user","pluralName":"users","displayName":"User"},"attributes":{}}`

func withAdmin(t *testing.T, s *schema.Set) *schema.Set {
	t.Helper()
	ct, err := schema.ParseContentType([]byte(adminUser))
	if err != nil {
		t.Fatal(err)
	}
	if err := ct.Prepare(schema.AdminUID("user"), "admin"); err != nil {
		t.Fatal(err)
	}
	s.AddContentType(ct)
	return s
}

func TestBuildErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		schemas map[string]string
		want    string
	}{
		{
			"owning oneToMany",
			map[string]string{
				"a": `{"collectionName":"as","info":{"singularName":"a","pluralName":"as","displayName":"A"},"attributes":{"bs":{"type":"relation","relation":"oneToMany","target":"api::b.b","inversedBy":"a"}}}`,
				"b": `{"collectionName":"bs","info":{"singularName":"b","pluralName":"bs","displayName":"B"},"attributes":{"a":{"type":"relation","relation":"manyToOne","target":"api::a.a","mappedBy":"bs"}}}`,
			},
			"cannot be the owner side",
		},
		{
			"unknown target",
			map[string]string{"a": `{"collectionName":"as","info":{"singularName":"a","pluralName":"as","displayName":"A"},"attributes":{"x":{"type":"relation","relation":"manyToMany","target":"api::nope.nope"}}}`},
			"unknown target api::nope.nope",
		},
		{
			"missing inverse",
			map[string]string{"a": `{"collectionName":"as","info":{"singularName":"a","pluralName":"as","displayName":"A"},"attributes":{"x":{"type":"relation","relation":"manyToMany","target":"api::a.a","inversedBy":"nope"}}}`},
			"inversedBy attribute nope not found",
		},
		{
			"duplicate table",
			map[string]string{
				"a": `{"collectionName":"same","info":{"singularName":"a","pluralName":"as","displayName":"A"},"attributes":{}}`,
				"b": `{"collectionName":"same","info":{"singularName":"b","pluralName":"bs","displayName":"B"},"attributes":{}}`,
			},
			`DB table "same" already exists`,
		},
		{
			"oneWay is not a database relation",
			map[string]string{"a": `{"collectionName":"as","info":{"singularName":"a","pluralName":"as","displayName":"A"},"attributes":{"x":{"type":"relation","relation":"oneWay","target":"api::a.a"}}}`},
			"unknown relation",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := metadata.Build(withAdmin(t, set(t, tc.schemas)), nil, naming.Default)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want an error containing %q", err, tc.want)
			}
		})
	}
}

func TestNamingErrorIsReturned(t *testing.T) {
	s := withAdmin(t, set(t, map[string]string{
		"a": `{"collectionName":"as","info":{"singularName":"a","pluralName":"as","displayName":"A"},"attributes":{}}`,
	}))
	if _, err := metadata.Build(s, nil, naming.New(4)); err == nil {
		t.Fatal("a 4 character limit cannot fit the names, Build must fail")
	}
}
