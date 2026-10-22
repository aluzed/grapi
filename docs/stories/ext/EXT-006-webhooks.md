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
- [ ] Payload et en-têtes identiques à Strapi pour `entry.publish` (banc de compatibilité avec un récepteur d'échos).
- [ ] Un webhook désactivé ne reçoit rien.
- [ ] Récepteur hors ligne : par défaut, essais espacés puis abandon après la limite, et la file survit à un redémarrage ; avec `retries: 0`, un seul essai et une erreur journalisée.
- [ ] L'en-tête `X-Strapi-Event` est conservé tel quel (compatibilité des récepteurs existants).

## Notes techniques
- Client HTTP dédié, avec délais d'attente et sans suivi de redirection vers des IP privées si `server.webhooks.blockPrivateIPs` est actif (protection SSRF, en option).

## Référence Strapi
- `core/core/src/services/webhook-store.ts`, `webhook-runner.ts`, `worker-queue.ts`
- `core/admin/server/src/routes/webhooks.ts`

## Dépendances
- `DOC-005`, `DB-001`

## Fichiers
- `internal/webhook/store.go`, `internal/webhook/runner.go`, `internal/webhook/queue.go`, `internal/webhook/admin_api.go`
