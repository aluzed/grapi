# Framework mode

grapi runs a Strapi project as is (`grapi start`). To add Go code (what a Strapi project does in `src/index.js`, lifecycles, custom controllers, policies), build your own `main` on the `github.com/aluzed/grapi` module. `grapi new <name> --framework` writes one:

```go
app := grapi.New(grapi.Options{Dir: "."})
// register hooks, routes, policies... here, then:
if err := app.Start(); err != nil {
	log.Fatal(err)
}
```

`grapi generate` writes the Go skeletons, each in a file of its own that adds a setup function in `init`; the generated `main.go` applies them to the App before `Start`:

- `grapi generate policy --name is-owner` (`global::is-owner`, or a full uid),
- `grapi generate route --method POST --path /account/anonymize --action api::account.account.anonymize` (`--public` for `config.auth: false`),
- `grapi generate document-middleware --name audit [--uid api::post.post]`,
- `grapi generate lifecycle --model api::post.post --event create|update|write|delete`.

The files are gofmt'ed and pass `go vet`; an existing file is never overwritten. `grapi new <name> --framework` also writes the `go.mod`; run `go get github.com/aluzed/grapi` once.

Everything below is registered before `Start` (or `Setup`, which returns the HTTP handler without listening, for tests).

## Phases (`src/index.js` register, bootstrap, destroy)

```go
app.OnRegister(func(ctx context.Context) error { return nil })  // before the database is synced
app.OnBootstrap(func(ctx context.Context) error {               // services ready, before the server answers
	_, err := app.Documents().Create(ctx, "api::tag.tag", document.Params{"data": map[string]any{"name": "seeded"}})
	return err
})
app.OnDestroy(func(ctx context.Context) error { return nil })   // when the server closes
```

An error in `OnRegister` or `OnBootstrap` stops the start.

## Hooks (`strapi.hook`)

```go
after := app.Hooks().Get(hook.AfterSync).(*hook.Parallel[hook.SyncEvent])
off := after.Register(func(ctx context.Context, ev hook.SyncEvent) (hook.SyncEvent, error) { return ev, nil })
app.Hooks().Register("plugin::my-plugin.ready", &hook.Series[string]{})
```

`hook.Series`, `hook.Waterfall`, `hook.Parallel` and `hook.Bail` are Strapi's four hook kinds. `hook.BeforeSync` runs before the schema sync, `hook.AfterSync` once the services are ready (so it can seed data), before `OnBootstrap`.

## Row lifecycles (`db.lifecycles.subscribe`)

```go
app.OnRowCreate("api::post.post").Bind(func(e *hook.RowEvent) error {
	e.Data["slug"] = slugify(e.String("title"))
	return nil
})
```

`OnRowCreate`, `OnRowUpdate`, `OnRowWrite` (both), `OnRowDelete`, or `OnRow("afterUpdate", uids...)` for any of the 18 Strapi events. No uid means every model. An error aborts the write.

## Document Service middlewares (`strapi.documents.use`)

```go
app.UseDocuments(func(c *document.Context, next document.Next) (any, error) {
	if c.UID == "api::post.post" && c.Action == document.Create {
		c.Params["status"] = "draft"
	}
	return next()
})
```

`app.Documents()` gives the Document Service itself once set up.

## Routes and policies

```go
app.Route("POST", "/account/anonymize", "api::account.account.anonymize", func(c *grapi.Request) error {
	if c.User == nil {
		return grapi.Error(http.StatusUnauthorized, "")
	}
	return c.JSON(http.StatusOK, map[string]any{"ok": true})
}).Policies("is-owner")

app.PublicRoute("GET", "/ping", ping) // config.auth: false
```

`app.MutateOpenAPI(func(doc map[string]any) {...})` changes the whole document before it is served. `.OpenAPI(operation)` adds the route to the OpenAPI document (an operation object: `operationId`, `parameters`, `requestBody`, `responses`). The path is under the API prefix (`/api`). The scope is the permission a role needs, granted like any Strapi action. `grapi.Error(status, message)` answers in Strapi's error format.

Policies run after authentication and the permission check:

```go
app.Policy("api::account.is-owner", func(c *grapi.PolicyContext, config any) (bool, error) {
	return c.User != nil && c.Params["id"] == fmt.Sprint(c.User["id"]), nil
})
app.RoutePolicies("api::post.post.update", "global::is-editor", grapi.PolicyRef{Name: "global::quota", Config: 10})
```

A name without namespace is looked up in the route's API or plugin (`is-owner` on an `api::account...` route is `api::account.is-owner`), then in `global::`. An unknown policy stops the start. `false` answers Strapi's 403 `PolicyError`.

## Errors (`@strapi/utils` errors)

A route handler, a policy or a hook returns Strapi's error classes, wrapped or not: `grapi.ApplicationError`, `ValidationError`, `PaginationError`, `NotFoundError`, `ForbiddenError`, `PolicyError`, `UnauthorizedError`, `PayloadTooLargeError`, `RateLimitError`, `NotImplementedError`. An empty message is Strapi's default; an optional argument sets the details. The answer is Strapi's `{data: null, error: {status, name, message, details}}`.

```go
app.OnRowCreate("api::tag.tag").Bind(func(*hook.RowEvent) error {
	return grapi.ValidationError("Tags are closed")
})
```

`grapi.Error(status, message)` is the `ctx.badRequest` family: the http-errors name of the status.

## Sanitizers (`strapi.sanitizers.add`)

```go
app.AddSanitizer("content-api.output", func(uid string, entry map[string]any) map[string]any {
	if uid == "api::post.post" {
		delete(entry, "internalScore")
	}
	return entry
})
```

`content-api.input` gets the data of a write, `content-api.output` each top-level entry of a response, after the built-in rules (private fields, permissions), in registration order.

Route middlewares (`config.middlewares`) run after the authentication and the policies, in declaration order, around the handler:

```go
app.Route("GET", "/posts/popular", "api::post.post.popular", popular).
	Middlewares(func(c *grapi.Request, next func() error) error {
		c.W.Header().Set("Cache-Control", "max-age=60")
		return next()
	})
```

## Core router and controller (`createCoreRouter`, `createCoreController`)

```go
app.CoreRouter("api::tag.tag").Except("delete") // or Only("find", "findOne")
app.OverrideCoreAction("api::post.post", "find", func(c *grapi.Request, next func() (map[string]any, error)) (map[string]any, error) {
	body, err := next() // super.find: {data, meta}
	if err != nil {
		return nil, err
	}
	body["meta"].(map[string]any)["extra"] = true
	return body, nil
})
```

A removed action answers 405 when its path serves other methods, as Koa's `allowedMethods`. An overridden action keeps the core status; its keys are re-encoded in name order.

## Upload and email providers

```go
app.UseUploadProvider("my-storage", myStorage) // grapi.UploadProvider: Upload, Delete, IsPrivate, SignedURL
app.UseEmailProvider("my-mailer", myMailer)    // grapi.EmailProvider: Send(ctx, grapi.EmailMessage)
```

They are used when `plugins.upload.config.provider` or `plugins.email.config.provider` names them, like an installed Strapi provider. An upload provider sets `UploadFile.URL`, and `UploadFile.Metadata` when it needs it back on delete (`provider_metadata`). A provider with a `CheckFileSize(f *grapi.UploadFile, sizeLimit int64) error` method replaces the `sizeLimit` check, as `checkFileSize` does in Strapi.

## Cron (`server.cron.tasks`)

```go
app.Cron("cleanup", "0 3 * * *", grapi.CronOptions{TZ: "Europe/Paris"}, func(ctx context.Context) error { return nil })
app.CronAt("launch", time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC), launch)
```

Tasks run only when `server.cron.enabled` is true, as in Strapi. Around daylight saving time changes, rules follow the wall clock as Strapi's croner does: a time that does not exist runs once the clock has jumped (02:30 runs at 03:30), a time that happens twice runs once, at its first occurrence.

## GraphQL (`strapi.plugin('graphql').service('extension')`)

```go
gql := app.GraphQL()
gql.ShadowCRUD("api::post.post").Field("internalNote").Disable()
gql.ShadowCRUD("api::tag.tag").DisableAction("delete")
gql.Use(grapi.GraphQLExtension{
	TypeDefs: `extend type Query { popularPosts(limit: Int = 5): [Post!]! }`,
	Resolvers: map[string]map[string]grapi.GraphQLResolveFunc{"Query": {
		"popularPosts": func(ctx context.Context, fc *grapi.GraphQLFieldContext) (any, error) { return popular(ctx, fc.Args["limit"]) },
	}},
	ResolversConfig: map[string]grapi.GraphQLResolverConfig{
		"Query.popularPosts": {Scope: "api::post.post.find"},
	},
})
```

The plugin is on when the project depends on `@strapi/plugin-graphql` or sets `plugins.graphql.enabled` (`docs/configuration.md`). Fields of `Query` and `Mutation` always go through the permission check; an `Auth` pointing to false lifts it, `Middlewares` wrap the resolver (they call `next()`), `Policies` run after them, on added and on generated fields alike.
