---
status: in-progress
---

# API-009 : Policies

## Contexte
Une policy est une fonction exécutée avant le handler, qui autorise ou refuse la requête. Elle est déclarée par nom sur une route.

## Objectif
Le registre de policies, la résolution des noms et les policies intégrées.

## Périmètre
- Signature : `func(pctx *PolicyContext, config any) (bool, error)`. Renvoyer autre chose que `true` (ou rien) lève une `PolicyError` 403.
- Résolution d'un nom déclaré sur une route : d'abord l'uid complet (`global::is-owner`, `api::post.is-owner`, `plugin::users-permissions.x`), puis relatif au plugin de la route, puis relatif à l'API de la route.
- Policy avec config : `{name, config}` sur la route, et `validator` optionnel sur la policy (config invalide = erreur au démarrage).
- Policies intégrées : `admin::isAuthenticatedAdmin`, `admin::hasPermissions` (avec liste d'actions), `admin::isTelemetryEnabled` remplacée par une policy qui refuse toujours (pas de télémétrie).
- Policies globales (`src/policies`) et par API ou plugin.

## Hors périmètre
- Le moteur de permissions (`AUTH-001`).

## Critères d'acceptation
- [x] Une policy relative `is-owner` déclarée dans l'API post se résout en `api::post.is-owner`.
- [x] Une policy inconnue fait échouer le démarrage avec le nom et la route en cause.
- [x] Une policy qui renvoie `false` produit le même 403 que Strapi.

## Avancement
- Mode framework (`policies.go`) : `app.Policy(uid, fn)` (`global::`, `api::<api>.`, `plugin::<plugin>.`), policies sur les routes custom (`app.Route(...).Policies("is-owner", grapi.PolicyRef{Name, Config})`) et sur les routes générées par action (`app.RoutePolicies("api::post.post.find", ...)`). Un nom relatif se résout sur l'API ou le plugin du scope de la route, puis en `global::`. Une policy inconnue arrête le démarrage avec la route et les noms cherchés.
- `internal/httpapi` : les policies passent après l'authentification et le contrôle de permission, routes publiques comprises, comme le pipeline de Strapi. `false` donne la `PolicyError` 403 de Strapi (`Policy Failed`, `details: {}`) ; une erreur renvoyée (`grapi.Error`) est répondue telle quelle. Tests `TestPolicies` et `TestPolicyResolution`.
- Côté GraphQL, les policies des `resolversConfig` existent depuis `PLG-004`.
- Reste : policies déclarées dans les fichiers de routes JS (`EXT-003`), `validator` de config au démarrage, policies admin (avec l'admin).

## Notes techniques
- Les policies sont résolues au démarrage, pas à chaque requête.

## Référence Strapi
- `core/core/src/registries/policies.ts`, `core/core/src/services/server/policy.ts`, `core/utils/src/policy.ts`
- `core/admin/server/src/policies/`

## Dépendances
- `API-002`, `SCH-005`

## Fichiers
- `internal/httpapi/policy/registry.go`, `internal/httpapi/policy/builtin.go`, `hook/policy.go`
