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

`server.host`, `server.port`, `api.rest.prefix`, `api.rest.defaultLimit`, `api.rest.maxLimit`, `api.rest.withCount`, `admin.apiToken.salt`, `database.connection` (`client`, `connection.{host,port,database,user,password,schema,filename,connectionString,ssl}`, `pool.{min,max}`, `acquireConnectionTimeout`, `debug`). Their types are checked at start: a wrong type stops with the file and the key.

Without `config/database.*`, grapi falls back to the `DATABASE_*` variables of a generated Strapi project.

## Variables

Each `GRAPI_*` variable also accepts its Strapi name (`internal/config/aliases.go`); when both are set, `GRAPI_*` wins and a warning is logged.

## Start

```sh
grapi start -dir <project> [-config <dir>]
```

`-dir` is the folder holding `src/`; `-config` defaults to `<dir>/config`.
