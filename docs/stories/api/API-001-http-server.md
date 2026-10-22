---
status: in-progress
---

# API-001 : Serveur HTTP, contexte de requête, /_health, arrêt propre

## Contexte
Strapi tourne sur Koa avec un contexte de requête asynchrone, deux portées d'API (`content-api` sous `/api`, `admin` sous `/admin` et `/<plugin>`) et un endpoint de santé utilisé par les orchestrateurs (Dokploy, Docker healthcheck).

## Objectif
Le serveur HTTP de grapi, sa configuration réseau et son cycle de vie.

## Périmètre
- Écoute sur `server.host` et `server.port` (défaut `0.0.0.0:1337`) ou sur une socket Unix (`server.socket`).
- `server.proxy` : prise en compte de `X-Forwarded-*` (`ipHeader`, `maxIpsCount`) derrière un reverse proxy.
- `server.http.serverOptions` : timeouts de lecture, d'écriture et d'inactivité.
- `GET|HEAD /_health` : `204` sans corps et sans en-tête qui identifie le logiciel (pas de `strapi: You are so French!`, voir `API-008`). Les sondes Dokploy et Traefik ne regardent que le code.
- Contexte de requête : `context.Context` enrichi (auth, locale, transaction, logger, request id) accessible partout.
- Calcul de `server.url` / `server.absoluteUrl` et `admin.url` / `admin.absoluteUrl`.
- Arrêt propre sur SIGTERM et SIGINT : arrêt des nouvelles connexions, drainage avec délai, puis `Destroy` dans l'ordre inverse du démarrage (plugins, hooks utilisateur, serveur, event hub, base).

## Hors périmètre
- Le routage (`API-002`), les middlewares globaux (`API-008`).
- HTTP/3.

## Critères d'acceptation
- [ ] `curl -I /_health` renvoie `204`, et aucun en-tête ne nomme le serveur ni sa version.
- [ ] Derrière un proxy avec `server.proxy.koa: true`, l'IP client est celle de `X-Forwarded-For`.
- [ ] SIGTERM pendant une requête longue : la requête se termine, puis le processus sort avec le code 0.
- [ ] Démarrage à vide (projet minimal SQLite) en moins de 300 ms sur le homelab.

## Notes techniques
- `net/http` standard. Le routeur de Go 1.22 (méthode + motif) suffit ; chi seulement si un besoin apparaît.
- Pas de dépendance à un framework : les middlewares sont des `func(http.Handler) http.Handler`.

## Référence Strapi
- `core/core/src/services/server/index.ts`, `http-server.ts`, `koa.ts`
- `core/core/src/Strapi.ts` (listen, destroy), `core/core/src/configuration/urls.ts`

## Dépendances
- `SETUP-001`, `EXT-001`

## Fichiers
- `internal/httpapi/server.go`, `internal/httpapi/health.go`, `internal/httpapi/reqctx.go`, `grapi.go`
