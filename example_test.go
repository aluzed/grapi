package grapi_test

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	"github.com/aluzed/grapi"
	"github.com/aluzed/grapi/document"
	"github.com/aluzed/grapi/hook"
)

// A framework project: the App with its Go code, set up without listening.
func ExampleApp_Setup() {
	dir, _ := os.MkdirTemp("", "grapi-example")
	defer func() { _ = os.RemoveAll(dir) }()
	_ = os.MkdirAll(filepath.Join(dir, "src"), 0o750)
	for k, v := range map[string]string{"DATABASE_CLIENT": "sqlite", "DATABASE_FILENAME": filepath.Join(dir, "data.db"), "JWT_SECRET": "example"} {
		_ = os.Setenv(k, v)
		defer func() { _ = os.Unsetenv(k) }()
	}

	app := grapi.New(grapi.Options{Dir: dir, Stderr: io.Discard})
	app.PublicRoute("GET", "/ping", func(c *grapi.Request) error {
		return c.JSON(http.StatusOK, map[string]any{"pong": true})
	})
	srv, err := app.Setup()
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = srv.Close() }()

	rec := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rec, httptest.NewRequest("GET", "/api/ping", nil))
	fmt.Println(rec.Code, strings.TrimSpace(rec.Body.String()))
	// Output: 200 {"pong":true}
}

// A row lifecycle, Strapi's db.lifecycles.subscribe: the slug of every
// written post comes from its title. An error aborts the write.
func ExampleApp_OnRowCreate() {
	app := grapi.New(grapi.Options{Dir: "."})
	app.OnRowCreate("api::post.post").Bind(func(e *hook.RowEvent) error {
		e.Data["slug"] = strings.ToLower(strings.ReplaceAll(e.String("title"), " ", "-"))
		return nil
	})
}

// A Document Service middleware, Strapi's strapi.documents.use.
func ExampleApp_UseDocuments() {
	app := grapi.New(grapi.Options{Dir: "."})
	app.UseDocuments(func(c *document.Context, next document.Next) (any, error) {
		if c.UID == "api::post.post" && c.Action == document.Create {
			if data, ok := c.Params["data"].(map[string]any); ok && data["views"] == nil {
				data["views"] = 0
			}
		}
		return next()
	})
}

// A custom route with its permission scope, and a policy on it.
func ExampleApp_Route() {
	app := grapi.New(grapi.Options{Dir: "."})
	app.Policy("global::is-authenticated", func(c *grapi.PolicyContext, _ any) (bool, error) {
		return c.User != nil, nil
	})
	app.Route("POST", "/account/anonymize", "api::account.account.anonymize", func(c *grapi.Request) error {
		return c.JSON(http.StatusOK, map[string]any{"ok": true})
	})
	app.RoutePolicies("api::account.account.anonymize", "global::is-authenticated")
}

// A cron task, Strapi's server.cron.tasks.
func ExampleApp_Cron() {
	app := grapi.New(grapi.Options{Dir: "."})
	app.Cron("cleanup", "0 3 * * *", grapi.CronOptions{TZ: "Europe/Paris"}, func(ctx context.Context) error {
		_, err := app.Documents().Count(ctx, "api::post.post", document.Params{})
		return err
	})
}

// Bootstrap code, src/index.js bootstrap: the services are ready, so it can
// seed. AfterSync, a Strapi hook, sees the content types of the sync.
func ExampleApp_OnBootstrap() {
	app := grapi.New(grapi.Options{Dir: "."})
	app.OnBootstrap(func(ctx context.Context) error {
		n, err := app.Documents().Count(ctx, "api::category.category", document.Params{})
		if err != nil || n > 0 {
			return err
		}
		_, err = app.Documents().Create(ctx, "api::category.category", document.Params{"data": map[string]any{"name": "News"}})
		return err
	})
	if h, ok := app.Hooks().Get(hook.AfterSync).(*hook.Parallel[hook.SyncEvent]); ok {
		h.Register(func(_ context.Context, e hook.SyncEvent) (hook.SyncEvent, error) {
			log.Printf("%d content types synced", len(e.ContentTypes))
			return e, nil
		})
	}
}
