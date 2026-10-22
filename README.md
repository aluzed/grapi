# grapi

A Strapi-compatible headless CMS written in Go.

grapi aims at the same features as Strapi 5 (content types, components and dynamic zones, draft and publish, i18n, REST and GraphQL APIs, users and permissions, media library, admin panel), plus the features Strapi reserves for its Enterprise edition (SSO, audit logs, content history, review workflows, releases). Everything is MIT.

The public content API stays wire-compatible with Strapi 5, so an existing front end keeps working. grapi can also open an existing Strapi 5 database as is.

## Status

Pre-alpha: nothing usable yet. The plan lives in [`docs/feasibility.md`](docs/feasibility.md) and the backlog in [`docs/stories/`](docs/stories).

## Development

```sh
make build             # dist/grapi
make test              # unit tests
make test-integration  # Postgres (GRAPI_TEST_PG_DSN) and SQLite
make lint              # golangci-lint + em-dash and story status checks
make ci                # everything CI would run (GitHub Actions is off): lint, licenses, tests, Postgres on the homelab
```

Requires Go 1.26 and golangci-lint v2.14.0.

## License

[MIT](LICENSE). grapi is a clean-room reimplementation: see [`CONTRIBUTING.md`](CONTRIBUTING.md).

Not affiliated with or endorsed by Strapi Solutions SAS. Strapi is a trademark of Strapi Solutions SAS.
