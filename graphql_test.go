package grapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aluzed/grapi"
)

func gql(t *testing.T, url, query string) (int, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"query": query})
	res, err := http.Post(url+"/graphql", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, out
}

func TestGraphQLExtensions(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "graphql", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o750); err != nil {
		t.Fatal(err)
	}
	conf := "graphql:\n  enabled: true\n  config:\n    depthLimit: 2\n    maxAliases: 3\n"
	if err := os.WriteFile(filepath.Join(dir, "config", "plugins.yaml"), []byte(conf), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	app := grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}})
	gq := app.GraphQL()
	gq.ShadowCRUD("api::article.article").Field("code").Disable()
	gq.ShadowCRUD("api::tag.tag").DisableAction("delete")
	no := false
	gq.Use(grapi.GraphQLExtension{
		TypeDefs: "type PopularTag { name: String! score: Int! }\nextend type Query { popularTags: [PopularTag!]! hello: String }",
		Resolvers: map[string]map[string]grapi.GraphQLResolveFunc{"Query": {
			"popularTags": func(context.Context, *grapi.GraphQLFieldContext) (any, error) {
				return []any{map[string]any{"name": "go", "score": 3}}, nil
			},
			"hello": func(context.Context, *grapi.GraphQLFieldContext) (any, error) { return "world", nil },
		}},
		ResolversConfig: map[string]grapi.GraphQLResolverConfig{
			"Query.popularTags": {Scope: "api::tag.tag.find"},
			"Query.hello": {Auth: &no, Middlewares: []grapi.GraphQLMiddlewareFunc{
				func(_ context.Context, _ *grapi.GraphQLFieldContext, next func() (any, error)) (any, error) {
					v, err := next()
					return fmt.Sprint(v, "!"), err
				},
			}},
			"Query.homepage": {Auth: &no, Policies: []grapi.GraphQLPolicyFunc{
				func(context.Context, *grapi.GraphQLFieldContext) (bool, error) { return false, nil },
			}},
		},
	})
	srv, err := app.Setup()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	_, out := gql(t, ts.URL, `{ hello }`)
	if out["data"].(map[string]any)["hello"] != "world!" {
		t.Errorf("hello: %v", out)
	}
	_, out = gql(t, ts.URL, `{ popularTags { name } }`)
	if !strings.Contains(jsonOf(out), `"code":"FORBIDDEN"`) {
		t.Errorf("popularTags is scoped: %v", jsonOf(out))
	}
	_, out = gql(t, ts.URL, `{ homepage { title } }`)
	if !strings.Contains(jsonOf(out), `"name":"PolicyError"`) {
		t.Errorf("policy: %v", jsonOf(out))
	}
	status, out := gql(t, ts.URL, `query Deep { articles { tags { articles { title } } } }`)
	if status != 400 || !strings.Contains(jsonOf(out), `'Deep' exceeds maximum operation depth of 2`) {
		t.Errorf("depth: %d %v", status, jsonOf(out))
	}
	status, out = gql(t, ts.URL, `query Wide { a: hello b: hello ...more } fragment more on Query { c: hello d: hello }`)
	if status != 400 || !strings.Contains(jsonOf(out), `'Wide' exceeds the limit of 3 aliases`) {
		t.Errorf("aliases: %d %v", status, jsonOf(out))
	}
	_, out = gql(t, ts.URL, `{ a: __type(name: "Article") { fields { name } } m: __type(name: "Mutation") { fields { name } } f: __type(name: "ArticleFiltersInput") { inputFields { name } } }`)
	s := jsonOf(out)
	if strings.Contains(s, `"code"`) {
		t.Errorf("code is disabled: %s", s)
	}
	if strings.Contains(s, `"deleteTag"`) || !strings.Contains(s, `"updateTag"`) {
		t.Errorf("only deleteTag is disabled: %s", s)
	}
}

func jsonOf(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// GraphQL answers go through the content API output sanitization, as REST:
// a password field reads null and the registered output sanitizers apply.
func TestGraphQLSanitizesOutput(t *testing.T) {
	dir := t.TempDir()
	if err := copyDir(filepath.Join("compat", "fixtures", "graphql", "src"), filepath.Join(dir, "src")); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "src", "api", "article", "content-types", "article", "schema.json")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	raw = []byte(strings.Replace(string(raw), `"attributes": {`, `"attributes": {
    "pin": { "type": "password" },`, 1))
	if err := os.WriteFile(p, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config", "plugins.yaml"), []byte("graphql:\n  enabled: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_CLIENT", "sqlite")
	t.Setenv("DATABASE_FILENAME", filepath.Join(dir, "data.db"))
	t.Setenv("JWT_SECRET", "test")
	app := grapi.New(grapi.Options{Dir: dir, Stderr: &bytes.Buffer{}})
	app.AddSanitizer("content-api.output", func(uid string, e map[string]any) map[string]any {
		if uid == "api::article.article" {
			e["summary"] = "sanitized"
		}
		return e
	})
	no := false
	app.GraphQL().Use(grapi.GraphQLExtension{ResolversConfig: map[string]grapi.GraphQLResolverConfig{
		"Query.articles":         {Auth: &no},
		"Mutation.createArticle": {Auth: &no},
	}})
	srv, err := app.Setup()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	_, out := gql(t, ts.URL, `mutation { createArticle(data: {title: "Hi", summary: "raw", pin: "1234"}, status: PUBLISHED) { title pin summary } }`)
	created, _ := out["data"].(map[string]any)["createArticle"].(map[string]any)
	if created == nil || created["pin"] != nil || created["summary"] != "sanitized" {
		t.Fatalf("mutation output: %s", jsonOf(out))
	}
	_, out = gql(t, ts.URL, `{ articles { title pin summary } }`)
	list, _ := out["data"].(map[string]any)["articles"].([]any)
	if len(list) != 1 {
		t.Fatalf("articles: %s", jsonOf(out))
	}
	if a := list[0].(map[string]any); a["pin"] != nil || a["summary"] != "sanitized" {
		t.Fatalf("query output: %s", jsonOf(out))
	}
}
