---
status: in-progress
---

# API-008 : Middlewares globaux

## Contexte
L'ordre et la configuration des middlewares globaux viennent de `config/middlewares`. Strapi impose certains d'entre eux.

## Objectif
Les mêmes middlewares, mêmes valeurs par défaut, même mécanisme d'ordre et de configuration.

## Périmètre
- Liste par défaut, dans cet ordre : logger, errors, security, cors, session, query, body, favicon, public. Obligatoires : errors, security, cors, query, body, public, favicon.
- Disponibles en plus : compression, ip (liste blanche et noire), responseTime, responses (corps personnalisé par code).
- Entrées : nom (`strapi::cors`, gardé pour compatibilité de config, alias `grapi::cors`), `{name, config}` ou middleware Go enregistré (`global::...`).

| Middleware | Comportement par défaut |
|---|---|
| logger | `METHOD url (ms) status` au niveau http |
| errors | Format `API-007`, 404 si aucun statut |
| security | En-têtes à la helmet : CSP par défaut, HSTS 1 an avec sous-domaines, frameguard sameorigin, COEP/COOP/CORP désactivés ; CSP assouplie pour `/documentation`, le playground GraphQL et l'admin en dev |
| cors | origin `*`, maxAge 31536000, credentials true, méthodes usuelles, en-têtes Content-Type, Authorization, Origin, Accept ; origin en tableau, chaîne séparée par des virgules ou fonction Go |
| session | Cookie signé `koa.sess`, 1 jour, httpOnly ; erreur au démarrage si `app.keys` manque |
| query | Parseur `API-003` avec `strictNullHandling`, `arrayLimit` 100, `depth` 20 |
| body | JSON, form, multipart ; fichiers temporaires supprimés après la requête ; `maxFileSize` donne 413 |
| favicon | `favicon.png` ou `.ico` du projet |
| public | Fichiers statiques de `public/` (cache 60 s), saute `/uploads` si un provider distant est utilisé, redirige `/` vers l'admin |

## Hors périmètre
- Le rate limit d'authentification (`AUTH-006`).

## Critères d'acceptation
- [ ] Les en-têtes de réponse d'une requête `/api/posts` sont identiques à Strapi, sauf `X-Powered-By` qui est absent : banc de compatibilité.
- [ ] Retirer un middleware obligatoire de la config donne une erreur claire au démarrage.
- [x] Préflight CORS identique à Strapi pour une origine listée et une origine refusée.
- [ ] Upload de 2 Go avec `maxFileSize` 1 Go : 413 sans saturer la mémoire (streaming vers fichier temporaire).

## Avancement
- Fait : `strapi::security` (en-têtes koa-helmet et CSP par défaut de Strapi 5.59, relevés sur l'image de référence, directives configurables) et `strapi::cors` (port de `matchOrigin` et de `@koa/cors`), lus depuis `config/middlewares.*` ; `internal/httpapi/middleware.go`.
- `strapi::body` : limites de koa-body (`jsonLimit`, `formLimit`, `textLimit`, défauts 1mb, 56kb, 56kb), 413 `PayloadTooLargeError` sans `details` avant le routage et l'authentification, comme Strapi. Corpus `basic_limits` (option `pad` du runner pour gonfler un corps), SQLite et Postgres ; `TestBodyLimits`.
- Reste : comparaison des en-têtes sur le banc, middlewares custom, erreur au démarrage si un middleware obligatoire manque.

## Notes techniques
- **Pas de middleware poweredBy, et aucun en-tête `X-Powered-By`** (décision du 2026-10-22). Il ne sert à rien et signale le logiciel à un attaquant qui cherche une CVE. Le serveur ne révèle ni son nom ni sa version, et le nom `strapi::poweredBy` est ignoré, avec un avertissement, dans une config migrée. De même, `/_health` ne renvoie pas l'en-tête `strapi: You are so French!`.
- Pas de dépendance helmet en Go : en-têtes écrits à la main, testés.

## Référence Strapi
- `core/core/src/middlewares/` (body, compression, cors, errors, favicon, ip, logger, powered-by, public, query, response-time, responses, security, session)
- `core/core/src/services/server/register-middlewares.ts`, `middleware.ts`

## Dépendances
- `API-001`, `API-003`, `API-007`, `EXT-001`

## Fichiers
- `internal/httpapi/middleware/*.go`, `internal/httpapi/middleware/registry.go`
