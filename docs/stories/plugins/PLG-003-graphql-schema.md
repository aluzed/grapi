---
status: in-progress
---

# PLG-003 : GraphQL : schéma généré dynamiquement

## Contexte
Le plugin GraphQL de Strapi génère un schéma complet à partir des content types (Apollo + Nexus) : types, filtres, pagination, relations, composants et zones dynamiques. Ce schéma fait partie de l'API de contenu publique, donc il doit être compatible fil à fil (D1).

## Objectif
Générer au démarrage, puis à chaque rechargement de schéma (`ADM-006`), un schéma GraphQL identique à celui de Strapi 5.59 pour le même projet.

## Périmètre
- Endpoint `POST /graphql` (configurable avec `endpoint`), et le playground en GET hors production (`playgroundAlways` pour le forcer).
- Scalaires : `DateTime`, `Date`, `Time`, `JSON`, `Long`, `Upload`.
- Naming, reproduit à l'identique, en partant de `getTypeName` (singularName en PascalCase, préfixé par le plugin pour `plugin::`) :
  - types : `Post`, `ComponentSeoMeta`, `PostFiltersInput`, `PostInput`, `StringFilterInput`, `ENUM_POST_STATUS` ;
  - queries : `post(documentId, status, locale)`, `posts(filters, pagination, sort, status, locale)` qui renvoie `[Post]!`, et `posts_connection(...)` qui renvoie `PostEntityResponseCollection {nodes, pageInfo {page, pageSize, pageCount, total}}` ;
  - mutations : `createPost(data, status, locale)`, `updatePost(documentId, data, status, locale)`, et `deletePost(documentId, locale)` qui renvoie `{documentId}` ;
  - single types : `homepage`, `updateHomepage` et `deleteHomepage`.
- Filtres par type : `and`, `or`, `not`, plus pour chaque champ un `<Scalar>FilterInput` avec eq, eqi, ne, nei, lt, lte, gt, gte, in, notIn, contains, notContains, containsi, notContainsi, startsWith, endsWith, null, notNull et between. Les relations sont filtrables par imbrication.
- Relations :
  - to-one : type direct ;
  - to-many : liste, avec arguments `filters`, `pagination` et `sort`, plus un champ `<rel>_connection`.
- Composants : types objet. Zones dynamiques : `union` des composants + `Error`, et scalaire d'entrée dédié pour les mutations (avec `__typename` ou `__component`).
- Relations polymorphes : `GenericMorph` en union.
- Arguments i18n (`locale`) et D&P (`status`, `publicationFilter`). `hasPublishedVersion` est déprécié mais exposé.
- Mode `v4CompatibilityMode` : optionnel (wrappers `data`, `attributes`), à faire en dernier.
- Résolveurs branchés sur le Document Service (`DOC-001`), sur les mêmes paramètres que REST (`API-004`).

## Hors périmètre
- Shadow CRUD, extensions, auth et limites (`PLG-004`).
- Subscriptions (désactivées par défaut chez Strapi).

## Critères d'acceptation
- [ ] **Le SDL généré** pour le projet de test dev-freelance est identique à celui de Strapi 5.59, ordre des types normalisé : diff dans le banc `SETUP-002`.
- [ ] Une série de requêtes (liste paginée, filtres imbriqués, populate de zone dynamique avec fragments, mutation create avec composant) donne des réponses JSON identiques (banc).
- [ ] Le schéma est reconstruit après un changement de content type, sans redémarrage.
- [ ] Introspection fonctionnelle (graphql-codegen tourne sur le schéma de grapi).

## Avancement
- `internal/graphql` : schéma (`schema.go`) dérivé du `schema.Set` comme le shadow CRUD du plugin (types, `FiltersInput`, `Input`, enums, zones dynamiques, `GenericMorph`, réponses, queries et mutations, extensions upload, users-permissions et i18n, élagage des types inaccessibles). `TestSchemaMatchesStrapi` compare le schéma à celui que Strapi 5.59 publie par introspection sur les 7 fixtures du banc (`internal/graphql/testdata/sdl-*.graphql`) : identique type par type, champ par champ, arguments et valeurs par défaut compris, hors types Enterprise de review-workflows (clean-room).
- Exécuteur maison (`exec.go`) sur `vektah/gqlparser` (MIT) pour le parsing et la validation, dont les messages sont ceux de graphql-js : fragments, alias, `@skip`/`@include`, propagation des null, sérialisation des scalaires, introspection (`introspection.go`) autorisée hors production comme Apollo. Choix contre `graphql-go/graphql`, dont les messages d'erreur ne sont pas ceux de graphql-js.
- Résolveurs dans `internal/httpapi/graphql.go`, branchés sur le Document Service et la validation REST : `transformArgs`, pagination `withDefaultPagination` (10 par défaut), filtres convertis en opérateurs Strapi, héritage du statut de la query racine par les relations, mutations avec `sanitize.input`, single types (`update` qui crée, `delete` en `NotFoundError`), format d'erreurs du plugin (`FORBIDDEN`, `BAD_USER_INPUT`, `STRAPI_*`), CSRF d'Apollo, codes 400 de parsing, validation et variables.
- Activation : dépendance `@strapi/plugin-graphql` dans le `package.json` du projet, ou `plugins.graphql.enabled` (`docs/configuration.md`).
- Banc `graphql` (fixture et corpus dédiés, 33 cas), SQLite et Postgres : 33/33 en mode grapi, contrôle inverse OK.
- Corpus GraphQL sur les fixtures existantes, en mode grapi : `components_graphql` 12/12 (composants imbriqués, répétables avec `filters`/`pagination`/`sort`, zones dynamiques avec fragments, filtres sur composants, mutations avec `__typename`, erreurs de validation), `i18n-dp_graphql` 19/19 (`locale` en lecture et en écriture, `localizations`, single type localisé, statut hérité par les relations, `publicationFilter`, code de locale inconnu refusé à la validation avec la liste ISO du plugin, `internal/isolocales`), `devfreelance_graphql` 13/13 (médias simples et multiples, `uploadFiles`, `updateUploadFile`, `deleteUploadFile`, utilisateurs, `me` anonyme).
- Reste : CRUD des rôles users-permissions (réponse `NotImplementedError`, il attend les routes de rôles du content API), rate limit des mutations d'auth, héritage de `publicationFilter` par les relations, relations polymorphes au banc, graphql-codegen sur le schéma.

## Notes techniques
- Le schéma est dynamique, ce qui exclut gqlgen. Utiliser `graphql-go/graphql` (`graphql.NewObject`, `NewUnion`, `NewInputObject` à la volée). `vektah/gqlparser` sert à parser le SDL des extensions (`PLG-004`).
- Résoudre les relations par lots (dataloader par requête) pour éviter le N+1. Strapi ne le fait pas, mais c'est sans effet visible sur la sortie.
- Mettre le schéma derrière un `atomic.Pointer` pour le remplacer à chaud.

## Référence Strapi
- `plugins/graphql/server/src/services/utils/naming.ts`
- `plugins/graphql/server/src/services/builders/` (types, filters, inputs, queries, mutations, dynamic-zones, generic-morph, enums)
- `plugins/graphql/server/src/services/builders/filters/operators/`
- `plugins/graphql/server/src/config/default-config.ts`
- `plugins/i18n/server/src/graphql.ts`

## Dépendances
- `DOC-001`, `API-004`, `SCH-005`, `PLG-001`

## Fichiers
- `internal/plugins/graphql/{plugin.go,naming.go,scalars.go,builder_types.go,builder_filters.go,builder_inputs.go,builder_queries.go,builder_mutations.go,resolvers.go,handler.go}`
