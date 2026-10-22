---
status: todo
---

# MEDIA-001 : Modèle File/Folder et API d'upload (Content API + admin)

## Contexte
Le plugin upload de Strapi gère les fichiers (`plugin::upload.file`, table `files`) et les dossiers (`plugin::upload.folder`). Un champ `media` d'un content type est une relation polymorphe vers `files` (`files_related_mph`, `DB-008`). Les fronts consomment l'objet fichier tel quel (`url`, `formats.small.url`...), donc sa forme JSON doit être identique.

## Objectif
Le modèle de fichier, la réception multipart et les routes d'upload du Content API, ainsi que le socle des routes admin.

## Périmètre
- Modèle `files` : name, alternativeText, caption, focalPoint (json), width, height, formats (json), hash, ext, mime, size (Ko, décimal à 2 chiffres), url, previewUrl, provider, provider_metadata (json), related (morph), folder, folderPath.
- Modèle `upload_folders` : name, pathId (entier unique), path (chemin matérialisé `/1/4`), parent, children, files.
- Nom de stockage `hash` = `<nom slugifié avec séparateur _, casse conservée>_<10 hex aléatoires>`. `ext` est conservé.
- Content API (préfixe `/api`) :
  - `POST /upload` (multipart `files`, et en option `fileInfo` en JSON avec name, alternativeText, caption, folder) ;
  - `POST /upload?id=` pour remplacer ;
  - `GET /upload/files`, `GET /upload/files/:id`, `DELETE /upload/files/:id`.
  - Les permissions viennent de `AUTH-004` (`plugin::upload.content-api.*`).
- Upload depuis l'API d'un content type : `POST /api/<type>` en multipart avec `files.<champ>` rattache directement les fichiers à l'entrée créée.
- Admin : `POST /upload`, CRUD `/upload/files` (avec filtres, recherche et pagination), `GET /upload/files/:id`. Le reste est dans `MEDIA-006`.
- `sizeLimit` : 1 Go par défaut (`plugin::upload.sizeLimit`). Au-delà, `PayloadTooLargeError` (413). Il est vérifié aussi par `checkFileSize` du provider (`MEDIA-003`).
- Événements `media.create`, `media.update` et `media.delete` sur l'event hub, pour les webhooks (`EXT-006`).
- Sanitize de sortie : `provider_metadata` reste visible, comme chez Strapi, et `related` n'est jamais exposé par le Content API.
- Fichiers temporaires dans un répertoire de travail par requête, supprimé en fin de requête même en cas d'erreur.

## Hors périmètre
- Images (`MEDIA-002`), providers distants (`MEDIA-004`, `MEDIA-005`), dossiers et actions groupées (`MEDIA-006`), UI (`ADM-007`).

## Critères d'acceptation
- [ ] Banc `SETUP-002` : `POST /api/upload` d'un JPEG renvoie un tableau dont chaque objet a les mêmes clés et types que Strapi 5.59. Seuls `hash`, les dates et les id diffèrent (normalisés par le banc).
- [ ] Un populate d'un champ media simple renvoie un objet, et un champ media multiple renvoie un tableau ordonné (banc).
- [ ] **Compat DB (D3)** : les 4 536 fichiers de la copie de base dev-freelance sont servis avec la même URL.
- [ ] Un upload au-delà de `sizeLimit` renvoie 413 avec le format d'erreur `API-007`.
- [ ] Aucun fichier temporaire ne reste après une erreur (TU).

## Notes techniques
- Streamer le multipart (`mime/multipart.Reader`) vers un fichier temporaire, sans tout charger en mémoire.
- MIME : détection par contenu (`net/http.DetectContentType` étendu, ou `gabriel-vasile/mimetype`), avec repli sur l'extension, comme formidable avec `hashAlgorithm`/`mimetype`.
- `nameToSlug` : reproduire le slugify de Strapi (séparateur `_`, casse conservée). Prévoir un TU avec des noms accentués.

## Référence Strapi
- `core/upload/server/src/content-types/{file,folder}.ts`
- `core/upload/server/src/routes/{content-api,admin}.ts`
- `core/upload/server/src/services/upload.ts`, `core/upload/server/src/controllers/content-api.ts`
- `core/upload/server/src/services/image-manipulation.ts` (`generateFileName`)
- `core/upload/server/src/config.ts`

## Dépendances
- `DB-008`, `API-006`, `API-008` (body multipart), `AUTH-004`, `MEDIA-003`

## Fichiers
- `internal/media/{model.go,service.go,routes_contentapi.go,routes_admin.go,multipart.go,naming.go}`
