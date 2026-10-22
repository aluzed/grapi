---
status: todo
---

# MEDIA-006 : Dossiers, actions groupées, remplacement, upload depuis URL, réglages

## Contexte
Le Media Library de Strapi organise les fichiers en dossiers (chemin matérialisé), offre des actions groupées et stocke ses réglages et sa configuration d'affichage dans le core store.

## Objectif
Les endpoints admin du Media Library, au-delà de l'upload de base (`MEDIA-001`).

## Périmètre
- Dossiers :
  - CRUD `/upload/folders` et `GET /upload/folder-structure` (arbre) ;
  - `pathId` est un compteur unique et `path` vaut `/<pathId parent>/.../<pathId>` ;
  - un fichier porte `folderPath`, recalculé quand on le déplace.
- Actions groupées :
  - `POST /upload/actions/bulk-delete` (fichiers et dossiers récursivement) ;
  - `POST /upload/actions/bulk-move` (avec recalcul des chemins, et refus de déplacer un dossier dans lui-même ou dans un descendant) ;
  - `POST /upload/actions/bulk-update` (métadonnées).
- Remplacement : `POST /upload/files/:id/replace` (même `id`, formats régénérés, anciens fichiers supprimés chez le provider).
- Upload depuis des URLs : `POST /upload/actions/upload-from-urls`, avec un téléchargement côté serveur limité en taille et en durée, une protection SSRF (refus des IP privées, link-local et loopback, sauf réglage explicite) et une vérification du type MIME.
- Réglages : `GET|PUT /upload/settings` (`sizeOptimization`, `responsiveDimensions`, `autoOrientation`), clé de core store `plugin_upload_settings`.
- Configuration de vue : `GET|PUT /upload/configuration` (pageSize, sort), clé `plugin_upload_view_configuration`.
- Permissions RBAC du Media Library : `plugin::upload.read`, `assets.create`, `assets.update`, `assets.download`, `assets.copy-link`, `configure-view-settings`, `settings.read` (`AUTH-001`).

## Hors périmètre
- Métadonnées générées par IA (fonction payante liée au cloud Strapi). On pourra brancher le générateur LLM de grapi plus tard, hors parité.

## Critères d'acceptation
- [ ] Déplacer un dossier qui contient 3 niveaux recalcule `path` et `folderPath` partout (intégration).
- [ ] Déplacer un dossier dans son propre descendant est refusé (TU).
- [ ] `upload-from-urls` vers `http://169.254.169.254/` est refusé (TU anti-SSRF).
- [ ] Les réglages modifiés s'appliquent immédiatement aux uploads suivants.
- [ ] **Compat DB (D3)** : l'arborescence de dossiers d'une base Strapi s'affiche à l'identique.

## Notes techniques
- Les déplacements et suppressions récursives se font dans une transaction unique (`DB-001`).
- Contrôle SSRF dans le `DialContext` du client HTTP (résolution puis vérification de l'IP), pas seulement sur le nom d'hôte.

## Référence Strapi
- `core/upload/server/src/routes/admin.ts`
- `core/upload/server/src/services/{folder,file,upload}.ts`
- `core/upload/server/src/controllers/admin-folder*.ts`, `.../admin-file*.ts`, `.../admin-upload.ts`, `.../admin-settings.ts`

## Dépendances
- `MEDIA-001`, `MEDIA-002`, `DB-010`, `AUTH-001`

## Fichiers
- `internal/media/{folder.go,bulk.go,fromurl.go,settings.go}`
