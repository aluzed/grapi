---
status: in-progress
---

# MEDIA-005 : Provider Cloudinary

## Contexte
`@strapi/provider-upload-cloudinary` envoie les fichiers à Cloudinary, qui gère lui-même les transformations, et stocke `public_id` et `resource_type` dans `provider_metadata`.

## Objectif
Un provider Cloudinary avec les mêmes options et les mêmes métadonnées.

## Périmètre
- Options : `cloud_name`, `api_key`, `api_secret`, et `actionOptions.upload|uploadStream|delete` transmises à l'API.
- Upload en streaming avec `resource_type: auto` et `public_id` = `hash`. Les vidéos sont supportées.
- `provider_metadata` vaut `{public_id, resource_type}`, et il est réutilisé pour le remplacement et la suppression.
- Suppression avec le bon `resource_type` (image, video ou raw). Un fichier déjà absent n'est pas une erreur.

## Hors périmètre
- Transformations Cloudinary à la volée dans l'API.

## Critères d'acceptation
- [ ] Upload, remplacement et suppression testés sur un compte Cloudinary de test, contre un sandbox ou un mock HTTP enregistré.
- [ ] **Compat DB (D3)** : un fichier envoyé par Strapi sur Cloudinary peut être supprimé depuis grapi grâce à son `provider_metadata`.

## Avancement
- `internal/media/provider/cloudinary.go` : upload signé (`auto/upload`, `public_id` = hash, `actionOptions.upload` signées avec), `secure_url` comme URL, `provider_metadata` = `{public_id, resource_type}` ; suppression par `<resource_type>/destroy` avec `invalidate`, un fichier absent (`not found`) n'est pas une erreur. Test sur un serveur `httptest` qui vérifie chemins, champs et signature.
- `provider_metadata` circule désormais dans l'upload : enregistré sur le fichier et sur chaque format, rendu au provider à la suppression (y compris les formats remplacés), ce qui permet de supprimer un fichier envoyé par Strapi.
- Reste : essai sur un compte Cloudinary de test, envoi en flux (`uploadStream`) pour les gros fichiers.

## Notes techniques
- `cloudinary/cloudinary-go/v2`.

## Référence Strapi
- `providers/upload-cloudinary/src/index.ts`

## Dépendances
- `MEDIA-003`

## Fichiers
- `internal/media/provider/cloudinary/{cloudinary.go,cloudinary_test.go}`
