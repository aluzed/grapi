---
status: in-progress
---

# SETUP-004 : Packaging : binaire, image Docker, admin embarqué, choix CGO

## Contexte
Un des intérêts de grapi face à Strapi est le déploiement : un binaire unique au lieu de `node_modules` et d'un build admin Node. La limite vient du traitement d'image : libvips (D4) impose CGO.

## Objectif
Produire les artefacts de release : binaires multi-plateformes, image Docker minimale avec libvips et SPA admin embarquée, dans un processus reproductible.

## Périmètre
- **Deux variantes de build** :
  - `vips` (par défaut) : CGO activé, liée à libvips 8.15 ou plus, pour l'image Docker et Linux amd64/arm64 ;
  - `purego` (build tag `purego`) : `CGO_ENABLED=0`, binaire statique, traitement d'image en Go pur (JPEG, PNG et GIF uniquement, sans encodage WebP ni AVIF), pour macOS, Windows et les déploiements simples.
- **SQLite** via `modernc.org/sqlite` (sans CGO) dans les deux variantes.
- **Admin embarqué** : la SPA est construite (`admin/dist`) puis embarquée via `//go:embed`. La CI construit l'admin avec Node ou Bun, jamais au runtime. En développement, le flag `--admin-dev-proxy` permet de servir un serveur Vite local.
- **Image Docker** (`Dockerfile`) :
  - multi-stage : build de l'admin, build Go, runtime `debian:trixie-slim` + `libvips42` ;
  - utilisateur non root (uid 1000) ;
  - `EXPOSE 1337`, `HEALTHCHECK` sur `/_health` ;
  - volumes documentés : `/app/public/uploads`, `/app/data` (SQLite) ;
  - taille cible inférieure à 150 Mo.
- **Releases** : `goreleaser` (linux, darwin, windows ; amd64 et arm64), checksums, SBOM (`syft`), image poussée sur `ghcr.io/aluzed/grapi`.
- **Doc** : `DEPLOY.md` (Docker, binaire et systemd, Dokploy, variables d'env minimales).

## Hors périmètre
- Le déploiement d'une instance publique.
- Le chart Helm.

## Critères d'acceptation
- [ ] `make docker` construit l'image **sur le homelab** (`docker --context homelab`) et elle démarre un projet fixture sur Postgres.
- [x] La variante `purego` compile avec `CGO_ENABLED=0` et le binaire Linux est statique (`file` : « statically linked »).
- [ ] La même requête d'upload d'image produit des formats `thumbnail`, `small`, `medium` et `large` dans la variante vips, et un message clair dans la variante purego si un format non supporté est demandé.
- [ ] L'admin est servi par le binaire seul, sans aucun fichier externe.
- [x] L'image tourne en uid 1000 et `HEALTHCHECK` est vert.
- [x] `DEPLOY.md` décrit les trois modes de déploiement.

## Avancement
- Fait : `Dockerfile` (variante Go pur, `debian:trixie-slim`, uid 1000, `HEALTHCHECK` via `grapi health`, 149 Mo), `make docker` qui construit sur le homelab, `DEPLOY.md`. Testé sur le homelab : base Postgres neuve, fixture dev-freelance, healthcheck vert, inscription par l'API.
- Reste : variante libvips (govips, CGO), SPA admin embarquée, goreleaser, SBOM, image sur ghcr.io.

## Notes techniques
- La compatibilité de sortie avec sharp n'est pas binaire (encodeurs différents). Le banc (`SETUP-002`) compare donc les dimensions, les formats et les métadonnées, pas les octets.
- Garder `ImageProcessor` derrière une interface (`MEDIA-002`) pour que le choix vips ou purego soit un détail de build.
- Ne jamais builder l'image sur le Mac de dev : la cible `make docker` force le contexte homelab.

## Référence Strapi
- `packages/core/admin/server/src/routes/serve-admin-panel.ts` : service des fichiers statiques de l'admin.
- `packages/core/upload/server/src/services/image-manipulation.ts` : dépendance à sharp et formats attendus.

## Dépendances
- `SETUP-001`, `SETUP-003` (NOTICE), `MEDIA-002` (interface image), `ADM-001` (forme de l'admin).

## Fichiers
- `Dockerfile`, `.goreleaser.yaml`, `DEPLOY.md`, `admin/embed.go`, `internal/media/image/{vips.go,purego.go}` (build tags), `Makefile` (cibles `docker`, `release`)
