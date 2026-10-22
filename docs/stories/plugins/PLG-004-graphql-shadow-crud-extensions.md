---
status: in-progress
---

# PLG-004 : GraphQL : shadow CRUD, extensions, auth, policies, limites

## Contexte
Strapi permet de masquer des parties du schéma généré (*shadow CRUD*), d'étendre le schéma avec du SDL et des résolveurs, et applique la même autorisation que REST sur chaque résolveur.

## Objectif
Les mêmes mécanismes de personnalisation et de sécurité, exposés en Go (mode framework, `EXT-002`) et en configuration.

## Périmètre
- Shadow CRUD, API Go reprenant les mêmes verbes que Strapi :
  - `ShadowCRUD(uid).Disable()`, `.DisableQueries()`, `.DisableMutations()`, `.DisableAction("create")`, `.DisableActions(...)` ;
  - `.Field("password").Disable()`, `.DisableOutput()`, `.DisableInput()`, `.DisableFilters()` ;
  - config globale `shadowCRUD: false` pour tout désactiver.
- Extensions : `Extension{TypeDefs string, Resolvers map, ResolversConfig map, Types}`. Le SDL est parsé puis fusionné dans le schéma dynamique (nouveaux types, `extend type Query`, champs sur des types générés).
- `ResolversConfig["Query.posts"] = {Auth: false | {Scope}, Policies, Middlewares}`.
- Autorisation : chaque champ Query et Mutation vérifie `auth.verify` avec le scope REST équivalent (`api::post.post.find`, etc.). `auth: false` lève la vérification. Les policies puis les middlewares s'exécutent avant le résolveur.
- Sanitize des arguments et de la sortie avec les mêmes règles que REST (`API-005`) : champs privés, password, permissions sur les relations.
- Limites :
  - `depthLimit` : validation de profondeur ;
  - `defaultLimit` et `maxLimit` sur la pagination ;
  - avertissement au boot si elles sont non bornées, avec le même message que Strapi ;
  - grapi ajoute une limite de complexité optionnelle.
- Extensions de users-permissions : mutations `login`, `register`, `forgotPassword`, `resetPassword`, `changePassword`, `emailConfirmation`, et la query `me`.
- Extension du plugin upload : `UploadFile` dans le schéma, mutations upload si activées.

## Hors périmètre
- Extensions écrites en JS (viendront avec `EXT-003`).

## Critères d'acceptation
- [ ] `ShadowCRUD("api::post.post").Field("internalNote").Disable()` retire le champ du SDL (TU sur le SDL).
- [ ] Une extension SDL qui ajoute `Query.popularPosts` avec un résolveur Go est appelable et soumise à son scope (intégration).
- [ ] Une requête plus profonde que `depthLimit` est rejetée avec une erreur GraphQL standard.
- [ ] Banc `SETUP-002` : `login` et `me` via GraphQL donnent les mêmes réponses que Strapi. Un rôle Public sans `find` reçoit une erreur `Forbidden access` identique.

## Avancement
- Shadow CRUD : `graphql.Options.ShadowCRUD` dans le builder (types, queries, mutations, actions, champs en sortie, entrée et filtres), exposé en mode framework par `app.GraphQL().ShadowCRUD(uid)` avec les verbes de Strapi (`Disable`, `DisableQueries`, `DisableMutations`, `DisableAction(s)`, `Field(name).Disable/DisableOutput/DisableInput/DisableFilters`, `graphql.go`). Les désactivations des plugins (admin, dossiers d'upload, mutations de fichiers et de locales, permissions, CRUD users et rôles remplacé) sont celles de Strapi : le SDL est identique à celui de Strapi sur les 7 fixtures (`PLG-003`).
- Extensions : `app.GraphQL().Use(GraphQLExtension{TypeDefs, Resolvers, ResolversConfig})`. Le SDL est chargé avec le schéma généré par gqlparser (`extend type Query` compris), les résolveurs Go sont branchés sur les champs, `ResolversConfig` règle `Auth: false`, le scope et les policies, y compris sur les champs générés.
- Autorisation : wrap-resolvers reproduit (champs de Query et Mutation toujours vérifiés, relations sur le scope `find` de la cible, scopes propres à upload et i18n), `PolicyError` en `FORBIDDEN`.
- Limites : `depthLimit` (message de graphql-depth-limit, 400 `GRAPHQL_VALIDATION_FAILED`), `defaultLimit`, `maxLimit`, avertissement au démarrage avec le message de Strapi quand elles ne sont pas bornées.
- Extensions users-permissions (`me`, `login`, `register`, `forgotPassword`, `resetPassword`, `changePassword`, CRUD users) et upload (`updateUploadFile`, `deleteUploadFile`) branchées sur les contrôleurs REST, rate limit compris (le `createRateLimitRunner` du plugin partage le compteur des routes REST).
- Test d'intégration `TestGraphQLExtensions` (extension SDL, scope, `auth: false`, policy, `depthLimit`, shadow CRUD vu par introspection).
- Banc `graphql` étendu (40 cas, SQLite et Postgres) : `register` (dont compte déjà pris), `login` (dont mauvais mot de passe), `me` avec JWT, `changePassword` invalide, `forgotPassword`, tous identiques à Strapi.
- Reste : middlewares de résolveur, `shadowCRUD: false` global, `emailConfirmation` et CRUD des rôles, limite de complexité.

## Notes techniques
- Pour la fusion du SDL : parser avec `gqlparser`, puis convertir les définitions en objets `graphql-go`. C'est à écrire, il n'y a pas de pont existant mûr.
- `graphql-go` accepte des règles de validation custom : y brancher la profondeur et la complexité.

## Référence Strapi
- `plugins/graphql/server/src/services/extension/{extension.ts,shadow-crud-manager.ts}`
- `plugins/graphql/server/src/services/content-api/wrap-resolvers.ts`
- `plugins/graphql/server/src/bootstrap.ts` (limites)
- `plugins/users-permissions/server/src/graphql/`

## Dépendances
- `PLG-003`, `API-005`, `API-009`, `AUTH-004`, `EXT-002`

## Fichiers
- `internal/plugins/graphql/{shadowcrud.go,extension.go,sdlmerge.go,auth.go,limits.go}`
