# Deploying grapi

grapi is one binary and a project directory (the `src/` of a Strapi project: `api/*/content-types/*/schema.json`, `components/`, `extensions/`) with its `config/` (YAML or JSON, see [`docs/configuration.md`](docs/configuration.md)).

Before exposing a project, review the [security findings and production settings](docs/security-review-20261029.md), particularly upload types, webhook network access, CORS origins and users-permissions grants.

Before pointing grapi at an existing Strapi database, run on a copy:

```sh
grapi db:sync --dry-run -dir <project>   # must print UNCHANGED
grapi db:check -dir <project>            # must print OK
```

## Docker

```sh
make docker                 # builds grapi:<version> and grapi:latest on the homelab (never locally)
docker run -d --name grapi \
  -p 1337:1337 \
  -v /srv/myproject:/app:ro \            # src/ and config/
  -v /srv/myproject-uploads:/app/public/uploads \
  -e DATABASE_CLIENT=postgres -e DATABASE_HOST=db -e DATABASE_NAME=strapi \
  -e DATABASE_USERNAME=strapi -e DATABASE_PASSWORD=... \
  -e JWT_SECRET=... \
  grapi:latest
```

- Runs as uid 1000; the uploads volume must belong to it.
- `HEALTHCHECK` calls `grapi health` (GET `/_health`, 204).
- Volumes: `/app/public/uploads` (local upload provider), `/app/data` (SQLite file when `DATABASE_CLIENT=sqlite`, with `DATABASE_FILENAME=/app/data/data.db`).
- With the project mounted read-only, set `JWT_SECRET`: without it grapi generates one and appends it to `/app/.env`, as Strapi does.
- The image is the pure Go build: no WebP formats and no EXIF orientation until the libvips variant (`MEDIA-002`).

## Binary and systemd

```sh
make build                  # dist/grapi, static (CGO_ENABLED=0 works too)
```

```ini
# /etc/systemd/system/grapi.service
[Unit]
Description=grapi
After=network-online.target postgresql.service

[Service]
User=grapi
WorkingDirectory=/srv/myproject
EnvironmentFile=/srv/myproject/.env
ExecStart=/usr/local/bin/grapi start -dir /srv/myproject
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

## Dokploy

Create an application from the repository with the Dockerfile build type, mount the project directory and the uploads volume as above, set the `DATABASE_*` and `JWT_SECRET` variables in the environment tab, and point the domain at port 1337. Keep port 1337 off the public interface: Docker publishes ports past ufw, so expose it through Traefik only.

## Minimal environment

| Variable | Use |
|---|---|
| `DATABASE_CLIENT` | `postgres` or `sqlite` (without `config/database.*`) |
| `DATABASE_HOST`, `DATABASE_PORT`, `DATABASE_NAME`, `DATABASE_USERNAME`, `DATABASE_PASSWORD` | Postgres |
| `DATABASE_FILENAME` | SQLite |
| `JWT_SECRET` | users-permissions tokens |
| `API_TOKEN_SALT` | API tokens, when `config/admin.*` does not set `apiToken.salt` |
| `HOST`, `PORT` | listen address (`0.0.0.0:1337`) |
| `GRAPI_ENV` / `NODE_ENV` | picks `config/env/<env>/` |
| `GRAPI_LOG_LEVEL` | `debug`, `info`, `warn`, `error` |
