---
status: in-progress
---

# DOC-004 : Middlewares du Document Service

## Contexte
`strapi.documents.use(fn)` enregistre un middleware global autour de toutes les opérations de documents, quel que soit le point d'entrée (admin, REST, GraphQL, code). C'est le point d'extension recommandé par Strapi 5. dev-freelance s'en sert pour la génération IA (toggles `generateFaq`, `generateTldr`, `generateMetadescription`), la régénération de slug et l'invalidation du cache Redis.

## Objectif
Offrir la même chaîne de middlewares, en Go (`EXT-002`) et plus tard en JS (`EXT-003`).

## Périmètre
- Signature : `func(ctx *document.Context, next document.Next) (any, error)`, avec `ctx.UID`, `ctx.ContentType`, `ctx.Action` (`findOne`, `findFirst`, `findMany`, `count`, `create`, `update`, `delete`, `clone`, `publish`, `unpublish`, `discardDraft`), `ctx.Params` modifiable.
- Ordre d'enregistrement = ordre d'exécution (style Koa : avant et après `next`).
- `Use` renvoie une fonction de désinscription.
- Middleware interne qui traduit les erreurs de base (contrainte unique, FK) en erreurs applicatives.

## Hors périmètre
- Les lifecycles de ligne (`DB-009`), qui sont une couche différente.

## Critères d'acceptation
- [x] Un middleware qui modifie `ctx.Params.Data` avant `next` voit sa modification persistée, quel que soit le point d'entrée (REST, admin, appel Go).
- [x] Un middleware peut court-circuiter (`return` sans `next`).
- [x] Désinscription effective.
- [ ] Reproduction des middlewares dev-freelance (slug si vide, toggles IA remis à `false`, invalidation par content type) : comportement identique au golden master de la session de caractérisation.

## Avancement
- Fait : `internal/document/middleware.go` (`Use`, ordre Koa, `Params` et `documentId` modifiables, désinscription), exposé par `grapi.App.UseDocuments` et le paquet public `document`. Tests : `internal/document/middleware_test.go`, `hooks_test.go` (par HTTP).
- Reste : la traduction des erreurs de base en erreurs applicatives, la reproduction des middlewares dev-freelance.

- Middlewares de dev-freelance portés en Go (`compat/fixtures/devfreelance/framework/middlewares.go`) : slug depuis le titre quand il est vide (post, média), toggles IA (FAQ, TL;DR, meta description) remplis puis remis à `false`, repli sur le contenu enregistré quand l'update n'en porte pas, toggles remis à `false` sans génération quand le contenu est vide. Même faux LLM des deux côtés (`rules.js` pour Strapi). Banc `devfreelance_rules` identique à Strapi.
- Reste : l'invalidation de cache par content type (Redis), absente du banc.

## Notes techniques
- Pas de réflexion : la chaîne est une slice de fonctions composée une fois par appel.

## Référence Strapi
- `core/core/src/services/document-service/middlewares/middleware-manager.ts`

## Dépendances
- `DOC-001`

## Fichiers
- `document/middleware.go`, `hook/document.go`
