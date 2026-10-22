---
status: in-progress
---

# API-002 : Routage déclaratif et pipeline par route

## Contexte
Chez Strapi, une route est une déclaration (`method`, `path`, `handler`, `config`) et chaque requête traverse une chaîne fixe : informations de route, authentification, autorisation, policies, middlewares de route, handler.

## Objectif
Le même modèle de routes et le même pipeline, alimentés par les routes générées (`API-006`), les plugins et le code utilisateur (`EXT-002`).

## Périmètre
- Route : `{method: GET|POST|PUT|PATCH|DELETE|ALL, path, handler, config: {auth: false | {scope[], strategies[]}, policies[], middlewares[], prefix}, info}`.
- Router : `{type: content-api|admin, prefix, routes[]}`. Un `config.prefix` sur la route remplace le préfixe du router.
- Préfixes : `api.rest.prefix` (défaut `/api`) pour content-api ; `/admin` et `/<plugin>` pour admin.
- Paramètres de chemin au format Strapi (`/articles/:id`) convertis vers le routeur Go.
- Handler : référence `"<controller>.<action>"` résolue dans le registre (api, plugin, admin) ou fonction Go.
- Scope d'auth généré automatiquement : `api::<api>.<controller>.<action>`, `plugin::<plugin>.<controller>.<action>`, `admin::...`.
- Pipeline : authenticate (stratégies enregistrées par type d'API : API token, users-permissions, admin) puis verify (401 si non authentifié, 403 si pas la permission) puis policies (`API-009`) puis middlewares de route puis handler.
- `auth: false` désactive authentification et autorisation pour la route.
- Validation de la config de route au chargement (erreur claire au démarrage).

## Hors périmètre
- Les stratégies elles-mêmes (`AUTH-002`, `AUTH-008`, `AUTH-007`).
- Le schéma de requête/réponse par route pour OpenAPI (`PLG-005`).

## Critères d'acceptation
- [ ] Une route custom `GET /api/posts/popular` avec `handler: post.popular` est servie et apparaît dans `routes:list`.
- [ ] Sans token, une route protégée renvoie 403 si le rôle Public n'a pas la permission, et une route marquée `auth: false` répond.
- [ ] Ordre d'exécution vérifié par test : auth, policies dans l'ordre déclaré, middlewares dans l'ordre déclaré, handler.
- [ ] Une `PolicyError` garde son message et son status 403 dans la réponse.
- [ ] Banc de compatibilité : codes de statut identiques à Strapi sur la matrice rôle x route de dev-freelance.

## Avancement
- Fait : routes custom en Go (`grapi.App.Route`, `PublicRoute` pour `auth: false`), placées avant les routes core, avec le scope de permission à la Strapi (`api::account.account.anonymize`) et les erreurs `ctx.badRequest`/`ctx.unauthorized` (`grapi.Error`, noms http-errors). Test : `hooks_test.go` (`TestCustomRoutes`).
- Reste : routes déclarées dans `src/api/*/routes/*` (fichiers JS ou déclaratifs), policies et middlewares de route, `routes:list`.

## Notes techniques
- Conflits de routes (même méthode et chemin) : erreur au démarrage, Strapi laisse gagner la première.
- Garder la liste des routes compilée une fois au démarrage, rechargée à chaud par le CTB (`ADM-006`).

## Référence Strapi
- `core/core/src/services/server/routing.ts`, `compose-endpoint.ts`, `register-routes.ts`, `api.ts`, `content-api.ts`, `admin-api.ts`
- `core/core/src/services/auth/index.ts`

## Dépendances
- `API-001`, `SCH-005`

## Fichiers
- `internal/httpapi/router.go`, `internal/httpapi/route.go`, `internal/httpapi/pipeline.go`, `internal/httpapi/authstrategy.go`
