# API-008 : Middlewares globaux

## Contexte
L'ordre et la configuration des middlewares globaux viennent de `config/middlewares`. Strapi impose certains d'entre eux.

## Objectif
Les mêmes middlewares, mêmes valeurs par défaut, même mécanisme d'ordre et de configuration.

## Périmètre
- Liste par défaut, dans cet ordre : logger, errors, security, cors, poweredBy, session, query, body, favicon, public. Obligatoires : errors, security, cors, query, body, public, favicon.
- Disponibles en plus : compression, ip (liste blanche et noire), responseTime, responses (corps personnalisé par code).
- Entrées : nom (`strapi::cors`, gardé pour compatibilité de config, alias `grapi::cors`), `{name, config}` ou middleware Go enregistré (`global::...`).

| Middleware | Comportement par défaut |
|---|---|
| logger | `METHOD url (ms) status` au niveau http |
| errors | Format `API-007`, 404 si aucun statut |
| security | En-têtes à la helmet : CSP par défaut, HSTS 1 an avec sous-domaines, frameguard sameorigin, COEP/COOP/CORP désactivés ; CSP assouplie pour `/documentation`, le playground GraphQL et l'admin en dev |
| cors | origin `*`, maxAge 31536000, credentials true, méthodes usuelles, en-têtes Content-Type, Authorization, Origin, Accept ; origin en tableau, chaîne séparée par des virgules ou fonction Go |
| poweredBy | `X-Powered-By` (valeur grapi, configurable) |
| session | Cookie signé `koa.sess`, 1 jour, httpOnly ; erreur au démarrage si `app.keys` manque |
| query | Parseur `API-003` avec `strictNullHandling`, `arrayLimit` 100, `depth` 20 |
| body | JSON, form, multipart ; fichiers temporaires supprimés après la requête ; `maxFileSize` donne 413 |
| favicon | `favicon.png` ou `.ico` du projet |
| public | Fichiers statiques de `public/` (cache 60 s), saute `/uploads` si un provider distant est utilisé, redirige `/` vers l'admin |

## Hors périmètre
- Le rate limit d'authentification (`AUTH-006`).

## Critères d'acceptation
- [ ] Les en-têtes de réponse d'une requête `/api/posts` sont identiques à Strapi (hors `X-Powered-By`) : banc de compatibilité.
- [ ] Retirer un middleware obligatoire de la config donne une erreur claire au démarrage.
- [ ] Préflight CORS identique à Strapi pour une origine listée et une origine refusée.
- [ ] Upload de 2 Go avec `maxFileSize` 1 Go : 413 sans saturer la mémoire (streaming vers fichier temporaire).

## Notes techniques
- `X-Powered-By: Strapi` n'est pas repris par défaut (marque, D5) ; option pour le forcer si un client en dépend.
- Pas de dépendance helmet en Go : en-têtes écrits à la main, testés.

## Référence Strapi
- `core/core/src/middlewares/` (body, compression, cors, errors, favicon, ip, logger, powered-by, public, query, response-time, responses, security, session)
- `core/core/src/services/server/register-middlewares.ts`, `middleware.ts`

## Dépendances
- `API-001`, `API-003`, `API-007`, `EXT-001`

## Fichiers
- `internal/httpapi/middleware/*.go`, `internal/httpapi/middleware/registry.go`
