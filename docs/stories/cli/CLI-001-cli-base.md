---
status: todo
---

# CLI-001 : CLI de base : `new`, `develop`, `start`, `version`

## Contexte
Strapi est piloté par `create-strapi-app` (scaffold) et par la CLI `strapi` (`develop`, `start`, `build`). Chez grapi, un seul binaire `grapi` regroupe tout. En mode framework (`EXT-002`), le binaire du projet expose les mêmes sous-commandes.

## Objectif
Le squelette de la CLI et les commandes de cycle de vie d'un projet.

## Périmètre
- Binaire `grapi` (cobra), flags globaux : `--dir` (racine du projet, `.` par défaut), `--env` (équivalent de `NODE_ENV`, alias `GRAPI_ENV`) et `--log-level`.
- `grapi new <nom>` : scaffold d'un projet, à partir de templates embarqués :
  - `config/` (server, database, admin, api, middlewares, plugins) ;
  - `src/api/`, `src/components/` et `public/uploads/` ;
  - `.env` généré, avec des secrets aléatoires : `APP_KEYS`, `API_TOKEN_SALT`, `ADMIN_JWT_SECRET`, `TRANSFER_TOKEN_SALT`, `JWT_SECRET`, `ENCRYPTION_KEY` ;
  - `.env.example` avec les mêmes clés, sans valeurs ;
  - options `--db postgres|sqlite` (sqlite par défaut), `--db-host`, `--db-port`, `--db-name`, `--db-user`, `--db-password`, `--db-ssl`, `--git-init` et `--framework` (génère un `main.go` pour le mode framework).
- `grapi develop` :
  - surveille `src/` et `config/` (fsnotify) ;
  - recharge le schéma et la config **en processus** quand c'est possible (schémas JSON, routes déclaratives) ;
  - sinon, redémarre le processus (code Go en mode framework, via `go run`) ;
  - active les écritures du Content-Type Builder (`ADM-006`) et ouvre l'admin si `--open`.
- `grapi start` : production, pas de watch, CTB en lecture seule.
- `grapi version`, `grapi report` (versions, OS, base, plugins activés, sans secrets).
- Codes de sortie : 0 en succès, 1 en erreur de config, avec un message lisible.

## Hors périmètre
- Générateurs (`CLI-002`), commandes d'inspection et d'admin (`CLI-003`), export et transfer (`CLI-004`, `CLI-005`).
- `build` : rien à builder côté serveur. L'admin est embarqué (`SETUP-004`).
- `console` (REPL JS de Strapi) : pas d'équivalent pertinent en Go, à rediscuter si `EXT-003` est fait.

## Critères d'acceptation
- [ ] `grapi new demo && cd demo && grapi develop` démarre sur `:1337`, et `/_health` renvoie 204.
- [ ] Modifier un `schema.json` pendant `develop` met à jour l'API sans redémarrage (le nouveau champ apparaît dans la réponse en moins de 2 s).
- [ ] `.env` et `.env.example` ont exactement les mêmes clés (TU sur le template).
- [ ] `grapi start` sur un projet avec une config invalide sort en code 1 avec le nom du fichier et la clé en cause.
- [ ] `grapi report` n'affiche aucun secret (TU).

## Notes techniques
- `spf13/cobra`, `fsnotify/fsnotify` avec un debounce de 300 ms.
- Rechargement en processus : on construit un nouvel état (registres, schéma, routes) puis on l'échange atomiquement. Les requêtes en cours terminent sur l'ancien état.

## Référence Strapi
- `core/strapi/src/cli/commands/` (develop, start, version, report)
- `core/strapi/src/node/develop.ts`
- `cli/create-strapi-app/src/`

## Repris d'autres tickets
- `DB-004` : au démarrage, `migrate.WithLock` puis `migrate.Run` puis `schemasync.Sync` (avec `MigrationsRan`), comme `compat schema-sync` ; une erreur de migration arrête le processus avec un code non nul. Commandes `migration:status` (`migrate.UserStatus`) et `migration:up`.

## Dépendances
- `EXT-001`, `SCH-004`, `API-001`

## Fichiers
- `cmd/grapi/main.go`, `internal/cli/{root.go,new.go,develop.go,start.go,version.go,report.go}`, `internal/cli/templates/`
