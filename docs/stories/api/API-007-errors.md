---
status: done
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
- [x] Les corps d'erreur des 10 classes sont identiques à Strapi (banc de compatibilité).
- [x] Une panique dans un handler donne un 500 propre, sans pile dans la réponse.

## Avancement
- Banc : 8 des 10 classes sortent déjà à l'identique de Strapi dans les goldens (`ValidationError`, `ForbiddenError`, `UnauthorizedError`, `NotFoundError`, `ApplicationError`, `PayloadTooLargeError`, `RateLimitError`, `PaginationError`). Strapi ne lève jamais `PolicyError` ni `NotImplementedError` sur ses propres routes : seul le code d'un projet le fait. Le banc `devfreelance_errors` les déclare des deux côtés (`rules.js` : policies nommées dans le registre de Strapi et routes `strapi.server.routes` ; `framework/main.go` : `app.Policy` et `PublicRoute`) : policy qui refuse (403 « Policy Failed »), policy qui lève une `PolicyError` avec détails, handler qui lève `NotImplementedError` (501, `details: {}`), identiques à Strapi.
- Erreurs publiques pour le code Go (`errors.go`) : un constructeur par classe, messages par défaut de `@strapi/utils`, détails optionnels, trouvées par `errors.As` même enveloppées. `TestStrapiErrorClasses` : 501 d'un handler, 403 `PolicyError` d'une policy qui refuse ou qui renvoie sa propre erreur, 400 d'un hook de ligne. Une erreur applicative garde `details` même en 5xx (501 `{}`), comme le `formatApplicationError` de Strapi ; une erreur interne n'en a pas.

## Notes techniques
- Type `*grapi.Error{Status, Name, Message, Details}` qui implémente `error` ; constructeurs par classe.

## Référence Strapi
- `core/utils/src/errors.ts`, `core/core/src/services/errors.ts`, `core/core/src/middlewares/errors.ts`

## Dépendances
- `API-001`

## Fichiers
- `errors.go` (public), `internal/httpapi/errors.go`

## Avancement
- Panique dans un handler (API de contenu ou admin) : 500 au format de Strapi, connexion conservée, valeur et pile seulement dans les journaux (`TestPanicGivesA500`). Avant, `net/http` coupait la connexion sans réponse.
- Banc : 8 des 10 classes y sont comparées (`ValidationError`, `NotFoundError`, `ForbiddenError`, `ApplicationError`, `UnauthorizedError`, `PaginationError`, `PayloadTooLargeError`, `RateLimitError`, plus le 500 générique). Restent `PolicyError` (il faudrait une fixture avec une policy JavaScript ; format testé dans `policies_test.go`) et `NotImplementedError` (aucune route standard ne la lève).
