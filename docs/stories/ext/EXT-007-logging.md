---
status: todo
---

# EXT-007 : Logs (pas de télémétrie)

## Contexte
Strapi journalise avec winston (niveau par défaut `http`) et envoie de la télémétrie anonyme. grapi journalise de façon structurée et n'envoie rien.

## Objectif
Des logs structurés configurables, sans aucune télémétrie.

## Périmètre
- `log/slog`, sortie texte en dev, JSON en production (`server.logger.format`).
- Niveaux : `error`, `warn`, `info`, `http`, `debug` ; niveau par défaut `http`, comme Strapi.
- Request id propagé (en-tête `X-Request-Id` lu ou généré, renvoyé dans la réponse).
- Bannière de démarrage : version, environnement, URL du serveur et de l'admin, base utilisée.
- Aucune télémétrie, aucun appel réseau sortant non demandé.

## Hors périmètre
- Sentry (`PLG-006`), audit logs (`XTR-002`).

## Critères d'acceptation
- [ ] Une requête produit une ligne `http` avec méthode, chemin, statut, durée et request id.
- [ ] Aucune connexion sortante au démarrage avec la config par défaut (vérifié par test sur un réseau isolé).

## Notes techniques
- Les clés `STRAPI_TELEMETRY_DISABLED` et `telemetry:*` sont acceptées sans effet, pour ne pas casser les configs migrées.

## Référence Strapi
- `utils/logger/src/`, `core/core/src/middlewares/logger.ts`, `core/core/src/services/metrics/`

## Dépendances
- `SETUP-001`, `EXT-001`

## Fichiers
- `internal/logging/logger.go`, `internal/httpapi/middleware/logger.go`
