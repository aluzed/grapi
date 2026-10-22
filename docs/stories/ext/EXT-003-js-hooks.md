---
status: todo
---

# EXT-003 : Hooks JS/TS embarqués (esbuild + goja)

## Contexte
Phase 3. Pour faciliter la migration d'un projet Strapi (`MIG-001`), grapi peut exécuter une partie du code utilisateur JS/TS sans Node : esbuild (écrit en Go, utilisable comme bibliothèque) transpile le TS et bundle les dépendances npm pures, puis goja (moteur JS en Go pur) exécute le résultat (décision D2, niveau 3).

## Objectif
Charger et exécuter les fichiers JS/TS de Strapi qui n'ont pas besoin de Node : configuration, lifecycles, middlewares du Document Service, policies, controllers simples, tâches cron.

## Périmètre
- Fichiers pris en charge : `config/*.{js,ts}` (avec `env` fourni), `src/index.{js,ts}` (`register`, `bootstrap`, `destroy`), `content-types/*/lifecycles.{js,ts}`, `src/policies`, `src/middlewares` (middlewares de Document Service ; les middlewares HTTP sont un sous-ensemble), controllers et services créés via `factories.createCoreController` et équivalents, `config/cron-tasks`.
- Objet `strapi` exposé avec un sous-ensemble : `documents()`, `db.query()`, `db.lifecycles.subscribe`, `documents.use`, `config.get`, `log`, `service()`, `plugin()`.
- Bundle à la volée (cache disque, invalidation sur modification) ; `import` de paquets npm présents dans `node_modules` s'ils sont en JS pur.
- Polyfills limités : `console`, `setTimeout`, `URL`, `TextEncoder`, `fetch` (implémenté en Go), `crypto.randomUUID`.
- Un pool de VMs goja (une VM n'est pas thread-safe) ; appels Go vers JS sérialisés par VM.

## Hors périmètre
- Modules natifs (sharp, bcrypt natif), API Node (`fs`, `net`, `http`, `child_process`, `crypto` complet), plugins Strapi tiers complets.

## Critères d'acceptation
- [ ] Les `lifecycles.ts` historiques de dev-freelance (slugify, uuid) tournent sans modification et donnent les mêmes slugs et hashes que Strapi.
- [ ] Un fichier qui importe `ioredis` ou `@anthropic-ai/sdk` échoue au chargement avec un message qui nomme le module et suggère un hook Go.
- [ ] Un hook JS qui boucle à l'infini est interrompu après le délai configuré (`jsrt.timeout`, défaut 5 s), sans bloquer les autres requêtes.
- [ ] Mesure publiée : coût d'un lifecycle JS par rapport au même hook en Go.

## Notes techniques
- Limites à documenter clairement dans le README : goja est environ 10 à 20 fois plus lent que V8 ; ESM converti en CommonJS par esbuild ; `async/await` pris en charge par goja, mais chaque attente bloque la VM courante ; seuls les paquets npm en JS pur sont bundlables.
- Le code JS ne voit jamais de types `internal/` : il passe par une façade stable.

## Référence Strapi
- `core/core/src/loaders/` (apis, src-index, policies, middlewares), `core/core/src/utils/load-config-file.ts`
- `core/core/src/utils/transform-content-types-to-models.ts` (branchement des lifecycles)

## Dépendances
- `EXT-001`, `EXT-002`

## Fichiers
- `internal/jsrt/bundle.go`, `internal/jsrt/vm.go`, `internal/jsrt/pool.go`, `internal/jsrt/strapi_facade.go`, `internal/jsrt/polyfills.go`
