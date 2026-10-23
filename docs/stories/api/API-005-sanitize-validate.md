---
status: done
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
- [x] Un champ `private` n'apparaît jamais dans `/api/*`, ni via `populate`, ni via `fields`.
- [x] Le rôle Public sans permission `find` sur `user` ne peut pas peupler `author` d'un post.
- [x] `filters[author][password][$startsWith]=a` est refusé (cas de fuite connu dans l'écosystème).
- [x] Banc de compatibilité : même ensemble de clés en sortie que Strapi pour chaque rôle (Public, Authenticated, token read-only, token full-access).

## Avancement
- Banc `devfreelance_sanitize` (41 cas, SQLite et Postgres) identique à Strapi pour Public, un utilisateur authentifié, un token lecture seule et un token complet : `fields`, `populate`, `filters` et `sort` sur `password` et `resetPasswordToken` (relation `author` comprise) refusés en `400 Invalid key`, `populate[author][fields][0]=password` refusé, `/api/users?fields[0]=password`. Puis le rôle Authenticated limité à `find` sur les posts : `author` n'est pas peuplé, ses filtres et tris sont refusés, `/api/users` répond 403. Le champ privé `internalNote` de `basic` ne sort ni par `fields` ni par `filters` (`basic`).
- Registre de sanitizers : `App.AddSanitizer("content-api.input" | "content-api.output", fn)`, appliqués après les règles intégrées, à chaque entrée de premier niveau, comme `strapi.sanitizers.add` (`TestSanitizers`). L'ordre des clés est conservé, les clés ajoutées suivent par ordre alphabétique.
- Hors périmètre pour l'instant : le registre des validators (`strapi.validators.add`).

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
