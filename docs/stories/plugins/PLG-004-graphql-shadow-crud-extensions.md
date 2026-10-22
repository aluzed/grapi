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
