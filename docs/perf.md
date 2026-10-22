# Performance: grapi and Strapi 5.59

Measured on the homelab with `make bench-compat FIXTURE=devfreelance MODE=perf DB=postgres` (2026-10-22).

## Method

- Strapi 5.59.0 (Node 24, the bench's reference container) and grapi (native binary) each serve their own Postgres 16 database, seeded by the same corpus run; the responses of both are identical (the bench passes 123/123 just before).
- Load: the variable free GET cases of the dev-freelance corpus (the calls of the front: home, article, category, theme, media, sitemap lists, with filters, populate and pagination), round robin, 32 concurrent workers, 30 s, after a 5 s warm-up at 8 workers.
- Memory: `docker stats` usage for the Strapi container, `VmRSS` for the grapi process, sampled every 0.5 s.

## Results

| Engine | req/s | p50 ms | p95 ms | p99 ms | errors | memory idle | memory peak | memory 10 s after |
|---|---|---|---|---|---|---|---|---|
| Strapi 5.59 (Node) | 326 | 98.7 | 114.5 | 124.6 | 0 | 475 MiB | 781 MiB | 259 MiB |
| grapi | 5569 | 5.2 | 10.6 | 13.0 | 0 | 32 MiB | 43 MiB | 42 MiB |

About 17 times the throughput, a tenth of the latency, a tenth to a twentieth of the memory.

### Copy of the dev-freelance production database

Same method on a copy of the production database (718 posts, 864 files; homelab, deleted after the run), with the 32 variable free GET cases of `compat/corpus/devfreelance-prod.yaml`; Strapi is the reference image with dev-freelance's schemas (default controllers on both sides).

| Engine | req/s | p50 ms | p95 ms | p99 ms | errors | memory idle | memory peak | memory 10 s after |
|---|---|---|---|---|---|---|---|---|
| Strapi 5.59 (Node) | 242 | 138.4 | 159.5 | 172.4 | 0 | 416 MiB | 744 MiB | 193 MiB |
| grapi | 5211 | 5.1 | 15.3 | 19.6 | 0 | 28 MiB | 47 MiB | 45 MiB |

## Limits

- Strapi runs in a container and grapi natively, on the same machine: no CPU limit on either.
- Strapi keeps its default middlewares (request logger included); grapi logs at `info`.
- Strapi under Bun is not measured here (see `docs/stories/migration/MIG-002-recette-dev-freelance.md`).
