---
status: in-progress
---

# MEDIA-004 : Provider S3 (R2, MinIO, privé, URLs signées)

## Contexte
`@strapi/provider-upload-aws-s3` est le provider distant le plus utilisé. Il fonctionne avec tout stockage compatible S3 (Cloudflare R2, MinIO, Scaleway, OVH) grâce à `endpoint`.

## Objectif
Un provider S3 avec les mêmes options de configuration, pour qu'une config Strapi existante se transpose sans effort (`MIG-001`).

## Périmètre
- Options :
  - `s3Options` : credentials, region, endpoint, forcePathStyle ;
  - `params` : Bucket, ACL, `signedUrlExpires` (15 min par défaut) ;
  - `baseUrl` (CDN) et `rootPath`.
- `providerConfig` : checksumAlgorithm, preventOverwrite (`If-None-Match`), storageClass, encryption (SSE-S3 ou KMS), tags, multipart (partSize, queueSize).
- Upload en streaming, en multipart au-delà du seuil.
- `IsPrivate()` est vrai si l'ACL vaut `private`. Dans ce cas, `SignedURL` utilise le presigner et l'expiration `signedUrlExpires`.
- URL publique : `baseUrl/rootPath/<hash><ext>` si `baseUrl` est défini, sinon l'URL de l'objet.
- Suppression par clé.

## Hors périmètre
- Migration des fichiers local vers S3 (un outil de CLI pourra venir plus tard).

## Critères d'acceptation
- [x] Upload, remplacement et suppression testés contre MinIO sur le homelab.
- [x] Bucket privé : les URLs renvoyées par l'API sont signées et expirent (intégration).
- [x] Avec `baseUrl`, les URLs pointent sur le CDN, formats compris.
- [x] Un upload de 200 Mo passe en multipart sans dépasser 100 Mo de RSS.

## Avancement
- Fait : provider `aws-s3` sans SDK (SigV4 maison, vérifiée sur le vecteur officiel d'AWS) : clé `rootPath/path/hash+ext`, ACL `public-read` par défaut (absente si `ACL` est vidée, `private` donne des URL signées, 15 minutes par défaut), `endpoint` et `forcePathStyle` pour les stockages compatibles, `baseUrl` pour un CDN, options `s3Options` et anciennes options à la racine. Test d'intégration (`s3_integration_test.go`) passé le 2026-10-22 contre RustFS sur le homelab (MinIO n'est plus distribué en image) : upload, lecture signée de l'original et de la miniature, suppression.
- Envoi multipart au-delà de 64 Mio : morceaux de 16 Mio lus un à un depuis le fichier temporaire, envoi annulé si un morceau échoue, réponse d'erreur de `CompleteMultipartUpload` en 200 détectée (`TestS3Multipart` ; l'envoi de 200 Mo du test d'intégration passe par là et se relit en entier, sans hausse du pic de RSS). Un objet n'est plus limité aux 5 Go d'un `PutObject`.
- Reste : options avancées du provider (checksum, classe de stockage, chiffrement, tags).

- `make test-integration-s3` (`scripts/with-homelab-s3.sh`) : un stockage S3 jetable sur le homelab, SeaweedFS, car MinIO ne publie plus ses images communautaires (Docker Hub et Quay refusés). Tests : envoi, GET signé, vignette et suppression (`TestUploadToS3`) ; remplacement qui garde le `hash` et sert le nouvel objet, URL signée refusée après `signedUrlExpires`, nouvelle URL signée à la relecture, envoi de 200 Mo sans hausse notable du pic de RSS (`TestS3ReplaceExpiryAndLargeUpload`) ; bucket public avec `baseUrl` : fichier et formats sur le CDN (`TestS3BaseURL`).
- Le test a trouvé une vraie fuite : `probeSVG` lisait tout fichier non-image en entier pour y chercher `<svg` (191 Mo de RSS pour 200 Mo envoyés, tous providers confondus). La recherche se limite maintenant au premier Mio.

## Notes techniques
- `aws-sdk-go-v2` : `service/s3`, `feature/s3/manager` (multipart) et `s3.NewPresignClient`.
- Les credentials peuvent aussi venir de la chaîne standard AWS (env, IAM role) si `s3Options.credentials` est absent.

## Référence Strapi
- `providers/upload-aws-s3/src/index.ts`, `providers/upload-aws-s3/src/utils.ts`

## Dépendances
- `MEDIA-003`

## Fichiers
- `internal/media/provider/s3/{s3.go,config.go,s3_test.go}`
