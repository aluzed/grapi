---
status: in-progress
---

# AUTH-001 : Moteur de permissions (actions, sujets, champs, locales, conditions traduites en filtres)

## Contexte
Strapi s'appuie sur un moteur de type CASL (`core/permissions`) partagé par le RBAC admin, les API tokens custom et les rôles users-permissions. Une permission vaut `{action, subject, properties:{fields[], locales[]}, conditions[]}`. Une condition (par exemple `is-creator`) se traduit en filtre de requête, ce qui limite à la fois *ce qu'on peut faire* et *quelles lignes on voit*.

## Objectif
Fournir un moteur de permissions générique, sans dépendance HTTP, réutilisé par `AUTH-004`, `AUTH-007`, `AUTH-008` et le sanitize (`API-005`).

## Périmètre
- Registre d'actions : `uid` (`plugin::content-manager.explorer.read`, `admin::webhooks.create`, `api::post.post.find`...), `section` (contentTypes, plugins, settings), `subjects` acceptés, `options.applyToProperties` (`fields`, `locales`).
- Registre de conditions : `{id, displayName, category, handler(user) -> filtre | bool}`. Conditions intégrées : `admin::is-creator` (`{createdBy.id: user.id}`) et `admin::has-same-role-as-creator` (`createdBy.roles` contient un rôle de l'utilisateur).
- Construction d'une *ability* à partir d'une liste de permissions et d'un utilisateur : `Can(action, subject, field?)` et `Filter(action, subject)`, qui renvoie le filtre à injecter dans `DB-005` (OU entre permissions, et OU aussi entre les conditions d'une même permission, comme le moteur de Strapi : une condition qui renvoie `true` lève la restriction, des conditions toutes à `false` suppriment la permission).
- Permissions au niveau champ (`properties.fields`) : liste des champs autorisés, chemins imbriqués dans les composants (`seo.title`).
- Permissions au niveau locale (`properties.locales`), ajoutées par i18n (`PLG-001`).
- Rôle super admin : toutes les actions, sans condition.

## Hors périmètre
- UI d'édition des rôles (`ADM-008`).
- Stockage des permissions admin (`AUTH-007`) et users-permissions (`AUTH-004`).

## Critères d'acceptation
- [x] `Can("plugin::content-manager.explorer.update", "api::post.post", "title")` est faux si `title` est absent de `fields`.
- [x] Une permission avec `is-creator` renvoie un filtre `{createdBy: {id: {$eq: <id>}}}` combinable avec les filtres utilisateur (TU).
- [x] Deux permissions sur la même action combinent leurs conditions en `$or` (TU).
- [ ] Une permission avec `locales: ["fr"]` empêche la lecture des documents `en` (test d'intégration avec `PLG-001`).
- [x] Le moteur ne fait aucun accès DB (il reçoit les permissions déjà chargées) : il est testé en pur unitaire.

## Avancement
- `internal/auth/permissions` : registres d'actions et de conditions (`is-creator`, `has-same-role-as-creator`), `Build`, `Can`, `CanLocale`, `Filter`, `Fields`, super admin. Motifs de champs à la CASL (sous-chemins, `*` pour un segment). Tests unitaires purs.
- Reste : le test d'intégration des locales avec i18n, et le branchement sur le Content Manager admin (`ADM-003`).

## Notes techniques
- Pas de port de CASL : un évaluateur simple suffit (map `action -> subject -> []rule`). Les conditions produisent des arbres de filtres au format interne de `DB-005`, jamais du SQL.
- Les conditions sont enregistrées en Go (mode framework, `EXT-002`). L'enregistrement en JS viendra avec `EXT-003`.
- Mettre en cache l'ability par utilisateur, puis l'invalider à chaque changement de rôle ou de permission (event hub, `DOC-005`).

## Référence Strapi
- `core/permissions/src/` (engine, domain, abilities)
- `core/admin/server/src/config/admin-actions.ts`, `core/admin/server/src/config/admin-conditions.ts`
- `plugins/i18n/server/src/services/permissions/actions.ts`

## Dépendances
- `SCH-005`, `DB-005`

## Fichiers
- `internal/auth/permissions/{action.go,condition.go,ability.go,ability_test.go}`
