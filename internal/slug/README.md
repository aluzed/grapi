# slug

Reproduces `@sindresorhus/slugify` 1.1.0 as used by Strapi 5.59. `tables_gen.go` is generated from the reference image:

```sh
scp compat/tools/gen-slug-tables.js homelab:/tmp/ && ssh homelab 'docker run --rm --entrypoint node -w /app -v /tmp/gen-slug-tables.js:/app/gen-slug-tables.js:ro <reference-image> /app/gen-slug-tables.js' > internal/slug/tables_gen.go && gofmt -w internal/slug/tables_gen.go
```

The tables come from MIT-licensed packages; their notices are in `THIRD_PARTY_LICENSES`.
