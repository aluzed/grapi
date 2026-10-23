package grapi_test

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/aluzed/grapi"
	"github.com/aluzed/grapi/document"
	"github.com/aluzed/grapi/query"
)

// exampleProject is a temporary project with a tag type; done removes it.
func exampleProject() (dir string, done func()) {
	dir, _ = os.MkdirTemp("", "grapi-example")
	schemaDir := filepath.Join(dir, "src", "api", "tag", "content-types", "tag")
	_ = os.MkdirAll(schemaDir, 0o750)
	_ = os.WriteFile(filepath.Join(schemaDir, "schema.json"), []byte(`{"kind":"collectionType","collectionName":"tags",
		"info":{"singularName":"tag","pluralName":"tags","displayName":"Tag"},"options":{"draftAndPublish":false},
		"attributes":{"name":{"type":"string"}}}`), 0o600)
	env := map[string]string{"DATABASE_CLIENT": "sqlite", "DATABASE_FILENAME": filepath.Join(dir, "data.db"), "JWT_SECRET": "example"}
	for k, v := range env {
		_ = os.Setenv(k, v)
	}
	return dir, func() {
		for k := range env {
			_ = os.Unsetenv(k)
		}
		_ = os.RemoveAll(dir)
	}
}

// The Document Service, the Query Engine under it and raw SQL, as
// strapi.documents, strapi.db.query and strapi.db.connection.
func ExampleApp_Query() {
	dir, done := exampleProject()
	defer done()
	app := grapi.New(grapi.Options{Dir: dir, Stderr: io.Discard})
	srv, err := app.Setup()
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	ctx := context.Background()

	for _, name := range []string{"go", "retro"} {
		if _, err := app.Documents().Create(ctx, "api::tag.tag", document.Params{"data": map[string]any{"name": name}}); err != nil {
			log.Fatal(err)
		}
	}
	row, _ := app.Query("api::tag.tag").FindOne(ctx, query.Params{Where: map[string]any{"name": "retro"}, Select: []string{"name"}})
	name, _ := row.Get("name")
	fmt.Println(name)

	var n int
	_ = app.SQL().QueryRow(ctx, "select count(*) from "+app.SQL().Quote("tags")).Scan(&n)
	fmt.Println(n)
	// Output:
	// retro
	// 2
}

// strapi.eventHub: the events entries emit, the ones webhooks send.
func ExampleApp_Events() {
	dir, done := exampleProject()
	defer done()
	app := grapi.New(grapi.Options{Dir: dir, Stderr: io.Discard})
	srv, err := app.Setup()
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = srv.Close() }()
	created := make(chan any, 1)
	off := app.Events().On("entry.create", func(payload map[string]any) { created <- payload["uid"] })
	defer off()
	_, _ = app.Documents().Create(context.Background(), "api::tag.tag", document.Params{"data": map[string]any{"name": "go"}})
	fmt.Println("created in", <-created)
	// Output: created in api::tag.tag
}

// The route table, Go routes included: what `go run . routes:list` prints.
func ExampleApp_Routes() {
	dir, done := exampleProject()
	defer done()
	app := grapi.New(grapi.Options{Dir: dir, Stderr: io.Discard})
	app.CoreRouter("api::tag.tag").Only("find")
	app.PublicRoute("GET", "/tags/popular", func(*grapi.Request) error { return nil })
	routes, err := app.Routes()
	if err != nil {
		log.Fatal(err)
	}
	for _, r := range routes {
		if r.Path == "/api/tags" || r.Path == "/api/tags/popular" {
			fmt.Println(r.Method, r.Path)
		}
	}
	// Unordered output:
	// GET /api/tags/popular
	// GET /api/tags
}
