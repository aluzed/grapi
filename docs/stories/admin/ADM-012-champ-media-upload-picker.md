---
status: done
---

# ADM-012 : Champ média abouti (téléversement, sélecteur, édition)

> Option retenue : B (admin maison), voir `docs/decisions/D1-admin.md`. Conventions UI : voir `ADM-002`.

## Contexte
La médiathèque (`ADM-007`) est complète, mais dans la vue édition le champ média se limite à des vignettes, des flèches et un sélecteur qui ne permet que de choisir un fichier déjà présent. Chez Strapi, le champ est un carrousel d'actions, et sa modale permet de parcourir la médiathèque **ou** d'ajouter de nouveaux fichiers (depuis l'ordinateur ou une URL) sans quitter le document.

## Objectif
Gérer les médias d'un document entièrement depuis la vue édition, comme dans Strapi.

## Périmètre
- **Champ** :
  - glisser-déposer de fichiers directement sur le champ : téléversement puis ajout au champ ;
  - aperçu en carrousel (image, vidéo, audio, PDF ; icône par type sinon), indicateur « 2 / 5 » pour un champ multiple ;
  - actions sur le fichier courant : « Ajouter », « Modifier » (détails), « Copier le lien », « Retirer » (confirmation) ;
  - réordonner par glisser-déposer en plus des flèches (clavier conservé) ;
  - `allowedTypes`, `multiple` et `required` respectés, avec un message clair si un fichier refusé est déposé.
- **Modale** avec deux onglets, comme Strapi :
  - « Parcourir » : la médiathèque en grille ou en liste, dossiers, recherche, filtres, tri, pagination, sélection multiple pour un champ multiple ;
  - « Ajouter des fichiers » : depuis l'ordinateur (sélecteur et glisser-déposer) ou depuis des URL, file d'attente avec progression, annulation et erreur par fichier, choix du dossier de destination, puis sélection automatique des fichiers ajoutés.
- **Détails depuis le champ** : nom, texte alternatif, légende, point focal, recadrage et remplacement du fichier, dans la même modale que la médiathèque (`ADM-007`), sans quitter le document.
- **États** : lecture seule si le champ n'est pas modifiable ; permissions de la médiathèque respectées (pas d'onglet « Ajouter » sans `plugin::upload.assets.create`).

## Hors périmètre
- Les providers de stockage (`MEDIA-003` à `MEDIA-005`).

## Critères d'acceptation
- [x] Déposer 3 images sur un champ multiple les téléverse et les ajoute au champ dans l'ordre de dépôt ; sur un champ simple, le dépôt de plusieurs fichiers est refusé avec un message.
- [x] Depuis l'onglet « Ajouter des fichiers », un fichier téléversé dans un dossier choisi apparaît dans ce dossier et est sélectionné à la fermeture.
- [x] Un fichier d'un type hors `allowedTypes` est refusé au dépôt comme dans la modale.
- [x] Modifier le texte alternatif depuis le champ met à jour le fichier (vérifié par l'API) sans perdre les modifications en cours du document.
- [x] Réordonner à la souris et au clavier donne le même ordre enregistré.
- [x] Un utilisateur sans droit de création ne voit pas l'onglet « Ajouter ».
- [x] À 375 px : carrousel en pleine largeur, actions empilées en pleine largeur, modale en plein écran.

## Référence Strapi
- `core/upload/admin/src/components/MediaLibraryInput/`, `MediaLibraryDialog/`, `UploadAssetDialog/`, `EditAssetDialog/` (code MIT).

## Dépendances
- `ADM-004`, `ADM-007`, `MEDIA-001`

## Fichiers
- `admin/src/pages/content-manager/MediaField.tsx`, `admin/src/pages/media/`
