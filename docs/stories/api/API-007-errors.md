---
status: in-progress
---

# API-007 : Classes d'erreur et format d'erreur

## Contexte
Les fronts et SDK Strapi s'appuient sur la forme exacte des erreurs : `{data: null, error: {status, name, message, details}}`.

## Objectif
Les mêmes classes d'erreur et la même sérialisation.

## Périmètre
- Classes : `ApplicationError` (400), `ValidationError` (400, `details.errors[]` avec `path`, `message`, `name`), `PaginationError` (400), `NotFoundError` (404), `ForbiddenError` (403), `PolicyError` (403), `UnauthorizedError` (401), `PayloadTooLargeError` (413), `RateLimitError` (429), `NotImplementedError` (501).
- Erreurs inconnues : 500 avec le message générique "Internal Server Error", détail journalisé côté serveur uniquement.
- 404 si aucune route ne répond, au même format.
- Utilisables depuis les hooks Go (`errors.As`) et les hooks JS.

## Hors périmètre
- Le middleware `responses` qui personnalise les réponses par code (`API-008`).

## Critères d'acceptation
- [ ] Les corps d'erreur des 10 classes sont identiques à Strapi (banc de compatibilité).
- [ ] Une panique dans un handler donne un 500 propre, sans pile dans la réponse.

## Notes techniques
- Type `*grapi.Error{Status, Name, Message, Details}` qui implémente `error` ; constructeurs par classe.

## Référence Strapi
- `core/utils/src/errors.ts`, `core/core/src/services/errors.ts`, `core/core/src/middlewares/errors.ts`

## Dépendances
- `API-001`

## Fichiers
- `errors.go` (public), `internal/httpapi/errors.go`
