<p align="center"><img src="docs/assets/logo.svg" alt="grapi" height="64"></p>

# grapi

A Strapi-compatible headless CMS written in Go.

grapi aims at the same features as Strapi 5 (content types, components and dynamic zones, draft and publish, i18n, REST and GraphQL APIs, users and permissions, media library, admin panel), plus the features Strapi reserves for its Enterprise edition (SSO, audit logs, content history, review workflows, releases). Everything is MIT.

The public content API stays wire-compatible with Strapi 5, so an existing front end keeps working. grapi can also open an existing Strapi 5 database as is.

## Status

Pre-alpha. `grapi start` serves the REST and GraphQL content APIs of a Strapi project, and an admin panel under `/admin` (first admin, login, profile so far; API: [`docs/admin-api.md`](docs/admin-api.md)) (configuration: [`docs/configuration.md`](docs/configuration.md); Go hooks, routes and policies: [`docs/framework.md`](docs/framework.md)). The plan lives in [`docs/feasibility.md`](docs/feasibility.md) and the backlog in [`docs/stories/`](docs/stories).

## Commands

```sh
grapi new <name> [--db sqlite|postgres]      # create a project
grapi develop                                # serve and reload on src/ or config/ changes
grapi develop --open                         # same, and open the admin; a framework project (main.go) is rebuilt and restarted on Go changes
grapi start -dir <project> [-config <dir>]    # serve the content API
grapi report                                 # versions and project facts, without secrets
grapi db:sync --dry-run -dir <project>       # what the start-up schema sync would change (read only)
grapi db:check -dir <project>                # consistency report: schema, documentIds, orphan links, password hashes
grapi api-token:create|list|delete|regenerate # content API tokens
grapi permissions:list|grant|revoke <role>   # users-permissions role permissions
grapi admin:create-user|reset-user-password|list-users  # admins (--email, --password-stdin)
grapi admin:delete-user|active-user|block-user  # --email; --active=false, --unblock; never the last active Super Admin
grapi generate content-type|component|migration --name <n> [--attr name:type[:required]]
grapi openapi [-o file]                      # the content API's OpenAPI document
grapi ts:generate-types [-o dir]             # types/generated/*.d.ts: @strapi/strapi format, and api.d.ts for API clients
grapi develop --types-out <dir>              # the same types, written again on each schema change
grapi routes:list | content-types:list | components:list | middlewares:list | hooks:list
grapi configuration:dump [-f file] | configuration:restore [-f file] [-s replace|merge|keep]
```

`grapi` without arguments prints every command and its flags.

On a database created by Strapi 5, run `db:sync --dry-run` and `db:check` first, on a copy.

## Development

```sh
make build             # dist/grapi, with the admin SPA embedded (needs Bun)
make test              # unit tests
make race              # unit tests with the race detector
make test-integration  # Postgres (GRAPI_TEST_PG_DSN) and SQLite
make lint              # golangci-lint + em-dash and story status checks
make ci                # everything CI would run (GitHub Actions is off): lint, licenses, admin checks, tests, Postgres on the homelab
make e2e-invite        # admin e-mails through Mailpit on the homelab: invitation, password reset, logout, test mail
```

Requires Go 1.26, golangci-lint v2.14.0 and Bun 1.2 (admin, see [`admin/README.md`](admin/README.md)).

## License

[MIT](LICENSE). grapi is a clean-room reimplementation: see [`CONTRIBUTING.md`](CONTRIBUTING.md).

Not affiliated with or endorsed by Strapi Solutions SAS. Strapi is a trademark of Strapi Solutions SAS.
