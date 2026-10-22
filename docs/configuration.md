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

A value that is a single directive keeps the directive's type; a directive inside a longer string is concatenated.

A key that is set but empty counts as set, as with Strapi's `env()`.

## Keys grapi reads today

`server.host`, `server.port`, `api.rest.prefix`, `api.rest.defaultLimit`, `api.rest.maxLimit`, `api.rest.withCount`, `admin.apiToken.salt`, `database.connection` (`client`, `connection.{host,port,database,user,password,schema,filename,connectionString,ssl}`, `pool.{min,max}`, `acquireConnectionTimeout`, `debug`). `plugins.users-permissions.config.{jwtSecret,jwt.expiresIn,jwt.algorithm,register.allowedFields}` (the secret falls back to `JWT_SECRET`; without either, one is generated and appended to `.env`, as Strapi does). Their types are checked at start: a wrong type stops with the file and the key.

`middlewares`: a list of names or `{name, config}`. grapi runs `strapi::security` (`contentSecurityPolicy.directives`, or `false` to drop a header) and `strapi::cors` (`origin`, `methods`, `headers`, `expose`, `credentials`, `maxAge`); leaving one out of the list turns it off, as in Strapi. Without the file, both run with Strapi's defaults.

`plugins.upload.config`: `provider` (`local` by default, or `aws-s3` with `providerOptions` as `@strapi/provider-upload-aws-s3` takes them: `baseUrl`, `rootPath`, `s3Options.{region,endpoint,forcePathStyle,credentials,params.{Bucket,ACL,signedUrlExpires}}`), `sizeLimit` (bytes), `breakpoints`.

`plugins.graphql`: the plugin is on when the project's `package.json` depends on `@strapi/plugin-graphql` (Strapi enables installed plugins) or when `enabled: true`; `enabled: false` turns it off. `config.endpoint` (`/graphql`), `config.defaultLimit` (10), `config.maxLimit` (unlimited), `config.depthLimit` (unlimited), `config.apolloServer.introspection` (on unless `NODE_ENV=production`, as Apollo).

Without `config/database.*`, grapi falls back to the `DATABASE_*` variables of a generated Strapi project.

## Variables

`GRAPI_LOG_LEVEL` (or `STRAPI_LOG_LEVEL`): `debug`, `info` (default), `warn`, `error`. At `debug`, every rejected request is logged with its error.


Each `GRAPI_*` variable also accepts its Strapi name (`internal/config/aliases.go`); when both are set, `GRAPI_*` wins and a warning is logged.

## Start

```sh
grapi start -dir <project> [-config <dir>]
```

`-dir` is the folder holding `src/`; `-config` defaults to `<dir>/config`.
