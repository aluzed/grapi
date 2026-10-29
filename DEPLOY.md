# Deploying grapi

grapi is one binary and a project directory (the `src/` of a Strapi project: `api/*/content-types/*/schema.json`, `components/`, `extensions/`) with its `config/` (YAML or JSON, see [`docs/configuration.md`](docs/configuration.md)).

Before exposing a project, apply the [production hardening](#production-hardening) below: several defaults stay permissive for compatibility with Strapi. The [security review](docs/security-review-20261029.md) explains why.

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

## Production hardening

These settings belong to the deployed project, not to grapi: start from what the project really uses, then check each one against a staging copy. The values below are starting points.

`config/plugins.yaml`:

```yaml
upload:
  config:
    sizeLimit: 20971520          # 20 MiB per file
    sharp:
      limitInputPixels: 40000000 # about 160 MB for one decoded RGBA image
    security:
      allowedTypes: [image/jpeg, image/png, image/webp, application/pdf]
users-permissions:
  config:
    jwtManagement: refresh       # or keep legacy-support with revokeOnPasswordChange
    jwt:
      revokeOnPasswordChange: true
    resetPassword:
      expiresIn: 1h
    emailConfirmation:
      expiresIn: 24h
graphql:
  enabled: false                 # or, when the front uses it:
  config:
    depthLimit: 8
    maxLimit: 100
    maxAliases: 20
```

`config/server.yaml`:

```yaml
proxy:
  koa: true
  maxIpsCount: 1                 # the number of proxies in front of grapi
webhooks:
  blockPrivateNetworks: true
http:
  serverOptions:
    requestTimeout: 120000
```

`config/middlewares.yaml`, with the origins of the front and the admin:

```yaml
- strapi::logger
- strapi::errors
- strapi::security
- name: strapi::cors
  config:
    origin: [https://www.example.com, https://cms.example.com]
- strapi::poweredBy
- strapi::query
- strapi::body
- strapi::session
- strapi::favicon
- strapi::public
```

What to check, beyond the files:

- **Uploads.** Only the roles that need it have `plugin::upload.content-api.upload`. An allowed type is accepted; HTML, SVG or JavaScript is refused, including when declared as `text/plain`. List the active files already stored (`.html`, `.svg`, `.js`, `.xml`) and decide what to do with them; grapi deletes nothing. Content that must stay active belongs on another origin, without admin cookies.
- **Webhooks.** Before enabling `blockPrivateNetworks`, list the hooks whose target is internal: they stop. A public hook still delivers; a private address, a name resolving to one or a redirect to one is refused at connection time.
- **CORS.** From a browser, the listed origins log in and use the admin; another origin gets no `Access-Control-Allow-Origin`. The default reflects any origin with credentials.
- **Users-permissions.** Public and Authenticated have no `user.update`, `user.create` or `user.destroy`: `PUT /api/users/:id` changes any user, its role included. A profile page needs a custom route that updates the caller only, on an explicit list of fields. Any public write kept (a newsletter unsubscribe, for example) needs its own justification and protection. Export the roles' permissions, without secrets, after the review.
- **Proxy.** `maxIpsCount` is the number of proxies that append to `X-Forwarded-For`: 1 for Traefik alone. The CMS port must not be reachable except through the proxy, or a client sets its own IP. Two clients behind the proxy then keep separate rate-limit counters.
- **Resources.** Size the container's memory for the largest accepted image (4 bytes per pixel of `limitInputPixels`, plus the formats) times the concurrent uploads; past the limit, an image is stored without formats. Set CPU and memory limits on the container.

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
