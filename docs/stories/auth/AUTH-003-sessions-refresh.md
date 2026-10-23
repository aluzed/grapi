---
status: in-progress
---

# AUTH-003 : Sessions et refresh tokens (partagé entre admin et utilisateurs finaux)

## Contexte
Strapi 5 a introduit un gestionnaire de sessions dans le core (`core/core/src/services/session-manager.ts`). L'admin s'en sert toujours, avec un access token court et un refresh token dans un cookie httpOnly. users-permissions s'en sert quand `jwtManagement: 'refresh'`.

## Objectif
Un service de sessions unique, utilisé par `AUTH-007` (admin) et par `AUTH-002` en mode refresh.

## Périmètre
- Modèle de session : userId, sessionId, childId (rotation), deviceId, origin (`admin` ou `users-permissions`), type (`session` ou `refresh`), status, expiresAt, absoluteExpiresAt, metadata.
- Durées configurables :
  - côté admin : `admin.auth.sessions.accessTokenLifespan`, `maxRefreshTokenLifespan`, `idleRefreshTokenLifespan`, `maxSessionLifespan`, `idleSessionLifespan` ;
  - côté utilisateurs finaux : `plugin users-permissions.sessions.*`.
- Rotation du refresh token à chaque usage. La réutilisation d'un refresh token déjà consommé révoque toute la chaîne (détection de vol).
- users-permissions en mode `refresh` :
  - `POST /auth/local` renvoie `{jwt, refreshToken, user}`. Si l'en-tête `x-strapi-refresh-cookie: httpOnly` est présent, le refresh token passe dans le cookie `strapi_up_refresh` au lieu du corps.
  - Routes `POST /auth/refresh`, `POST /auth/logout`, `GET /auth/sessions`, `DELETE /auth/sessions/:id`.
- Côté admin : `POST /admin/access-token`, `POST /admin/logout`, `GET|DELETE /admin/users/me/sessions[/:sessionId]`. « Remember me » donne le type `refresh` (cookie persistant), sinon le type `session` (cookie de session).
- Signature : `jwtSecret` HS256, ou RSA via `jwtOptions` si fourni.
- Purge périodique des sessions expirées (`EXT-005`).

## Hors périmètre
- SSO (`XTR-001`).

## Critères d'acceptation
- [x] Banc `SETUP-002` en mode refresh : login, refresh, logout et sessions ont les mêmes formes de réponse et les mêmes cookies (nom, `HttpOnly`, `SameSite`, `Secure` derrière HTTPS) que Strapi 5.59.
- [x] Rejouer un refresh token déjà utilisé donne 401 et révoque la session (TU + intégration).
- [x] L'expiration idle et l'expiration absolue sont respectées (tests avec une horloge injectable).
- [ ] Un access token admin expiré renvoie 401 ; la SPA appelle alors `/admin/access-token` et obtient un nouveau token.

## Avancement
- Routes `POST /auth/refresh`, `POST /auth/logout`, `GET /auth/sessions` et `DELETE /auth/sessions/:sessionId` présentes, avec leurs permissions (Public pour `refresh`, Authenticated pour les autres, seedées au bootstrap). En mode `legacy-support` (défaut de Strapi), elles répondent 404 comme Strapi : corpus `basic_roles`, SQLite et Postgres.
- Gestionnaire de sessions `internal/auth/session` sur `strapi_sessions` : rotation, rejeu détecté (révocation de l'appareil), expirations idle et absolue, horloge injectable (tests unitaires sur un store mémoire).
- Côté admin, branché dans l'API `/admin/api/v1` (option B) : login, `access-token`, `logout`, sessions du profil, cookie httpOnly. Testé de bout en bout (`admin_test.go`). Le contrat est celui de grapi (`docs/admin-api.md`), pas les routes `/admin/*` de Strapi.
- Purge des sessions expirées par le planificateur du serveur (au démarrage puis au plus une fois par heure), en plus de celle déclenchée toutes les 50 connexions (compteur désormais atomique) ; `TestAdminPurgesExpiredSessions`.
- Mode `refresh` de users-permissions (`internal/httpapi/users_sessions.go`), repris du plugin MIT de Strapi 5.59 :
  - connexion : `{jwt, refreshToken, user}`, ou le refresh token dans le cookie `strapi_up_refresh` (nom, `sameSite`, `path`, `domain`, `secure`, `maxAge` de `sessions.cookie`) avec `sessions.httpOnly` ou l'en-tête `x-strapi-refresh-cookie: httpOnly` ; `deviceId` du corps, métadonnées `loginAt` et `deviceName` (« Chrome on macOS », heuristique de `@strapi/utils`) ;
  - inscription : refresh token toujours dans le corps ; changement et réinitialisation du mot de passe : toutes les sessions fermées puis une nouvelle ;
  - `POST /auth/refresh` (400 sans token, 401 s'il est invalide, rotation), `POST /auth/logout` (`scope: all`, `deviceId`, ou la session courante ; cookie effacé), `GET /auth/sessions` (session courante d'abord, puis la plus récente), `DELETE /auth/sessions/:id` (404 « Session not found ») ;
  - seuls les access tokens d'une session vivante sont acceptés ; durées par défaut de Strapi pour cette origine (access 10 min) ; `sessions.*` en secondes.
  - Testé : `TestUsersPermissionsRefreshMode`, `TestUsersPermissionsRefreshCookie`, `TestUsersPermissionsLegacyModeHidesSessions`, et le corpus de banc `sessions` (fixture `sessions`, `upConfig.jwtManagement` dans `compat.json`) : 21 cas identiques à Strapi 5.59 sous SQLite et Postgres (identifiants d'appareil générés en UUID v4 comme Strapi). Le runner compare maintenant les `Set-Cookie` (nom, attributs, effacement, sans la valeur) : 21 cas identiques à Strapi sous SQLite et Postgres, y compris le cookie de signature `strapi_up_refresh.sig` (keygrip, HMAC-SHA1 avec la première clé de `server.app.keys`). La fixture fixe `cookie.secure: false` : Strapi tourne en production dans le banc et Koa refuse un cookie `Secure` en HTTP simple (500).
- Rejeu : `TestReplayRevokesTheFamily` et le voleur de `TestAdminFirstAdminAndSessions` ; expirations : `TestIdleAndAbsoluteExpiry`.
  - Divergences assumées (`docs/divergences.md`) : access token refusé dès la révocation de sa session ; vrai token à la confirmation d'e-mail.

## Notes techniques
- Horloge injectable (`func() time.Time`) partout pour tester les expirations.
- Stocker en base un hash SHA-256 du refresh token, jamais le token en clair.
- Le cookie admin doit respecter `admin.auth.cookie` (domain, path, sameSite) et `server.proxy` pour décider de `Secure` (`API-001`).

## Référence Strapi
- `core/core/src/services/session-manager.ts`, `core/core/src/providers/session-manager.ts`
- `core/admin/server/src/content-types/session.ts`
- `core/admin/server/src/routes/authentication.ts`
- `plugins/users-permissions/server/src/config.js` (`jwtManagement`, `sessions`)
- `core/admin/admin/src/utils/getFetchClient.ts` (côté client, logique de refresh)

## Dépendances
- `AUTH-002`, `DB-010`, `EXT-001`

## Fichiers
- `internal/auth/session/{model.go,manager.go,cookie.go,manager_test.go}`
