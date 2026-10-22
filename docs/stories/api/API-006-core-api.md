# API-006 : Core API générée (routes, controllers, services CRUD, format {data, meta}, surcharges)

## Contexte
Pour chaque content type, Strapi génère un router, un controller et un service CRUD (`createCoreRouter`, `createCoreController`, `createCoreService`), que le projet peut restreindre ou surcharger action par action.

## Objectif
La même API REST générée, avec les mêmes chemins, codes et formats de réponse.

## Périmètre
- Routes collection type : `GET /<plural>` (find), `GET /<plural>/:id` (findOne, `:id` = documentId), `POST /<plural>` (create, 201), `PUT /<plural>/:id` (update), `DELETE /<plural>/:id` (delete, 204).
- Routes single type : `GET|PUT|DELETE /<singular>` (`DOC-007`).
- Options du router : `prefix`, `only`, `except`, `config` par action (`policies`, `middlewares`, `auth`), `type`.
- Controller de base : validation et assainissement de la requête (`API-005`), appel du service, assainissement de la sortie, réponse `{data, meta}` (meta = `{pagination}` pour find). `create` et `update` exigent `body.data` (sinon `ValidationError` "Missing \"data\" payload in the request body").
- Service de base : appelle le Document Service avec `status: published` par défaut côté Content API, gère la pagination (`API-004`).
- Surcharges : remplacer une action, ajouter une action, appeler l'action de base depuis la surcharge (équivalent de `super`). En Go via `EXT-002`.
- Upload en multipart : `data` en JSON dans un champ de formulaire et fichiers dans `files.<champ>`, comme Strapi.

## Hors périmètre
- GraphQL (`PLG-003`).

## Critères d'acceptation
- [ ] Les 5 routes d'un collection type répondent avec les mêmes statuts et corps que Strapi pour post, category, theme et medium de dev-freelance.
- [ ] `except: ['delete']` retire la route.
- [ ] Une surcharge `find` qui appelle la base puis ajoute `meta.extra` fonctionne.
- [ ] Réponse de `DELETE` : 204 sans corps, comme Strapi 5.59 (vérifier sur le banc).

## Notes techniques
- La surcharge Go prend la forme `func(ctx *httpapi.Ctx, base httpapi.Action) error` : pas d'héritage, juste la base passée en paramètre.

## Référence Strapi
- `core/core/src/factories.ts`
- `core/core/src/core-api/routes/index.ts`, `core/core/src/core-api/controller/{index,collection-type,single-type,transform}.ts`, `core/core/src/core-api/service/{index,collection-type,single-type}.ts`

## Dépendances
- `API-002`, `API-004`, `API-005`, `DOC-001`, `DOC-007`

## Fichiers
- `internal/httpapi/coreapi/router.go`, `internal/httpapi/coreapi/controller.go`, `internal/httpapi/coreapi/service.go`, `internal/httpapi/coreapi/transform.go`
