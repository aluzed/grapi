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
- [ ] Banc `SETUP-002` en mode refresh : login, refresh, logout et sessions ont les mêmes formes de réponse et les mêmes cookies (nom, `HttpOnly`, `SameSite`, `Secure` derrière HTTPS) que Strapi 5.59.
- [ ] Rejouer un refresh token déjà utilisé donne 401 et révoque la session (TU + intégration).
- [ ] L'expiration idle et l'expiration absolue sont respectées (tests avec une horloge injectable).
- [ ] Un access token admin expiré renvoie 401 ; la SPA appelle alors `/admin/access-token` et obtient un nouveau token.

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
