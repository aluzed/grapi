---
status: done
---

# XTR-005 : Content releases (dont planifiées)

> Clean-room (`SETUP-003`, décision D6) : ce ticket se spécifie uniquement à partir de la documentation publique de Strapi et du comportement observable. Aucun fichier source Strapi sous licence Enterprise ne doit être ouvert, y compris pour en lire le nom.

## Contexte
Une release Strapi regroupe des entrées, de content types et de locales différents, pour les publier ou les dépublier ensemble. La documentation publique décrit : une action par entrée (publier ou dépublier), un état par entrée (déjà publiée, prête, pas prête), des statuts de release (Empty, Blocked, Ready, Done), une publication manuelle, une planification avec date, heure et fuseau horaire, et le webhook `releases.publish`. Une release publiée n'est plus modifiable. grapi n'impose aucune limite (nombre de releases en attente, d'entrées).

## Objectif
Publier ou dépublier un lot de documents de façon atomique, à la main ou à une date planifiée, avec validation préalable de chaque entrée.

## Périmètre
- **Release** : `name`, `scheduledAt` (optionnel), `timezone` (IANA, défaut : réglage `releases.defaultTimezone`), `status`, `releasedAt`, `createdBy`.
- **Action** : `type` (`publish` | `unpublish`), `uid`, `documentId`, `locale` (null si pas d'i18n). Une seule action par `(release, uid, documentId, locale)`. Seuls les content types avec Draft & Publish sont acceptés ; single types compris.
- **Ajout** : depuis la vue édition (« Ajouter à une release », choix de la release et de l'action) et depuis la vue liste en lot (`ADM-003`, `ADM-004`). Le document affiche les releases en attente qui le contiennent.
- **État d'une entrée**, recalculé à la lecture et via « Actualiser » :
  - `already-published` / `already-unpublished` ;
  - `ready` ;
  - `not-ready` : validation stricte de publication en échec (`DOC-003`), ou étape requise non atteinte (`XTR-004`), avec la liste des erreurs.
- **Statut de release** : `empty` (aucune action), `blocked` (au moins une entrée `not-ready`), `ready`, `done` (publiée), et `failed` (ajout grapi : échec d'une publication planifiée, avec le message d'erreur).
- **Publication** (`POST .../releases/:id/publish`) : refusée si `blocked` ; sinon toutes les actions s'exécutent dans **une seule transaction**, tout ou rien. Ensuite `releasedAt` est renseigné, `status: done`, la release devient en lecture seule. Événements `entry.publish` / `entry.unpublish` émis après commit pour chaque entrée.
- **Planification** : `scheduledAt` + `timezone` convertis en instant UTC. Un job cron unique par release (`EXT-005`), reprogrammé au démarrage pour les releases en attente ; une échéance passée pendant un arrêt s'exécute au démarrage suivant. À l'échéance, `blocked` passe en `failed` (aucune publication partielle). Modifier ou supprimer la release reprogramme ou annule le job.
- **Cycle de vie des entrées** : un document supprimé disparaît des releases en attente ; une locale supprimée aussi.
- **Webhook** `releases.publish` (`EXT-006`) : `{event, createdAt, isPublished, release: {id, name, releasedAt, scheduledAt, timezone, createdAt, updatedAt, actions: {count}}}`, émis après une publication réussie ou échouée (`isPublished: false`).
- **Audit** (`XTR-002`) : `release.*`, `release-action.*`, `release-settings.update`, avec `origin: scheduler` pour les publications planifiées et la locale de chaque entrée.
- **Permissions** : `admin::releases.read|create|update|delete|publish`, `admin::releases.actions.create|delete`, `admin::releases.settings.read|update`. Publier une release exige aussi `publish` sur chacun de ses content types et locales.
- **Écrans** : liste des releases (onglets En attente / Terminées, pleine largeur sur mobile), détail groupé par content type ou par locale, état de chaque entrée avec ses erreurs, réglage du fuseau par défaut. Conformes à `ADM-002`.

## Hors périmètre
- Releases récurrentes ou annulation d'une release `done` (rollback) : amélioration possible.
- Prévisualisation de l'état du site après release.

## Critères d'acceptation
- [x] Une release avec 2 publish et 1 unpublish sur deux content types et deux locales s'applique entièrement ; les 3 documents ont l'état attendu.
- [x] Une entrée avec un champ requis vide rend la release `blocked` ; la publication manuelle renvoie 400 avec la liste des entrées fautives.
- [x] Panne simulée sur la 3e action : aucune des actions n'est appliquée et la release n'est pas `done`.
- [x] Planification à 09:00 `Europe/Paris` le jour du passage à l'heure d'hiver : exécution à l'instant UTC correct.
- [x] Serveur arrêté pendant l'échéance : la release est publiée au redémarrage.
- [x] Release planifiée devenue `blocked` : passe en `failed` à l'échéance, webhook reçu avec `isPublished: false`.
- [x] Une release `done` refuse toute modification (409).
- [x] Ajouter deux fois le même document et la même locale à une release est refusé.
- [x] Un content type sans Draft & Publish n'apparaît pas dans « Ajouter à une release ».
- [x] Payload du webhook `releases.publish` conforme au périmètre (récepteur de test).

## Avancement
Fait : tables, API (`internal/adminapi/releases.go`), état par publication d'essai, publication tout ou rien, planificateur (démarrage, 30 s, réveil sur date passée), `failed`, webhook, audit (origine `scheduler`), retrait des documents supprimés (admin, API de contenu, Document Service) et des versions d'une locale supprimée, permissions, réglage du fuseau par défaut (`GET|PUT /releases/settings`, permissions `releases.settings.*`), écrans (liste, détail groupé par content type ou par locale, « Ajouter à une release » dans l'édition et en lot depuis la liste), atomicité testée par une panne injectée (`TestAdminReleaseIsAtomic`), permission `publish` vérifiée par locale à l'ajout et à la publication (`TestAdminReleaseLocalePermissions`), conversion de fuseau testée sur les deux changements d'heure (`admin/src/lib/tz.test.ts`), test de bout en bout `TestAdminReleases`.

Test de la release mixte : `TestAdminReleaseAcrossTypesAndLocales`. Amélioration possible : bouton « Actualiser » (aujourd'hui l'état se recalcule à chaque ouverture du détail).

## Notes techniques
- Tables `grapi_releases` et `grapi_release_actions` (index `(uid, document_id, locale)` pour afficher les releases d'un document).
- La publication réutilise `document.Publish` / `Unpublish` (`DOC-002`) dans la transaction de la release (`DB-001`, transaction portée par le `context`).
- Plusieurs instances : verrou applicatif en base sur la release (`UPDATE ... WHERE status IN ('ready','blocked') AND released_at IS NULL`) pour qu'une seule instance l'exécute.

## Référence
- Releases : https://docs.strapi.io/cms/features/releases
- Webhooks : https://docs.strapi.io/cms/backend-customization/webhooks
- Spécification clean-room : aucune source Strapi sous licence Enterprise consultée.

## Dépendances
- `DOC-002`, `DOC-003`, `DOC-005`, `DB-001`, `EXT-005`, `EXT-006`, `PLG-001`, `AUTH-001`, `ADM-003`, `ADM-004`, `XTR-002`, `XTR-004`, `SETUP-003`

## Fichiers
- `internal/extra/releases/` (`model.go`, `actions.go`, `validate.go`, `publish.go`, `scheduler.go`, `webhook.go`, `handlers.go`)
- `admin/src/pages/releases/`, `admin/src/pages/content-manager/releases/`
