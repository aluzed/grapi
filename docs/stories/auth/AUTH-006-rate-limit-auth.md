---
status: in-progress
---

# AUTH-006 : Rate limiting des routes d'authentification

## Contexte
Strapi limite les routes sensibles d'users-permissions (koa2-ratelimit) et de l'admin (login, forgot/reset password). Le correctif de sécurité GHSA-7mqx a changé la clé de limitation pour empêcher qu'on la contourne en variant l'identifiant.

## Objectif
Le même rate limiting, avec les mêmes clés, la même configuration et le même format d'erreur.

## Périmètre
- Configuration users-permissions : `ratelimit {interval: 60000, max: 10}` (fenêtre en millisecondes), désactivable avec `enabled: false`.
- La clé vaut `email:path:ip`. Sur `/auth/local`, `/auth/reset-password` et `/auth/change-password`, elle vaut `noIdentifier:path:ip`, pour qu'on ne puisse pas la contourner en changeant d'identifiant à chaque essai.
- Routes concernées : auth local, register, forgot/reset/change password, send-email-confirmation, connect et callback, ainsi que `POST /api/email` (`PLG-002`).
- Admin : `admin.rateLimit` (enabled, interval, max) sur login, forgot et reset password.
- Dépassement : 429 avec `RateLimitError` (« Too many requests, please try again later. ») au format d'erreur `API-007`.
- IP réelle : `ctx.request.ip`, en tenant compte de `server.proxy` (`ipHeader`, `maxIpsCount`).
- Stockage en mémoire par défaut. Interface prévue pour un stockage Redis (multi-instance).

## Hors périmètre
- Le rate limiting global de l'API (non fourni par Strapi). Il peut faire l'objet d'un middleware optionnel plus tard.

## Critères d'acceptation
- [x] La 11e tentative de login en moins de 60 s depuis la même IP donne 429 (intégration).
- [x] Changer d'`identifier` à chaque essai ne contourne pas la limite sur `/auth/local` (régression GHSA-7mqx).
- [x] Derrière un proxy avec `X-Forwarded-For`, c'est l'IP client qui sert de clé, seulement si `server.proxy` est actif.
- [x] Le corps de la réponse 429 est identique à celui de Strapi (banc `SETUP-002`).

## Avancement
- Fait : middleware `rateLimit` de users-permissions sur auth local, register, forgot, reset, change-password et send-email-confirmation, clé `buildPrefixKey` à l'identique (routes sans identifiant comprises), fenêtre fixe en mémoire comme koa2-ratelimit, en-têtes `X-RateLimit-*` et `Retry-After`, 429 `RateLimitError`, config `ratelimit` (`enabled`, `interval` en ms ou `{min: 5}`, `max`), IP réelle avec `server.proxy`. Banc dev-freelance : la 11e connexion sur `/api/auth/local` donne le même 429 que Strapi (`ratelimit-login-11`).
- Vérifié dans Strapi 5.59 : `POST /api/email` n'a pas de rate limit ; les routes limitées sont les 6 de grapi plus `/connect/(.*)`, qui viendra avec `AUTH-005`. Les mutations GraphQL d'auth partagent le compteur des routes REST (`PLG-004`).
- Critères prouvés : `TestLoginRateLimitIgnoresIdentifier` (11 identifiants différents et un `X-Forwarded-For` falsifié : la 11e tentative donne 429), `TestLoginRateLimitUsesProxyClientIP` (`server.proxy.koa` : une IP client par compteur ; `maxIpsCount` écarte une première adresse forgée), banc dev-freelance `ratelimit-login-11` pour le corps du 429.
- `server.proxy` lu comme Strapi 5 : `koa`, `ipHeader`, `maxIpsCount` (`internal/clientip`, port de `request.ip` de Koa). Un `proxy: true` booléen ne fait plus confiance aux en-têtes (Strapi ne lit que `proxy.koa`) : avant, grapi croyait `X-Forwarded-For` là où Strapi l'ignore, ce qui permettait de contourner la limite.
- Reste : `/connect/(.*)` (avec `AUTH-005`), routes admin (`admin.rateLimit`), stockage Redis.

## Notes techniques
- `ulule/limiter/v3` (mémoire + Redis) ou un compteur maison à fenêtre fixe. Le comportement de koa2-ratelimit est une fenêtre fixe, à reproduire pour ne pas surprendre.

## Référence Strapi
- `plugins/users-permissions/server/src/middlewares/rateLimit.js`
- `plugins/users-permissions/server/src/config.js`
- `core/admin/server/src/middlewares/rateLimit.ts`

## Dépendances
- `AUTH-002`, `API-002`, `API-007`

## Fichiers
- `internal/auth/ratelimit/{limiter.go,middleware.go,limiter_test.go}`
