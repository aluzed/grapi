---
status: todo
---

# CLI-005 : Transfer distant (WebSocket push/pull)

## Contexte
`strapi transfer --from <url> | --to <url>` synchronise deux instances en direct, via une WebSocket admin (`/admin/transfer/runner/push|pull`) authentifiée par transfer token (`AUTH-009`). Les mêmes étapes que l'export sont streamées par lots.

## Objectif
Le transfert distant entre deux instances grapi, et si possible entre Strapi et grapi dans les deux sens.

## Périmètre
- Côté serveur : endpoints WebSocket `push` et `pull`, montés seulement si `server.transfer.remote.enabled`.
- Protocole :
  - messages `init {transfer: push|pull, strategy, options}`, puis `status` et `end` ;
  - pour chaque étape, des lots `stream` avec accusés de réception ;
  - assets en base64, avec checksums.
- CLI : `grapi transfer --from <url> --from-token <t>` ou `--to <url> --to-token <t>`, avec les mêmes filtres que `CLI-004` (`--only`, `--exclude`, `--throttle`, `--force`).
- Le transfert entrant remplace les données, mais pas les admins (même règle qu'à l'import).
- Reprise impossible en cas de coupure : on recommence de zéro (comme Strapi), avec un message explicite.

## Hors périmètre
- Synchronisation incrémentale (Strapi ne la fait pas non plus).

## Critères d'acceptation
- [ ] Un transfert Strapi 5.59 vers grapi d'une copie de dev-freelance aboutit, et les réponses de l'API sont identiques (banc `SETUP-002`). C'est l'objectif de compat, à confirmer pendant l'implémentation, car le protocole n'est pas documenté publiquement.
- [ ] Un transfert grapi vers grapi aboutit (intégration).
- [ ] Un token sans la permission requise est refusé à l'init.
- [ ] Le transfert est refusé si `remote.enabled` est faux sur la cible.

## Notes techniques
- `coder/websocket` (ex-nhooyr). Le protocole se spécifie à partir de `remote/handlers/{push,pull}.ts` (MIT).
- Il passe par TLS dès qu'il traverse un réseau public : refuser `ws://` hors localhost, sauf `--insecure`.

## Référence Strapi
- `core/data-transfer/src/strapi/remote/handlers/{push,pull,abstract,utils}.ts`
- `core/data-transfer/src/strapi/providers/remote-*`
- `core/admin/server/src/routes/transfer.ts`
- `core/strapi/src/cli/commands/transfer/`

## Dépendances
- `CLI-004`, `AUTH-009`

## Fichiers
- `internal/transfer/remote/{server.go,client.go,protocol.go}`, `internal/cli/transfer.go`
