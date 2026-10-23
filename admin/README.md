# admin

The admin SPA: Vite, React, TypeScript (strict), Tailwind, React Query, React Hook Form and Zod. It talks to the admin API `/admin/api/v1` (`docs/admin-api.md`).

## Build

Bun builds it; the Go binary embeds `admin/dist` with the `adminui` build tag:

```sh
make build          # bun install + vite build + go build -tags adminui
make admin-check    # typecheck + bun tests (part of make ci)
```

A binary built without the tag (plain `go build`) serves a placeholder page under `/admin`.

## Develop

```sh
grapi start                         # the API, on :1337
cd admin && bun run dev             # the SPA with hot reload, /admin/api proxied to GRAPI_URL (http://localhost:1337)
```

## Notes

- The build uses relative URLs (`base: './'`); the server injects `<base href="<admin.url>/">` into `index.html`, so one build works under any admin path.
- The access token lives in memory only; the refresh token is an httpOnly cookie. A 401 triggers one refresh (shared by the concurrent requests) and the request is retried.
- UI conventions: see `docs/stories/admin/ADM-002-shell-admin.md`.
