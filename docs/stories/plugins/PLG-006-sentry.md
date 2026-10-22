---
status: todo
---

# PLG-006 : Sentry

## Contexte
Le plugin sentry de Strapi capture les erreurs serveur et les envoie à Sentry, avec des métadonnées sur la requête et l'utilisateur.

## Objectif
L'équivalent pour grapi, activable par configuration.

## Périmètre
- Configuration : `dsn`, `sendMetadata` (true par défaut), `init` (options du SDK : environment, release, tracesSampleRate).
- Middleware : capture des erreurs 5xx, et des panics récupérés par le middleware `errors` (`API-008`).
- Service `SendError(err, func(scope))` utilisable depuis les hooks.
- Métadonnées : méthode, URL sans query sensible, id utilisateur ou admin si connu. Jamais de corps de requête, jamais d'en-têtes d'auth.
- Inactif sans `dsn`.

## Hors périmètre
- Tracing de performance avancé.

## Critères d'acceptation
- [ ] Une erreur 500 provoquée est reçue par un Sentry de test (ou par un transport mock en TU).
- [ ] Les erreurs 4xx ne sont pas envoyées.
- [ ] Aucun en-tête `Authorization` ni cookie n'est transmis (TU sur l'événement construit).

## Notes techniques
- `getsentry/sentry-go`, avec le hub par requête via `context`.

## Référence Strapi
- `plugins/sentry/server/src/`

## Dépendances
- `API-008`, `EXT-001`

## Fichiers
- `internal/plugins/sentry/{plugin.go,middleware.go}`
