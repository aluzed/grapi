# API admin (`/admin/api/v1`)

L'API de l'admin de grapi (décision D1, option B : `docs/decisions/D1-admin.md`). C'est un contrat propre à grapi, versionné par le segment `v1`. Il n'imite pas les routes internes de l'admin Strapi. Il travaille sur les tables admin d'une base Strapi 5 (`admin_users`, `admin_roles`, `admin_permissions`, `strapi_sessions`) : un admin créé par Strapi se connecte sur grapi avec son mot de passe.

Le préfixe suit `admin.url` (`/admin` par défaut). Le reste de `/admin` sert la SPA (`admin/`), ou une page d'attente si le binaire a été compilé sans elle.

## Format
- Succès : `{"data": ...}`.
- Erreur : `{"data": null, "error": {"status", "name", "message", "details"}}`. Une erreur de validation porte `details.errors: [{path: [...], message}]`.
- Les routes authentifiées attendent `Authorization: Bearer <access token>`. Sans token valide : 401 `UnauthorizedError`.

## Sessions
- Une connexion ouvre une session dans `strapi_sessions` (origine `admin`) et renvoie un access token (30 minutes par défaut) dans le corps.
- Le refresh token est dans le cookie httpOnly `grapi_admin_refresh`, limité au chemin `/admin/api/v1/auth`.
  - Avec `rememberMe`, la session est de type `refresh` et le cookie est persistant.
  - Sans, la session est de type `session` et le cookie meurt avec le navigateur.
- `POST /auth/access-token` fait tourner le refresh token (nouveau cookie) et renvoie un nouvel access token. Rejouer un refresh token déjà utilisé révoque toutes les sessions de cet appareil, sauf dans la minute qui suit sa rotation et tant que le token suivant n'a pas lui-même tourné : la requête reçoit alors le même token suivant. Les onglets d'un navigateur partagent le cookie et peuvent renouveler en même temps.
- Durées : `admin.auth.sessions.{accessTokenLifespan, maxRefreshTokenLifespan, idleRefreshTokenLifespan, maxSessionLifespan, idleSessionLifespan}`, en secondes, avec les défauts de Strapi (30 min, 30 j, 14 j, 1 j, 2 h).
- Cookie : `admin.auth.cookie.{secure, sameSite, domain}`. Sans `secure` explicite, le cookie est `Secure` derrière HTTPS (ou `X-Forwarded-Proto: https` avec `server.proxy`).
- Secret : `admin.auth.secret`, sinon `ADMIN_JWT_SECRET`. S'il manque, grapi en génère un et l'ajoute au `.env`.

## Routes

| Méthode et chemin | Auth | Rôle |
|---|---|---|
| `GET /auth/init` | non | `{hasAdmin}` |
| `POST /auth/register-admin` | non | Crée le premier Super Admin (`firstname`, `lastname`, `email`, `password`), refusé dès qu'un admin existe. Renvoie `{token, user}`. |
| `POST /auth/login` | non | `email`, `password`, `rememberMe`, `deviceId` facultatif. Renvoie `{token, user}`. |
| `POST /auth/access-token` | cookie | `{token}` et un nouveau cookie |
| `POST /auth/logout` | cookie | Ferme les sessions de l'appareil du cookie |
| `POST /auth/forgot-password` | non | `email`. Répond toujours `{}` ; envoie le lien `<admin.absoluteUrl>/auth/reset-password?code=` si le compte existe et est actif. |
| `POST /auth/reset-password` | non | `resetPasswordToken`, `password`. Token valable `admin.forgotPassword.expiresIn` (1 h). Ferme les autres sessions et renvoie `{token, user}`. |
| `GET /auth/registration-info?registrationToken=` | non | `{email, firstname, lastname}` d'une invitation |
| `POST /auth/register` | non | `registrationToken`, `userInfo: {firstname, lastname, password}`. Active le compte et renvoie `{token, user}`. |
| `GET /users/me` | oui | L'admin connecté, avec ses rôles |
| `PUT /users/me` | oui | `firstname`, `lastname`, `username`, `email`, `preferedLanguage` (`en`, `fr`), `currentPassword` + `password`. Changer le mot de passe ferme les sessions des autres appareils. |
| `GET /users/me/permissions` | oui | `[{action, subject, properties, conditions}]`. Le Super Admin reçoit toutes les actions enregistrées. |
| `GET /users/me/sessions` | oui | Sessions actives, `isCurrent` pour celle de la requête |
| `DELETE /users/me/sessions/{id}` | oui | Révoque une session (et son appareil) |
| `DELETE /users/me/sessions` | oui | Révoque toutes les sessions des autres appareils |
| `GET /information` | oui | `{apiVersion, develop, languages}` |

## Règles
- Mot de passe : 8 caractères au moins, une minuscule, une majuscule, un chiffre, 72 octets au plus (limite de bcrypt). Hash bcrypt de coût 10, comme Strapi.
- Un admin inactif ou bloqué ne peut ni se connecter ni utiliser un token déjà émis.
- Au premier démarrage sur une base sans rôle admin, grapi crée Super Admin, Editor et Author (codes `strapi-super-admin`, `strapi-editor`, `strapi-author`). Editor reçoit les permissions du Content Manager sur les content types `api::` et celles de la médiathèque ; Author les mêmes sans `publish`, limitées à ses propres entrées (`admin::is-creator`). Une base qui a déjà des rôles n'est jamais modifiée.
