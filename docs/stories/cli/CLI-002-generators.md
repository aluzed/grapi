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
- [ ] Un content type généré se charge sans erreur, et ses routes CRUD répondent (intégration).
- [ ] Un nom réservé (`locale`, `documentId`...) est refusé avec un message clair.
- [ ] Le mode non interactif est utilisable en CI (aucune question si tous les flags sont fournis).
- [ ] Les fichiers Go générés passent `gofmt` et `go vet`.

## Notes techniques
- `charmbracelet/huh` pour les questions interactives, `text/template` pour les squelettes (il n'y a ici que des données internes, pas d'entrée utilisateur non maîtrisée dans les templates de code).

## Référence Strapi
- `generators/generators/src/plops/`
- `core/strapi/src/cli/commands/generate.ts`

## Dépendances
- `CLI-001`, `SCH-003`, `DB-004`, `EXT-002`

## Fichiers
- `internal/cli/generate/{api.go,contenttype.go,component.go,code.go,migration.go}`, `internal/cli/generate/templates/`
