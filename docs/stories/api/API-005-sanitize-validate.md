---
status: todo
---

# API-005 : Sanitize et validation (entrée, sortie, requête, selon les permissions)

## Contexte
Strapi parcourt l'entrée, la sortie et les paramètres de requête avec un traverseur générique piloté par le schéma, et retire (sanitize) ou refuse (validate) ce que l'appelant n'a pas le droit de voir ou d'écrire.

## Objectif
Un traverseur schéma-dépendant et les règles de sanitize et validate de Strapi.

## Périmètre
- Entrée : retirer `id` et `documentId` racine, les attributs non `writable`, les relations vers des types que l'auth ne peut pas lire ; en mode strict, les champs inconnus.
- Sortie : retirer les attributs `private` (schéma ou `api.responses.privateAttributes`), les mots de passe, les relations et champs non autorisés par les permissions (champs de `AUTH-001`), `createdBy`/`updatedBy` sauf `populateCreatorFields`.
- Requête : retirer ou refuser les champs non autorisés, privés et mots de passe dans `filters`, `sort`, `fields`, `populate` ; refuser les cibles morphTo et les zones dynamiques dans les filtres.
- Deux variantes, comme Strapi : `sanitize` (retire silencieusement) et `validate` (lève une `ValidationError`). La core API valide la requête puis assainit.
- Registre de sanitizers et validators personnalisés (enregistrables en Go).

## Hors périmètre
- Les règles de valeurs (`DOC-003`).

## Critères d'acceptation
- [ ] Un champ `private` n'apparaît jamais dans `/api/*`, ni via `populate`, ni via `fields`.
- [ ] Le rôle Public sans permission `find` sur `user` ne peut pas peupler `author` d'un post.
- [ ] `filters[author][password][$startsWith]=a` est refusé (cas de fuite connu dans l'écosystème).
- [ ] Banc de compatibilité : même ensemble de clés en sortie que Strapi pour chaque rôle (Public, Authenticated, token read-only, token full-access).

## Notes techniques
- Un seul traverseur générique (visitor) réutilisé par les trois usages ; il ne doit jamais faire d'accès base.
- Sécurité : c'est le ticket le plus sensible. Tests de non-régression dédiés et revue de sécurité avant fusion.

## Référence Strapi
- `core/utils/src/sanitize/`, `core/utils/src/validate/`, `core/utils/src/traverse/`
- `core/core/src/core-api/controller/index.ts` (sanitizeQuery, sanitizeInput, sanitizeOutput, validateQuery)

## Dépendances
- `SCH-001`, `AUTH-001`

## Fichiers
- `internal/httpapi/sanitize/traverse.go`, `internal/httpapi/sanitize/input.go`, `internal/httpapi/sanitize/output.go`, `internal/httpapi/sanitize/query.go`, `internal/httpapi/sanitize/registry.go`
