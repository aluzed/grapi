# API-004 : Paramètres REST : filters, sort, fields, populate, pagination, locale, status, strictParams

## Contexte
Le contrat de l'API de contenu repose sur ses paramètres de requête. C'est la partie la plus utilisée par les fronts existants (dont le front Go de dev-freelance) : la compatibilité doit être exacte.

## Objectif
Traduire l'arbre qs (`API-003`) en `document.Params` validés, avec les mêmes règles et erreurs que Strapi.

## Périmètre
- Clés autorisées : `filters`, `sort`, `fields`, `populate`, `status`, `locale`, `pagination`, `page`, `pageSize`, `start`, `limit`, `_q`, `publicationFilter`. Avec `api.rest.strictParams` (ou `api.documents.strictParams`), une clé inconnue est une `ValidationError`.
- `filters` : arbre d'opérateurs (`DB-005`), à travers relations et composants, validé contre le schéma (champs privés, mots de passe et champs inconnus refusés, `API-005`).
- `sort` : `"a:desc"`, `"a"` (asc), liste, objet imbriqué `{rel: {field: 'asc'}}`.
- `fields` : champs scalaires uniquement ; `id` et `documentId` toujours inclus.
- `populate` : `*` (profondeur 0 seulement), liste, objet `{fields, filters, sort, populate, count}`, fragments `on: {'comp.uid': {...}}` réservés aux zones dynamiques et morphTo (un fragment n'accepte que `on`, `count` et `populate: '*'`), `count: true` pour le nombre de relations.
- Pagination :
  - `page`/`pageSize` ou `start`/`limit` ; mélanger les deux est une `PaginationError`.
  - `api.rest.defaultLimit` (25) et `api.rest.maxLimit` (aucun par défaut) ; `limit=-1` ramène à `maxLimit`.
  - `withCount` : `true|t|1` ou `false|f|0`, toute autre valeur est une `ValidationError` ; défaut `api.rest.withCount` (true).
  - Meta : `{page, pageSize, pageCount, total}` ou `{start, limit, total}` ; `total` et `pageCount` omis si le comptage est désactivé.
- `locale` (code ou `*`), `status` (`draft` ou `published` ; le Content API lit `published` par défaut).

## Hors périmètre
- L'exécution SQL (`DB-005`, `DB-006`).

## Critères d'acceptation
- [ ] Rejouer les requêtes réelles du front dev-freelance (journal d'accès Traefik d'une journée) : réponses identiques octet par octet à Strapi 5.59 sur la copie de la base.
- [ ] `?pagination[page]=1&pagination[limit]=5` renvoie la même erreur que Strapi.
- [ ] `populate=*` en profondeur 1 refusé comme chez Strapi.
- [ ] `fields[0]=password` ou `filters[password][$eq]=x` refusés.
- [ ] Test des 25 opérateurs sur chaque type d'attribut compatible.

## Notes techniques
- Parse en deux temps : arbre qs vers AST typé, puis validation contre le schéma. Les erreurs gardent le chemin (`filters.author.name.$foo`).
- Le Content API force `status: published` par défaut, alors que le Document Service lit les brouillons par défaut : ne pas confondre les deux couches.

## Référence Strapi
- `core/utils/src/convert-query-params.ts`, `core/utils/src/content-api-constants.ts`, `core/utils/src/pagination.ts`
- `core/core/src/core-api/service/pagination.ts`, `core/core/src/core-api/service/collection-type.ts`

## Dépendances
- `API-003`, `API-005`, `DB-005`, `DOC-001`

## Fichiers
- `internal/httpapi/params/parse.go`, `internal/httpapi/params/populate.go`, `internal/httpapi/params/pagination.go`, `internal/httpapi/params/sort.go`
