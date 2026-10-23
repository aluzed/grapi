---
status: in-progress
---

# MEDIA-002 : Traitement d'image (miniature, breakpoints, optimisation, orientation, animés)

## Contexte
À l'upload d'une image, Strapi (avec sharp, donc libvips) génère une miniature et des formats responsive, optimise la qualité et corrige l'orientation EXIF, selon les réglages du Media Library. Le résultat est stocké dans `files.formats`.

## Objectif
Produire les mêmes formats, avec les mêmes noms, les mêmes dimensions et les mêmes règles de déclenchement.

## Périmètre
- Formats concernés :
  - redimensionnement : jpeg, png, webp, tiff, gif ;
  - traitement (métadonnées) : en plus svg et avif ;
  - optimisation : jpeg, png, webp, tiff, avif.
- **Miniature** `thumbnail` : 245 x 156, `fit: inside`. Elle n'est générée que si l'image dépasse l'une des deux dimensions.
- **Breakpoints** (si le réglage `responsiveDimensions` est actif) : par défaut `large: 1000`, `medium: 750`, `small: 500`. Ils sont configurables via `plugin::upload.breakpoints`. Un format n'est généré que si l'original est plus grand que le breakpoint.
- Chaque format contient : name (`<format>_<nom>`), hash (`<format>_<hash>`), ext, mime, path, width, height, size (Ko), sizeInBytes, url.
- **Optimisation** (si `sizeOptimization` est actif) : ré-encodage en qualité 80, sinon en qualité 100 si seule l'orientation est demandée. On garde l'original si le résultat est plus gros.
- **Orientation** (si `autoOrientation` est actif) : rotation selon l'EXIF.
- Images animées (gif, webp animé) : on conserve toutes les frames, et `height` vaut la hauteur d'une page, pas de la bande.
- `sharp.limitInputPixels` (`plugin::upload.sharp`) : limite anti-bombe de décompression.
- Les réglages sont lus dans le core store (`MEDIA-006`). Par défaut, `sizeOptimization: true`, `responsiveDimensions: true` et `autoOrientation: false`.

## Hors périmètre
- Recadrage et édition dans l'UI (`ADM-007`).
- Génération d'images à la demande (pas fournie par Strapi).

## Critères d'acceptation
- [x] Un JPEG de 2000 x 1500 donne `thumbnail` (208 x 156), `large` (1000 x 750), `medium` et `small`, avec les mêmes dimensions que Strapi 5.59 (banc `SETUP-002`, comparaison des dimensions et non des octets).
- [x] Une image de 400 x 300 ne génère que `thumbnail`.
- [x] Un GIF animé reste animé dans tous ses formats.
- [x] Une photo avec EXIF orientation 6 sort droite si `autoOrientation` est actif.
- [x] `limitInputPixels` rejette une image hors limite avec une erreur explicite (TU).
- [x] Le traitement d'une image de 20 Mpx tient en moins de 300 Mo de RSS (mesure sur le homelab).

- Mémoire (2026-10-23, homelab, PNG de 5000 x 4000, 20 Mpx, envoi par l'admin, 4 formats) : pic de RSS de 486 Mo au départ, 419 Mo en décodant l'image une seule fois pour l'optimisation et tous les formats, puis 185 Mo (envoi en 2,3 s au lieu de 4,6) en ramenant d'abord une grande image à deux fois la taille cible avec `ApproxBiLinear` : le CatmullRom de `x/image` garde un tampon flottant de la largeur cible par la hauteur source (128 Mo pour `large`). Repos : 28 Mo.
- GIF animé : chaque format garde toutes les images et leurs délais (`TestAnimatedGIFStaysAnimated` : 3 images en 600 x 400, vignette en 234 x 156).
- `autoOrientation` : l'orientation EXIF d'un JPEG (segment APP1, balise 0x0112) redresse l'image comme `rotate()` de sharp, pour les 8 orientations ; les dimensions enregistrées et les formats suivent l'image redressée (`TestAutoOrientation` : orientation 6, 32 x 16 devient 16 x 32, la moitié gauche passe en haut).
- `plugins.upload.config.sharp.limitInputPixels` (booléen ou entier >= 0, défaut de sharp 16383², valeur invalide refusée au démarrage avec le message de Strapi). Correction du critère : Strapi ne rejette pas l'image, ses appels sharp échouent et l'upload la range comme un fichier ordinaire, sans dimensions ni formats ; grapi fait de même (`TestUploadLimitInputPixels`).
- Banc `devfreelance_formats` (SQLite et Postgres), identique à Strapi : 2000 x 1500 donne `thumbnail` 208 x 156, `small` 500 x 375, `medium` 750 x 563, `large` 1000 x 750 ; 400 x 300 seulement `thumbnail` ; 150 x 150 aucun format ; 1000 x 3000 (portrait) `thumbnail` 52 x 156 à `large` 333 x 1000. Le runner génère des PNG : le calcul des dimensions ne dépend pas du format.

## Notes techniques
- Décision **D4** : interface `ImageProcessor`, avec une implémentation `govips` par défaut (libvips, la même bibliothèque que sharp, donc des résultats très proches) et une implémentation Go pure (`disintegration/imaging`) derrière le build tag `novips`, qui ne fait ni WebP ni AVIF en écriture.
- Traiter les formats en parallèle, avec un sémaphore global pour borner le CPU et la mémoire.
- Les arrondis de dimension de sharp (`fit: inside`) doivent être reproduits : prévoir un TU sur une table de cas.

## Référence Strapi
- `core/upload/server/src/services/image-manipulation.ts` (constantes `THUMBNAIL_RESIZE_OPTIONS`, `DEFAULT_BREAKPOINTS`, `FORMATS_TO_*`, `optimize`)
- `core/upload/server/src/services/upload.ts` (enchaînement)

## Dépendances
- `MEDIA-001`, `SETUP-004` (choix CGO)

## Fichiers
- `internal/media/image/{processor.go,vips.go,purego.go,formats.go,formats_test.go}`
