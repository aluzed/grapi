---
status: done
---

# CLI-002 : Générateurs (`generate api|content-type|controller|policy|middleware|migration`)

## Contexte
`strapi generate` (plop) crée les fichiers d'une API, d'un content type, d'un controller, etc. Chez grapi, les schémas restent en JSON (format Strapi). Le code (controllers, policies, middlewares) est du Go en mode framework (`EXT-002`), ou du JS/TS si `EXT-003` est en place.

## Objectif
Des générateurs interactifs et non interactifs qui produisent des fichiers prêts à l'emploi.

## Périmètre
- `grapi generate api <nom>` : `src/api/<nom>/` avec routes, controller et service vides.
- `grapi generate content-type` : questions sur displayName, singular et plural (déduits), kind, draftAndPublish, i18n et attributs. Il écrit `src/api/<api>/content-types/<nom>/schema.json` et les routes core. Le mode non interactif passe par des flags ou par `--from schema.json`.
- `grapi generate component <categorie>.<nom>`.
- `grapi generate controller|service|policy|middleware <nom> [--api <api>|--plugin <p>]` : squelettes Go (ou JS/TS si `EXT-003` est actif et `--lang ts`).
- `grapi generate migration <nom>` : fichier horodaté dans `database/migrations/` (`DB-004`).
- Validation à la génération (`SCH-003`) : noms réservés, unicité de singular et plural.

## Hors périmètre
- `templates:generate` (templates de projet Strapi Cloud).

## Critères d'acceptation
- [x] Un content type généré se charge sans erreur, et ses routes CRUD répondent (intégration).
- [x] Un nom réservé (`locale`, `documentId`...) est refusé avec un message clair.
- [x] Le mode non interactif est utilisable en CI (aucune question si tous les flags sont fournis).
- [x] Les fichiers Go générés passent `gofmt` et `go vet`.

## Avancement
- `grapi generate content-type --name <n> [--plural] [--api] [--kind] [--draft-and-publish] [--i18n] [--attr nom:type[:required]]...` et `grapi generate component --name <categorie>.<nom> [--attr ...]` (`cmd/grapi/generate.go`), non interactifs. Les noms réservés du Content-Type Builder sont refusés avant écriture ; le schéma écrit est rechargé et supprimé s'il ne charge pas ; un schéma existant n'est pas écrasé. `TestGenerate` : le content type généré est servi (`GET /api/notes` 200).
- `grapi generate migration --name <n>` : `database/migrations/<YYYY.MM.DDTHH.mm.ss>.<n>.sql` (nommage de Strapi, qui ordonne les migrations), exécutée au démarrage suivant (`TestGenerate`).
- Squelettes Go d'un projet framework : `policy`, `route` (l'équivalent Go d'un controller), `document-middleware`, `lifecycle` (`cmd/grapi/generate_go.go`). Chaque fichier ajoute sa fonction de configuration dans `init`, le `main.go` généré les applique à l'App ; un fichier existant n'est jamais écrasé, un projet sans `go.mod`/`main.go` est refusé. `TestGenerateGoSkeletons` : projet `grapi new --framework` (qui écrit maintenant aussi `go.mod`), les cinq squelettes, puis `gofmt -l`, `go vet` et `go build`.
- `grapi generate api --name <n>` (framework) : `api_<n>.go` avec un service Go et la route d'exemple de `strapi generate api` (`GET /<n>`, action `api::<n>.<n>.exampleAction`), couvert par `TestGenerateGoSkeletons`. Un projet sans framework n'exécute pas de code JS : rien à générer pour lui.
- Mode interactif : dans un terminal, `generate content-type` et `generate component` sans `--name` posent les questions de `strapi generate` (nom, pluriel, kind, draft and publish, i18n, puis les attributs un par un) et seulement pour les flags absents ; hors terminal, rien n'est demandé (`TestGenerateInteractive`).

## Notes techniques
- `charmbracelet/huh` pour les questions interactives, `text/template` pour les squelettes (il n'y a ici que des données internes, pas d'entrée utilisateur non maîtrisée dans les templates de code).

## Référence Strapi
- `generators/generators/src/plops/`
- `core/strapi/src/cli/commands/generate.ts`

## Dépendances
- `CLI-001`, `SCH-003`, `DB-004`, `EXT-002`

## Fichiers
- `internal/cli/generate/{api.go,contenttype.go,component.go,code.go,migration.go}`, `internal/cli/generate/templates/`
