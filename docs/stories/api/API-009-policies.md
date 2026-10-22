---
status: todo
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
- [ ] Une policy relative `is-owner` déclarée dans l'API post se résout en `api::post.is-owner`.
- [ ] Une policy inconnue fait échouer le démarrage avec le nom et la route en cause.
- [ ] Une policy qui renvoie `false` produit le même 403 que Strapi.

## Notes techniques
- Les policies sont résolues au démarrage, pas à chaque requête.

## Référence Strapi
- `core/core/src/registries/policies.ts`, `core/core/src/services/server/policy.ts`, `core/utils/src/policy.ts`
- `core/admin/server/src/policies/`

## Dépendances
- `API-002`, `SCH-005`

## Fichiers
- `internal/httpapi/policy/registry.go`, `internal/httpapi/policy/builtin.go`, `hook/policy.go`
