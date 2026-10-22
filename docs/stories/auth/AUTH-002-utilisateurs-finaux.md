---
status: in-progress
---

# AUTH-002 : Utilisateurs finaux : inscription, connexion, JWT, confirmation e-mail, mot de passe, `/users/me`, CRUD

## Contexte
Le plugin users-permissions gère les utilisateurs du site (table `up_users`), qui sont distincts des admins. Ses routes sont publiques et utilisées par les fronts : leur compatibilité fil à fil est obligatoire (D1 : l'API de contenu est compatible).

## Objectif
Reproduire l'authentification locale des utilisateurs finaux, avec les mêmes routes, les mêmes corps de requête et de réponse, et les mêmes réglages.

## Périmètre
- Modèle `plugin::users-permissions.user` (`up_users`) : username, email, provider, password (privé), resetPasswordToken (privé), confirmationToken (privé), confirmed, blocked, role. Rôles dans `up_roles`, permissions dans `up_permissions` (`AUTH-004`).
- Routes Content API (préfixe `/api`) :
  - `POST /auth/local` (`identifier` = email ou username, `password`) renvoie `{jwt, user}`.
  - `POST /auth/local/register` : champs autorisés par `register.allowedFields` en plus de username, email, password. Le rôle par défaut vient de `advanced.default_role`.
  - `POST /auth/forgot-password` (`email`) et `POST /auth/reset-password` (`code`, `password`, `passwordConfirmation`).
  - `POST /auth/change-password` (`currentPassword`, `password`, `passwordConfirmation`), réservé aux utilisateurs authentifiés.
  - `GET /auth/email-confirmation?confirmation=` (redirige vers `email_confirmation_redirection`) et `POST /auth/send-email-confirmation`.
  - `GET /users`, `GET /users/count`, `GET /users/me`, `GET|PUT|DELETE /users/:id`, `POST /users`.
- Réglages avancés, stockés dans le core store (`DB-010`, clé `plugin_users-permissions_advanced`) : `unique_email`, `allow_register`, `email_confirmation`, `email_reset_password` (URL), `email_confirmation_redirection`, `default_role`.
- Templates e-mail (clé `plugin_users-permissions_email`) : `reset_password` et `email_confirmation`, avec from, response_email, object et message. Les variables autorisées sont limitées : `URL`, `SERVER_URL`, `ADMIN_URL`, `CODE`, `TOKEN`, `USER.*`. Pas d'évaluation de code (voir `PLG-002`).
- Mode `jwtManagement: legacy-support` (défaut) : JWT HS256 signé avec `jwtSecret` (env `JWT_SECRET`), `expiresIn` par défaut de `30d`, payload `{id}`. Le mode `refresh` est traité dans `AUTH-003`.
- Si `JWT_SECRET` est absent au premier démarrage, on génère un secret et on l'ajoute au `.env`, comme Strapi, avec un log d'avertissement.
- Un utilisateur bloqué, ou non confirmé alors que la confirmation est active, est refusé à la connexion et sur toute requête authentifiée.

## Hors périmètre
- OAuth (`AUTH-005`), rate limiting (`AUTH-006`), sessions et refresh (`AUTH-003`).

## Critères d'acceptation
- [ ] **Compat DB (D3)** : un utilisateur créé par Strapi 5.59, dont le hash bcryptjs est en `$2a$10$...`, se connecte avec son mot de passe sur grapi, sans migration.
- [x] Un JWT émis par Strapi avec le même `JWT_SECRET` est accepté par grapi, et inversement.
- [ ] Banc `SETUP-002` : register, login, me, forgot, reset, change-password et email-confirmation donnent un JSON identique (champs privés absents), avec les mêmes codes HTTP et les mêmes messages d'erreur (`ValidationError` « Invalid identifier or password », etc.).
- [ ] `allow_register: false` donne 403 sur register. `unique_email` est respecté.
- [ ] Le lien de reset et de confirmation reçu par e-mail (Mailpit sur le homelab) fonctionne de bout en bout.
- [ ] `password` n'apparaît jamais dans une réponse, y compris en populate depuis un autre type.

## Avancement
- Fait : JWT HS256 identique à jsonwebtoken 9 (`internal/auth/users`), stratégies api-token puis users-permissions dans l'ordre de Strapi, `POST /auth/local`, `/auth/local/register`, `/auth/change-password`, `GET /users/me`, CRUD `/users`, génération de `JWT_SECRET` dans `.env`. Les routes HTTP sont dans `internal/httpapi/users.go`.
- Fait aussi : forgot-password, reset-password, email-confirmation, send-email-confirmation, et la confirmation à l'inscription (`internal/httpapi/users_email.go`) ; forgot-password vérifié de bout en bout vers Mailpit. Banc dev-freelance : 116/116.
- Reste : `validatePassword` de la config (JS), le lien de confirmation suivi de bout en bout, `unique_email` entre providers (avec `AUTH-005`). `allow_register: false` et le refus d'un e-mail déjà pris pour le même provider (même sans `unique_email`, comme Strapi) sont couverts par `TestRegisterSettings`.

## Notes techniques
- `golang.org/x/crypto/bcrypt` lit les préfixes `$2a$`, `$2b$` et `$2y$`. Garder le coût 10 à l'écriture pour rester interopérable.
- `golang-jwt/jwt/v5`, avec une durée `expiresIn` parsée au format `ms` de Node (`30d`, `7h`, `60s`) : écrire un petit parseur.
- Tokens de reset et de confirmation : 64 octets aléatoires en hexadécimal, comme Strapi (crypto/rand).

## Référence Strapi
- `plugins/users-permissions/server/src/config.js`
- `plugins/users-permissions/server/src/routes/content-api/auth.js`, `.../user.js`
- `plugins/users-permissions/server/src/controllers/auth.js`, `.../user.js`
- `plugins/users-permissions/server/src/services/user.js`, `.../jwt.js`, `.../users-permissions.js`
- `plugins/users-permissions/server/src/bootstrap/index.js` (valeurs par défaut des réglages et templates)

## Dépendances
- `AUTH-004`, `API-006`, `DB-010`, `PLG-002`, `DOC-001`

## Fichiers
- `internal/auth/users/{model.go,routes.go,controller.go,service.go,jwt.go,settings.go}`
- `internal/auth/users/*_test.go`, `compat/cases/users-permissions/*.yaml`
