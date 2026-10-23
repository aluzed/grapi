---
status: done
---

# AUTH-007 : Utilisateurs admin, rôles admin, invitations

## Contexte
Les administrateurs vivent dans `admin_users`, une table distincte des utilisateurs finaux. Ils ont des rôles RBAC (`admin_roles`, `admin_permissions`) évalués par `AUTH-001`. Le premier admin est créé par un écran d'inscription quand aucun admin n'existe.

## Objectif
Le backend de l'administration des admins : création du premier admin, connexion, invitations, mot de passe, rôles et permissions.

## Périmètre
- Modèle `admin::user` : firstname, lastname, username, email, password, resetPasswordToken, registrationToken, isActive, blocked, preferedLanguage, roles. Modèles `admin::role` (name, code, description, permissions) et `admin::permission` (action, actionParameters, subject, properties, conditions, role).
- Rôles seedés : Super Admin (`strapi-super-admin`), Editor (`strapi-editor`), Author (`strapi-author`). Editor et Author reçoivent les permissions Content Manager par défaut sur tous les content types, avec la condition `is-creator` pour Author.
- Endpoints (contrat interne grapi si D1 = B, à documenter dans l'OpenAPI admin `PLG-005`) :
  - `GET /admin/init` (hasAdmin) et `POST /admin/register-admin` (seulement si aucun admin n'existe) ;
  - `POST /admin/login` (avec `AUTH-003`), `POST /admin/forgot-password`, `POST /admin/reset-password` ;
  - invitations : `POST /admin/users` génère un `registrationToken`, puis `GET /admin/registration-info?registrationToken=` et `POST /admin/register` ;
  - `GET|PUT /admin/users/me`, `GET /admin/users/me/permissions` ;
  - CRUD `/admin/users` et `/admin/roles`, `batch-delete`, `GET|PUT /admin/roles/:id/permissions`, `GET /admin/permissions` (catalogue des actions et conditions, groupé par section) ;
  - `POST /admin/permissions/check`.
- Règles :
  - on ne peut pas supprimer le dernier super admin, ni retirer son propre rôle de super admin s'il est le dernier ;
  - un admin désactivé ou bloqué ne peut pas se connecter ;
  - politique de mot de passe : 8 caractères minimum, avec une majuscule, une minuscule et un chiffre ;
  - le mot de passe ne doit pas dépasser 72 octets (limite bcrypt), et l'erreur est explicite.
- Les permissions Content Manager sont générées par content type (`create`, `read`, `update`, `delete`, `publish`), avec les champs et les locales.
- Les champs `createdBy` et `updatedBy` sont renseignés par les actions admin (`DOC-001`).
- Auth admin (`isAuthenticatedAdmin`, `hasPermissions`) : policies (`API-009`).

## Hors périmètre
- UI (`ADM-002`, `ADM-008`), SSO (`XTR-001`), audit logs (`XTR-002`). Pas de limite de sièges : c'est une contrainte de licence Strapi, sans objet ici.

## Critères d'acceptation
- [x] Sur une base vide, `/admin/init` renvoie `hasAdmin: false`. Après `register-admin`, il renvoie `true`, et un second `register-admin` est refusé.
- [x] **Compat DB (D3)** : un admin d'une base Strapi 5.59 se connecte sur grapi avec son mot de passe, et ses rôles et permissions s'appliquent.
- [x] Le flux d'invitation fonctionne de bout en bout (Mailpit sur le homelab).
- [x] Un Author ne voit que ses propres entrées dans le Content Manager (intégration avec `AUTH-001`).
- [x] La suppression du dernier super admin est refusée (TU).
- [x] La CLI `admin:create-user` et `admin:reset-user-password` (`CLI-003`) passent par le même service.

## Avancement
- `internal/adminapi` (`docs/admin-api.md`) : `auth/init`, `auth/register-admin`, `auth/login`, `auth/forgot-password`, `auth/reset-password` (expiration `admin.forgotPassword.expiresIn`), `auth/registration-info`, `auth/register`, `users/me` (GET, PUT), `users/me/permissions`. Politique de mot de passe et limite de 72 octets.
- Rôles Super Admin, Editor et Author créés sur une base sans rôle, avec les permissions Content Manager et médiathèque par défaut (`is-creator` pour Author).
- Banc en mode grapi : un admin enregistré par Strapi 5.59 (`/admin/register-admin`) se connecte sur grapi (`/admin/api/v1/auth/login`), SQLite et Postgres.
- CRUD des admins (invitation par e-mail avec `registrationToken`, recherche, pagination, batch-delete) et des rôles, permissions d'un rôle validées contre le catalogue, catalogue groupé par section, règle du dernier Super Admin actif. Chaque route exige son action admin. Testé de bout en bout (`admin_test.go`, e-mails capturés par un provider de test).
- `POST /permissions/check` : `{permissions: [{action, subject, field}]}` renvoie un booléen par entrée (`TestAdminPermissionsCheck`, permissions par champ comprises).
- Invitation de bout en bout : `make e2e-invite` (`scripts/e2e-invite-mailpit.sh`) lance Mailpit sur le homelab, grapi ici avec le provider SMTP, invite un admin, lit l'e-mail dans Mailpit, suit le lien, inscrit et connecte l'invité. `/auth/init` et le refus d'un second `register-admin` : `TestAdminFirstAdminAndSessions`.

## Notes techniques
- Mémoire projet : **ne jamais vider ni réinitialiser la table des admins** sur un serveur (migrations et `DB-011` comprises).
- bcrypt coût 10, comme `AUTH-002`.

## Référence Strapi
- `core/admin/server/src/content-types/{User,Role,Permission}.ts`
- `core/admin/server/src/services/{user,role,auth,permission}.ts` (Super Admin, Editor et Author créés dans `role.ts`)
- `core/admin/server/src/routes/{authentication,users,roles,permissions}.ts`
- `core/admin/server/src/config/admin-actions.ts`, `.../admin-conditions.ts`
- `core/admin/server/src/validation/common-validators.ts` (règles de mot de passe)

## Dépendances
- `AUTH-001`, `AUTH-003`, `API-009`, `PLG-002`

## Fichiers
- `internal/auth/admin/{model.go,service.go,routes.go,seed.go,password.go}`
