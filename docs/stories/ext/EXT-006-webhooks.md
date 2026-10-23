---
status: in-progress
---

# EXT-006 : Webhooks

## Contexte
Les webhooks notifient des services externes (rebuild de site statique, n8n, etc.) après les écritures. Strapi les stocke en base et les déclenche depuis l'event hub.

## Objectif
Le même modèle de webhooks, le même payload et les mêmes en-têtes, avec une fiabilité améliorée de façon optionnelle.

## Périmètre
- Table `strapi_webhooks` : `id`, `name`, `url`, `headers` (json), `events` (json), `enabled`, et `contentTypeEvents` (json, filtrage par uid de content type).
- Événements autorisés : `entry.create|update|delete|publish|unpublish|draft-discard`, ainsi que ceux ajoutés par les plugins (`media.create|update|delete`, review workflows, releases).
- Livraison : POST JSON `{event, createdAt, ...info}` (info = `{model, uid, entry}` de `DOC-005`). En-têtes = `server.webhooks.defaultHeaders`, puis ceux du webhook, puis `X-Strapi-Event: <event>` et `Content-Type: application/json`. Délai d'attente de 10 s. File en mémoire, 5 envois en parallèle.
- Strapi ne fait **aucune nouvelle tentative** : l'échec est seulement journalisé.
- grapi fait **de nouvelles tentatives par défaut** (décision du 2026-10-22) : backoff exponentiel, file persistée en base pour survivre à un redémarrage, limite configurable via `server.webhooks.retries`. `retries: 0` redonne le comportement de Strapi.
- Déclenchement manuel (`/:id/trigger`) pour l'admin, avec le statut et le message renvoyés.
- API admin : CRUD, suppression en lot, déclenchement manuel.

## Hors périmètre
- L'écran admin (`ADM-008`).
- La signature HMAC des payloads : amélioration possible, à proposer séparément.

## Critères d'acceptation
- [x] Payload et en-têtes identiques à Strapi pour `entry.publish` (banc de compatibilité avec un récepteur d'échos).
- [x] Un webhook désactivé ne reçoit rien.
- [ ] Récepteur hors ligne : par défaut, essais espacés puis abandon après la limite, et la file survit à un redémarrage ; avec `retries: 0`, un seul essai et une erreur journalisée.
- [x] L'en-tête `X-Strapi-Event` est conservé tel quel (compatibilité des récepteurs existants).

## Avancement
- Fait : `internal/webhooks` : webhooks lus dans `strapi_webhooks` à chaque événement, payload `{event, createdAt, model, uid, entry}`, en-têtes `defaultHeaders` puis webhook puis `X-Strapi-Event` et `Content-Type`, délai de 10 s, 5 envois en parallèle, nouvelles tentatives avec backoff exponentiel (`server.webhooks.retries`, 3 par défaut, 0 pour le comportement de Strapi).
- Écart avec le ticket : la file n'est pas persistée en base (une table de plus modifierait le schéma d'une base Strapi). `contentTypeEvents` n'existe pas dans Strapi 5.59.0 publié (il est sur la branche principale après le tag).
- Protection SSRF optionnelle : `server.webhooks.blockPrivateNetworks: true` refuse les livraisons vers les adresses de loopback, privées et link-local, contrôlées à la connexion (après DNS), `TestPublicOnlyTransport`.
- Reste : API admin (CRUD, trigger), comparaison avec un récepteur d'échos sur le banc.

- Banc : livraisons identiques à Strapi (événements, en-têtes, corps) pour création, publication, mise à jour et suppression, fixtures `basic` et `relations` (voir `DOC-005`). En production, comme Strapi, une URL de webhook qui pointe vers une adresse locale ou privée est refusée à la création et à la modification (« Url is not supported because it isn't reachable over the public internet ») ; le banc relance donc la référence Strapi en développement pour son récepteur d'échos.
