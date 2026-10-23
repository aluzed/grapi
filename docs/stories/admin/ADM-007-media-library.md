---
status: in-progress
---

# ADM-007 : Media Library

> Option retenue : B (admin maison), voir `docs/decisions/D1-admin.md`. Conventions UI : voir `ADM-002`.

## Contexte
La Media Library de Strapi gère les fichiers et les dossiers. Elle permet l'upload multiple en glisser-déposer ou depuis une URL, l'édition des métadonnées (nom, texte alternatif, légende, point focal), le recadrage, le remplacement de fichier, le déplacement et la suppression groupés, la recherche, les filtres et le tri. Elle sert aussi de sélecteur pour les champs média de la vue édition.

## Objectif
Offrir la gestion complète des médias dans l'admin, à la fois comme page à part entière et comme modale de sélection réutilisable.

## Périmètre
- **Vue grille et vue liste** des fichiers et des dossiers, avec un fil d'Ariane des dossiers.
- **Recherche**, filtres (type : image, vidéo, audio, fichier ; date de création ; date de modification) et tri (nom, date).
- **Upload** :
  - glisser-déposer multiple, sélection de fichiers, upload depuis des URLs ;
  - barre de progression par fichier, annulation ;
  - rejet affiché si la taille limite est dépassée.
- **Détail d'un fichier** :
  - aperçu, nom, texte alternatif, légende ;
  - point focal (`focalPoint`) et recadrage d'image (crée un nouveau fichier ou remplace l'existant, au choix) ;
  - téléchargement, copie de l'URL, remplacement du fichier en gardant le même id ;
  - formats générés (`thumbnail`, `small`, `medium`, `large`) ;
  - documents qui utilisent le fichier.
- **Dossiers** : création, renommage, déplacement, suppression (avec la liste du contenu impacté dans la modale de confirmation).
- **Actions groupées** : déplacer, supprimer.
- **Modale de sélection** pour les champs média (`ADM-004`) : mêmes fonctions, avec le filtre `allowedTypes` du champ et une sélection simple ou multiple.
- **Réglages** (Settings > Media Library) : optimisation de taille, formats responsive, orientation automatique.
- **Permissions** : access, create (upload), read, update, download, copy link, et la condition `is-creator`.

## Avancement
- API (`docs/admin-api.md`, Médiathèque) : liste des fichiers par dossier avec recherche, type et tri, détail, édition des métadonnées et déplacement, suppression, upload par la chaîne de l'API de contenu (`AdminUpload`) dans un dossier, remplacement, dossiers (liste avec compteurs, création numérotée comme Strapi). Testé de bout en bout (`TestAdminMedia`).
- Interface : page Médiathèque (fil d'Ariane des dossiers, dossiers avec compteur, grille avec miniatures, recherche, filtre par type, pagination, upload multiple par sélection ou glisser-déposer, création de dossier, fiche fichier avec aperçu, taille, dimensions, copie de l'URL, téléchargement, édition du nom, du texte alternatif et de la légende, suppression confirmée) et modale de sélection réutilisée par les champs média (simple ou multiple, filtre `allowedTypes`). Vérifié dans Chrome sur le fixture `devfreelance`.
- Actions groupées (mode sélection : déplacer vers un dossier, supprimer avec confirmation), renommage et suppression de dossier avec son contenu (confirmation), remplacement d'un fichier depuis sa fiche (même id, même URL). Testé (`TestAdminMediaBulk`) et vérifié dans Chrome.
- Faits depuis : import depuis des URL, déplacement de dossier, page de réglages, tri et configuration d'affichage (voir `MEDIA-006`).
- Détail d'un fichier : type, date d'envoi, formats générés, et entrées qui l'utilisent (`GET /upload/files/{id}/related`, lu dans `files_related_mph`, filtré par les droits de lecture ; `TestAdminFileRelated`) avec un bouton « Ouvrir l'entrée ».
- Vue liste (table sur desktop, cartes sur mobile, choix mémorisé dans le navigateur) et filtres par date d'envoi (`from`, `to` sur `createdAt`, validés côté serveur).
- Progression et annulation des envois : barre de progression et bouton « Annuler l'envoi » (l'envoi passe par XMLHttpRequest, seul à suivre le corps d'une requête ; `admin/src/lib/upload.test.ts`).
- Un fichier utilisé dans un composant (même imbriqué) est rattaché à l'entrée qui porte le composant, via les tables de liaison des composants (`TestAdminFileRelatedThroughComponent`).
- Recadrage dans le navigateur (canvas), comme Strapi : zone tracée à la souris ou au doigt, puis « Recadrer l'original » (remplacement, formats régénérés) ou « Recadrer une copie » (nouveau fichier dans le même dossier) ; vérifié dans Chrome (copie de 450 × 300 pour une moitié d'image de 900 × 600). Calcul des pixels testé (`admin/src/lib/crop.test.ts`).
- Point focal : clic sur l'image du détail, repère affiché, bouton pour le retirer ; `focalPoint` `{x, y}` en pourcentage validé comme Strapi (0 à 100, `null` l'efface) (`TestAdminFocalPoint`), vérifié dans Chrome.

## Hors périmètre
- Le traitement d'image côté serveur (`MEDIA-002`) et les providers (`MEDIA-003` à `MEDIA-005`).
- Les métadonnées générées par IA (fonction EE chez Strapi, non prévue).

## Critères d'acceptation
- [ ] Uploader 20 images en une fois crée 20 fichiers, avec leurs formats générés selon les réglages.
- [ ] Remplacer un fichier conserve son id, et tous les documents qui l'utilisent pointent sur le nouveau contenu.
- [ ] Supprimer un dossier non vide passe par une modale qui annonce le nombre de fichiers et sous-dossiers impactés.
- [x] Le point focal enregistré est renvoyé par l'API (`focalPoint`).
- [ ] Le sélecteur depuis un champ `allowedTypes: ["images"]` ne propose que des images.
- [ ] Un utilisateur sans `update` voit les détails en lecture seule.
- [ ] À 375 px, la grille passe sur une ou deux colonnes et toutes les actions sont en pleine largeur.

## Notes techniques
- Upload en multipart vers l'API admin d'upload (`MEDIA-001`), en parallèle limité (3 fichiers à la fois).
- Le recadrage se fait côté client (canvas) puis part en upload ou en remplacement, comme dans Strapi.
- **Si l'option A est retenue** : reproduire les routes admin `/upload/*` (`files`, `folders`, `folder-structure`, `actions/bulk-*`, `settings`, `configuration`) selon `upload/shared/contracts/`.

## Référence Strapi
- `core/upload/server/src/routes/admin.ts`, `core/upload/server/src/controllers/`
- `core/upload/admin/src/`
- `core/upload/shared/contracts/`

## Dépendances
- `ADM-002`, `MEDIA-001`, `MEDIA-002`, `MEDIA-006`, `AUTH-001`

## Fichiers
- `admin/src/pages/media-library/`, `admin/src/features/media-picker/`
- `internal/media/admin_handlers.go`
