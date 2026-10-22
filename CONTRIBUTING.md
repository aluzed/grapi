# Contributing

## Clean-room rules

grapi reimplements Strapi's features, not its code. Full rules (in French): [`docs/clean-room.md`](docs/clean-room.md).

- Strapi's MIT code may be read as a specification of behaviour. Do not translate it line by line.
- Never read code under Strapi's Enterprise license. That is more than the `ee/` folders: every directory whose `LICENSE` file says "Enterprise License" (in 5.59: `core/admin/ee`, `core/core/src/ee`, `core/review-workflows`, `core/content-releases`, `core/content-manager/{admin,server}/src/{history,preview}`). Enterprise features are specified from Strapi's public documentation only.
- Every dependency must have a license compatible with MIT.

## Conventions

- Commits follow Conventional Commits (`feat(db): ...`, `fix(api): ...`).
- Comments explain a non-obvious *why* only. No ticket numbers in code.
- No em-dash (U+2014) anywhere: `make check-emdash` enforces it.
- Docs are updated in the same change as the code.
