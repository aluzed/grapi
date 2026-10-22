---
status: in-progress
---

# MEDIA-003 : Interface provider et provider local

## Contexte
Strapi délègue le stockage à un provider (`config/plugins` : `upload.config.provider` et `providerOptions`). Le provider renseigne `file.url` et `file.provider_metadata`. Le provider local écrit dans `public/uploads`, servi en statique.

## Objectif
Définir l'interface provider de grapi et fournir le provider local.

## Périmètre
- Interface Go :
  ```go
  type Provider interface {
      Upload(ctx context.Context, f *File, r io.Reader) error // renseigne f.URL et f.ProviderMetadata
      Delete(ctx context.Context, f *File) error
  }
  // Optionnelles, détectées par assertion de type :
  type Replacer interface { Replace(ctx context.Context, f *File, r io.Reader) error }
  type SizeChecker interface { CheckFileSize(f *File, sizeLimit int64) error }
  type PrivateProvider interface { IsPrivate() bool; SignedURL(ctx context.Context, f *File) (string, error) }
  ```
- Ordre utilisé par le service : `Replace` s'il existe, sinon `Delete` puis `Upload`. `CheckFileSize` par défaut renvoie 413 au-delà de `sizeLimit`.
- Pour un provider privé, les URLs sont signées à la lecture, dans toutes les réponses : fichier, formats, populate.
- Enregistrement des providers par nom (`local`, `aws-s3`, `cloudinary`, ou un provider custom en mode framework, `EXT-002`).
- **Provider local** :
  - écrit dans `public/uploads/<hash><ext>`, avec `url` = `/uploads/<hash><ext>` ;
  - créé le dossier au boot s'il manque ;
  - suppression idempotente (un fichier déjà absent n'est pas une erreur) ;
  - le middleware `public` (`API-008`) sert `/uploads` avec les bons en-têtes de cache.
- Les options de `config/plugins` (`providerOptions`, `actionOptions.upload|delete`) sont transmises au provider.

## Hors périmètre
- S3 (`MEDIA-004`), Cloudinary (`MEDIA-005`).

## Critères d'acceptation
- [ ] **Compat DB (D3)** : les URLs `/uploads/...` d'une base Strapi existante sont servies depuis le même dossier, sans renommage.
- [ ] Un provider custom enregistré en Go est utilisé à la place du local (TU avec un provider factice).
- [ ] Le remplacement d'un fichier local garde le même `hash` et met à jour les formats (intégration).
- [x] Une tentative de traversée de chemin (`../`) dans un nom de fichier est neutralisée (TU).

## Avancement
- Fait : `internal/media/provider` (`Upload`, `Delete`, `IsPrivate`, `SignedURL`), provider local (écriture atomique, suppression idempotente), choix du provider par `plugins.upload.config.provider`, URL signées dans les réponses des routes upload (`isUrlSigned`), suppression limitée aux fichiers du provider configuré.
- Providers du projet en mode framework : `app.UseUploadProvider(nom, p)` (`providers.go`, `docs/framework.md`), choisis par `plugins.upload.config.provider` ; `provider_metadata` passe par `UploadFile.Metadata`.
- `SizeChecker` (`checkFileSize`) : un provider qui l'implémente remplace le contrôle de `sizeLimit` (`TestProviderCheckFileSize`).
- Reste : `Replacer`, signature des URL dans les populate du Content API.

## Notes techniques
- Écriture atomique : fichier temporaire dans le même dossier, puis `os.Rename`.
- Le volume `public/uploads` doit appartenir à l'utilisateur du conteneur (uid 1000, comme pour l'image dev-freelance actuelle).

## Référence Strapi
- `core/upload/server/src/services/provider.ts`, `core/upload/server/src/register.ts`
- `providers/upload-local/src/index.ts`

## Dépendances
- `EXT-001`, `API-008`

## Fichiers
- `internal/media/provider/{provider.go,registry.go}`, `internal/media/provider/local/{local.go,local_test.go}`
