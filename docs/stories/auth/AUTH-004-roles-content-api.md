---
status: in-progress
---

# AUTH-004 : Rôles et permissions du Content API (Public, Authenticated, rôles custom)

## Contexte
Chaque route du Content API porte un scope `api::<api>.<controller>.<action>` ou `plugin::<plugin>.<controller>.<action>`. La stratégie users-permissions associe l'appelant à un rôle (Public sans token, sinon le rôle de l'utilisateur), puis vérifie que le scope figure dans les permissions de ce rôle.

## Objectif
Reproduire le modèle rôles et permissions de users-permissions et son application sur toutes les routes du Content API.

## Périmètre
- Tables `up_roles` (name, description, type unique) et `up_permissions` (action, role).
- Rôles seedés : `public` et `authenticated`. Rôles custom en CRUD via `/users-permissions/roles` (admin) et `/api/users-permissions/roles` (Content API, soumis aux permissions).
- Synchronisation au boot (`syncPermissions`) : suppression des actions qui ne correspondent plus à aucun controller enregistré, et seed de `DEFAULT_PERMISSIONS` :
  - Public : connect, callback, register, local, forgot/reset-password, email-confirmation et send-email-confirmation ;
  - Authenticated : `user.me`, `auth.changePassword`, `auth.logout` et les routes de sessions.
- Listing des actions disponibles par API, controller et action (`getActions`, `getRoutes`), pour l'UI (`ADM-008`).
- Stratégie d'auth `users-permissions` (`API-002`) :
  - avec un bearer JWT : charger l'utilisateur, refuser s'il est bloqué ou non confirmé, puis construire l'ability depuis les permissions de son rôle (`AUTH-001`) ;
  - sans token : utiliser le rôle Public ;
  - `verify` contrôle `scope` : 401 si l'appelant n'est pas authentifié et que Public n'a pas le droit, 403 s'il est authentifié sans le droit.
- Route `config.auth: false` : pas de vérification. Une route sans scope exige un utilisateur connecté.
- Les permissions filtrent aussi le sanitize : une relation vers un type que le rôle ne peut pas lire disparaît de la sortie (`API-005`).

## Hors périmètre
- API tokens (`AUTH-008`), qui sont une autre stratégie sur le même pipeline.

## Critères d'acceptation
- [x] Banc `SETUP-002` : avec Public sans `find` sur `api::post.post`, `GET /api/posts` donne 403 avec le même corps d'erreur que Strapi.
- [x] Après avoir ajouté `find` à Public, la même requête donne 200.
- [ ] Un contrôleur supprimé du projet voit ses permissions purgées au boot (intégration).
- [x] Un populate vers un type non lisible par le rôle est retiré de la réponse (banc).
- [x] **Compat DB (D3)** : les rôles et permissions d'une base Strapi existante s'appliquent sans conversion.

## Avancement
- Fait : vérification des permissions du rôle Public et du rôle de l'utilisateur sur chaque route (`internal/httpapi/auth.go`), restriction des relations en lecture et en écriture. `internal/bootstrap` crée au démarrage les rôles Authenticated et Public et les 13 permissions par défaut de Strapi 5.59 quand la base n'en a aucune, ainsi que les réglages `grant`, `email` et `advanced`.
- Routes du content API `/api/users-permissions/roles` (liste avec `nb_users`, détail avec l'arbre des permissions, création avec `type` dérivé du nom, mise à jour qui synchronise les permissions, suppression qui renvoie les utilisateurs vers Public et refuse de supprimer Public) et `/api/users-permissions/permissions` (`getActions` : actions des content types, des routes Go du projet et des contrôleurs des plugins), dans `internal/httpapi/roles.go`. Corpus `basic_roles` (14 cas, SQLite et Postgres) identique à Strapi, y compris l'ordre non trié de la liste (Strapi passe `sort` à `db.query`, qui l'ignore).
- Divergence voulue : grapi ne purge jamais les permissions des actions qu'il ne connaît pas (Strapi le fait au boot). Tant que les contrôleurs JS custom (`EXT-003`) ne sont pas lus, une purge détruirait les permissions de ces routes sur une base existante. Le critère « contrôleur supprimé, permissions purgées » attend donc `EXT-003`.

- Preuves (banc, 2026-10-23) : `devfreelance_sanitize` passe le rôle Authenticated de 403 (aucune permission, corps identique à Strapi) à 200 sur `/api/posts` après l'ajout de `find` par l'API des rôles ; `author` n'y est pas peuplé faute de `find` sur les utilisateurs ; `basic / create-public-forbidden` donne le même 403. En mode `grapi`, les rôles et permissions viennent de la base créée par Strapi.
- Critère 3 (purge des permissions d'un contrôleur supprimé) : écart volontaire, grapi ne supprime jamais les permissions d'actions qu'il ne connaît pas, car les actions des contrôleurs JS lui sont invisibles et les purger effacerait des permissions valides (`internal/bootstrap`).

## Notes techniques
- Mettre en cache les permissions par rôle, avec invalidation sur écriture dans `up_permissions` (lifecycle `DB-009`).
- L'ordre des stratégies sur le Content API : `api-token` (`AUTH-008`) puis `users-permissions`, comme Strapi.

## Référence Strapi
- `plugins/users-permissions/server/src/strategies/users-permissions.js`
- `plugins/users-permissions/server/src/services/users-permissions.js` (`syncPermissions`, `getActions`, `DEFAULT_PERMISSIONS`)
- `plugins/users-permissions/server/src/services/role.js`
- `core/core/src/services/auth/index.ts`

## Dépendances
- `AUTH-001`, `API-002`, `API-005`, `SCH-005`

## Fichiers
- `internal/auth/users/{role.go,permission.go,strategy.go,sync.go}`
