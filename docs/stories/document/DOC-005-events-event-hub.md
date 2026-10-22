---
status: todo
---

# DOC-005 : Événements entry.* et event hub

## Contexte
Strapi émet des événements après chaque écriture de document. Ils alimentent les webhooks (`EXT-006`), l'audit (`XTR-002`) et les plugins.

## Objectif
Un event hub interne et l'émission des événements de documents avec le même contenu que Strapi.

## Périmètre
- Event hub : `Emit` (asynchrone), `On`, `Off`, `Once`, `Subscribe` (abonné à tous les événements), `Destroy`.
- Événements : `entry.create`, `entry.update`, `entry.delete`, `entry.publish`, `entry.unpublish`, `entry.draft-discard`. Les plugins en ajoutent (`media.create|update|delete`, etc.).
- Émis **après commit** de la transaction, avec `{model, uid, entry}` où `entry` est la version assainie (champs privés retirés) et peuplée.
- `server.webhooks.populateRelations` contrôle le peuplement des relations dans l'événement.

## Hors périmètre
- La livraison HTTP des webhooks (`EXT-006`).

## Critères d'acceptation
- [ ] Aucun événement émis si la transaction est annulée.
- [ ] Le payload de `entry.publish` correspond à Strapi (champs, relations peuplées ou non selon la config) sur le banc de compatibilité.
- [ ] Un abonné qui panique ne bloque ni l'écriture ni les autres abonnés (erreur journalisée).

## Notes techniques
- Les hooks `onCommit` de la transaction (`DB-001`) déclenchent l'émission.

## Référence Strapi
- `core/core/src/services/document-service/events.ts`
- `core/core/src/services/event-hub.ts`

## Dépendances
- `DOC-001`, `DB-001`

## Fichiers
- `internal/event/hub.go`, `document/events.go`
