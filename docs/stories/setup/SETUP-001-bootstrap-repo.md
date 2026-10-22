# SETUP-001 : Bootstrap du repo Go

## Contexte
grapi démarre de zéro. Le dépôt local `~/Documents/dev/grapi` contient déjà `LICENSE` (MIT, aluzed), `docs/feasibility.md` et `docs/stories/`. Le dépôt GitHub `aluzed/grapi` est créé plus tard, sur demande explicite de l'utilisateur. Description retenue : « A Strapi-compatible headless CMS written in Go » (voir D5).

## Objectif
Poser un squelette compilable, linté et testé en CI, avec le layout cible du README des stories, pour que chaque ticket suivant n'ait plus qu'à remplir son package.

## Périmètre
- `go.mod` : module `github.com/aluzed/grapi`, version Go alignée sur la toolchain locale (1.26).
- Arborescence vide mais compilable : `cmd/grapi/main.go` (affiche la version), `grapi.go`, `schema/`, `document/`, `hook/`, `internal/{config,registry,db,httpapi,auth,media,plugins,jsrt,transfer}/`, `compat/`, `admin/` (placeholder).
- `Makefile` (ou `Taskfile.yml`) : `build`, `test`, `test-integration` (Postgres + SQLite), `lint`, `bench-compat` (délègue à `SETUP-002`).
- `golangci-lint` : config versionnée (`govet`, `staticcheck`, `errcheck`, `gofumpt`, `revive`, `gosec`).
- CI GitHub Actions :
  - lint et tests unitaires sur chaque push et chaque PR ;
  - tests d'intégration avec un service Postgres 16, plus SQLite ;
  - un job dédié qui vérifie l'absence d'em-dash (recherche du caractère U+2014 avec `grep -rnP '\x{2014}'`) et échoue s'il en trouve.
- Version injectée au build (`-ldflags "-X main.version=..."`).
- `README.md` en anglais : pitch, statut (pré-alpha), lien vers `docs/feasibility.md`, licence MIT et la mention « Not affiliated with or endorsed by Strapi Solutions SAS. Strapi is a trademark of Strapi Solutions SAS. »
- `CONTRIBUTING.md` court : règles clean-room (renvoi à `SETUP-003`), conventions de commit, pas de numéro de ticket dans le code.
- `.editorconfig`, `.gitignore` (binaires, `dist/`, `.env*` sauf `.env.example`, `public/uploads/`).

## Hors périmètre
- Création du dépôt distant et premier push (sur demande explicite).
- Toute logique métier.

## Critères d'acceptation
- [ ] `go build ./...` et `go test ./...` passent sur un clone propre.
- [ ] `make lint` passe et la CI est verte sur la branche `main`.
- [ ] `grapi version` affiche la version injectée.
- [ ] Le job de CI anti em-dash échoue si on ajoute volontairement le caractère U+2014 dans un fichier (testé une fois).
- [ ] Le README contient l'avertissement de marque et le lien vers l'étude de faisabilité.
- [ ] Aucun fichier sous licence autre que MIT dans le dépôt.

## Notes techniques
- Pas de répertoire `pkg/` : les packages publics (mode framework, `EXT-002`) sont à la racine du module (`grapi`, `schema`, `document`, `hook`), le reste est dans `internal/`.
- Dépendances limitées au strict nécessaire dès le départ. Chaque ajout passe le contrôle de licence de `SETUP-003`.
- Tests d'intégration derrière le build tag `integration`, pour que `go test ./...` reste rapide en local.

## Référence Strapi
- Aucune (ticket d'infrastructure). La structure monorepo de Strapi (`packages/core/*`) sert seulement de repère pour le découpage fonctionnel.

## Dépendances
- Aucune.

## Fichiers
- `go.mod`, `Makefile`, `.golangci.yml`, `.github/workflows/ci.yml`, `README.md`, `CONTRIBUTING.md`, `.editorconfig`, `.gitignore`
- `cmd/grapi/main.go`, `grapi.go`, `doc.go` des packages vides
