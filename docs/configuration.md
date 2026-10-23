# Configuration

grapi reads the same configuration keys as Strapi 5, from declarative files instead of `config/*.js`.

## Files

- `config/<name>.yaml`, `.yml` or `.json`. `config/*.js` and `config/*.ts` are skipped silently until JS hooks land (EXT-003).
- `config/env/<environment>/<name>.*` overrides `config/<name>.*` (lodash `merge`: objects merge deeply, arrays index by index).
- The environment is `GRAPI_ENV` (or `NODE_ENV`), `development` by default.
- `.env` at the project root (or the file named by `GRAPI_ENV_PATH` / `ENV_PATH`) is loaded first and never overrides a variable already set.
- Strapi's restricted names (`uuid`, `license`, `telemetry`, `strapi`, `internal`, `dirs`, `middleware`, `plugin`, ...) are refused with a warning.

## Directives

| Directive | Strapi equivalent |
|---|---|
| `${env:KEY}` / `${env:KEY:default}` | `env('KEY', 'default')` |
| `${env.int:KEY:1337}` | `env.int('KEY', 1337)` (`parseInt`) |
| `${env.float:KEY:1.5}` | `env.float` (`parseFloat`) |
| `${env.bool:KEY:false}` | `env.bool`: true only for the exact value `true` |
| `${env.json:KEY}` | `env.json`: invalid JSON fails the start |
| `${env.array:KEY}` | `env.array`: comma split, surrounding `[]`, spaces and quotes removed |
| `${env.date:KEY}` | `env.date` |
| `${env.oneOf:KEY:a\|b\|c:default}` | `env.oneOf`: a default outside the list fails the start |
| `${file:path/to/file}` | `fs.readFileSync` (relative to the project root; missing file fails the start) |

A value that is a single directive keeps the directive's type; a directive inside a longer string is concatenated. Directives nest, innermost first: `${file:dkim/${env:DKIM_PRIVATE_KEY}}` reads the file the variable names, as `path.resolve(env('STRAPI_PATH'), 'dkim', env('DKIM_PRIVATE_KEY'))` would.

A key that is set but empty counts as set, as with Strapi's `env()`.

## Keys grapi reads today

`server.host`, `server.port`, `api.rest.prefix`, `api.rest.defaultLimit`, `api.rest.maxLimit`, `api.rest.withCount`, `admin.apiToken.salt`, `database.connection` (`client`, `connection.{host,port,database,user,password,schema,filename,connectionString,ssl}`, `pool.{min,max}`, `acquireConnectionTimeout`, `debug`). `plugins.users-permissions.config.{jwtSecret,jwt.expiresIn,jwt.algorithm,register.allowedFields}` (the secret falls back to `JWT_SECRET`; without either, one is generated and appended to `.env`, as Strapi does). Their types are checked at start: a wrong type stops with the file and the key.

`server.openapi['content-api']`: `access: public` serves the content API's OpenAPI 3.1 document on `<api prefix>/openapi.json` (`route.path` to move it), as Strapi 5.59 does; `disabled` (the default) does not. The document is Strapi's for the same project, limited to the routes grapi serves. `grapi openapi [-o file]` writes it without starting the server.

`server.socket`: a Unix socket path to listen on instead of `server.host` and `server.port`, as Strapi. `server.http.serverOptions`: Node's `requestTimeout`, `headersTimeout` and `keepAliveTimeout` (milliseconds) become the server's read, header and idle timeouts; without them, headers must arrive within 10 seconds and the rest has no limit.

`server.proxy`: `koa: true` trusts the proxy headers, as Koa does for Strapi: the client IP (rate limits, audit logs) is the first address of `ipHeader` (`X-Forwarded-For` by default), after keeping only the last `maxIpsCount` ones when it is set, and `X-Forwarded-Proto: https` marks the admin cookie `Secure`. Strapi 5 reads `server.proxy.koa` only, so `proxy: true` trusts nothing (grapi logs a warning).

`admin.rateLimit`: `enabled` (true), `interval` (5 minutes, in ms or as `{min: 5}`), `max` (5), as Strapi: admin login and register-admin are limited per email, path and client IP, past the limit with Strapi's 429 `RateLimitError`. `plugins.users-permissions.config.ratelimit` does the same for the users-permissions auth routes (1 minute, 10).

`server.webhooks`: `retries` (3; Strapi makes none), `defaultHeaders`, and `blockPrivateNetworks` (false), a grapi option that refuses deliveries to loopback, private and link-local addresses.

`api.documents.strictRelations` (false by default): required relations and media are enforced on non-draft writes, as in Strapi. A value other than a boolean stops the start (Strapi fails on the first write).

`middlewares`: a list of names or `{name, config}`. `strapi::body` always runs with koa-body's limits (`jsonLimit` 1mb, `formLimit` and `textLimit` 56kb, as a number of bytes or `"2mb"`): a larger body gets Strapi's 413 before routing. Its `formidable.maxFileSize` (200 MiB, formidable's default) caps the file data of one multipart request, as Strapi: files stream to temporary files and the upload stops with `413 FileTooBig` as soon as the cap is crossed, so neither memory nor disk takes the rest. The upload plugin's `sizeLimit` then applies per file. grapi runs `strapi::security` (`contentSecurityPolicy.directives`, or `false` to drop a header) and `strapi::cors` (`origin`, `methods`, `headers`, `expose`, `credentials`, `maxAge`); as in Strapi, the list must name `strapi::errors`, `strapi::security`, `strapi::cors`, `strapi::query`, `strapi::body`, `strapi::public` and `strapi::favicon`, or the start stops with Strapi's message naming the missing ones. Without the file, all run with Strapi's defaults.

`plugins.email.config`: `provider` (`sendmail` by default, `nodemailer` with `providerOptions.{host,port,secure,auth.user,auth.pass,dkim.*}`, `sendgrid` with `providerOptions.apiKey`, `mailgun` with `providerOptions.{key,username,url,domain}`, `amazon-ses` with `providerOptions.{key,secret,amazon}`), `settings.{defaultFrom,defaultReplyTo}`.

`plugins.upload.config`: `provider` (`local` by default, `cloudinary` with `providerOptions.{cloud_name,api_key,api_secret}` and `actionOptions.{upload,delete}`, or `aws-s3` with `providerOptions` as `@strapi/provider-upload-aws-s3` takes them: `baseUrl`, `rootPath`, `s3Options.{region,endpoint,forcePathStyle,credentials,params.{Bucket,ACL,signedUrlExpires}}`), `sizeLimit` (bytes), `breakpoints`.

`plugins.graphql`: the plugin is on when the project's `package.json` depends on `@strapi/plugin-graphql` (Strapi enables installed plugins) or when `enabled: true`; `enabled: false` turns it off. `config.endpoint` (`/graphql`), `config.defaultLimit` (10), `config.maxLimit` (unlimited), `config.depthLimit` (unlimited), `config.shadowCRUD` (true; false generates nothing from the content types, the schema is then the extensions' only), `config.apolloServer.introspection` (on unless `NODE_ENV=production`, as Apollo).

Without `config/database.*`, grapi falls back to the `DATABASE_*` variables of a generated Strapi project.

## Variables

`GRAPI_LOG_LEVEL` (or `STRAPI_LOG_LEVEL`): `debug`, `http` (default, as Strapi: one line per request), `info`, `warn`, `error`. At `debug`, every rejected request is logged with its error.

`GRAPI_LOG_FORMAT` (or `logger.format` in the config): `text`, or `json` (the default in production). Every request line carries its method, path, status, duration and request id: the `X-Request-Id` header when the client or a proxy sends a valid one, a generated one otherwise, sent back in the response. grapi sends no telemetry and opens no outgoing connection it was not configured for; `STRAPI_TELEMETRY_DISABLED` is accepted and has nothing to disable.


Each `GRAPI_*` variable also accepts its Strapi name (`internal/config/aliases.go`); when both are set, `GRAPI_*` wins and a warning is logged.

## Start

```sh
grapi start -dir <project> [-config <dir>]
```

`-dir` is the folder holding `src/`; `-config` defaults to `<dir>/config`.
