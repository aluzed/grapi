---
status: done
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
- [x] Une requête produit une ligne `http` avec méthode, chemin, statut, durée et request id.
- [x] Aucune connexion sortante au démarrage avec la config par défaut (vérifié par test sur un réseau isolé).

## Avancement
- `internal/logging` : niveaux de Strapi sur `log/slog` (`http` entre debug et info, niveau par défaut comme Strapi), texte en développement, JSON en production (`GRAPI_LOG_FORMAT` ou `logger.format`).
- `internal/httpapi/logger.go` : une ligne `http` par requête au format du middleware `strapi::logger` (`GET /api/x?y (12 ms) 200`), avec méthode, chemin, statut, durée et request id (`X-Request-Id` lu s'il est valide, sinon généré, renvoyé dans la réponse). Test `TestRequestLog`.
- Bannière de démarrage : version, adresse, environnement, URL du serveur et de l'admin, base de données.
- `compat/scripts/offline-start.sh [fixture]` démarre grapi dans un conteneur sans réseau (`--network none`, homelab) : `/_health` répond 204, aucune tentative de connexion ni erreur dans les logs (devfreelance, 2026-10-22).

## Notes techniques
- Les clés `STRAPI_TELEMETRY_DISABLED` et `telemetry:*` sont acceptées sans effet, pour ne pas casser les configs migrées.

## Référence Strapi
- `utils/logger/src/`, `core/core/src/middlewares/logger.ts`, `core/core/src/services/metrics/`

## Dépendances
- `SETUP-001`, `EXT-001`

## Fichiers
- `internal/logging/logger.go`, `internal/httpapi/middleware/logger.go`
