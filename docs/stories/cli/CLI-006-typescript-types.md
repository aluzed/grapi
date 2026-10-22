# CLI-006 : Génération des types TypeScript pour les clients

## Contexte
`strapi ts:generate-types` écrit `types/generated/{contentTypes,components}.d.ts` à partir des schémas. Ces fichiers servent aux projets TS, côté serveur et côté front. Avec grapi, le serveur est en Go, mais les fronts restent souvent en TS : des types de l'API de contenu leur sont utiles.

## Objectif
Générer des types TypeScript qui décrivent les réponses et les entrées de l'API de contenu de grapi.

## Périmètre
- `grapi ts:generate-types [-o dossier]` :
  - une interface par content type et par composant (attributs, `documentId`, `id`, timestamps, `locale` et `publishedAt` si concernés) ;
  - des unions pour les zones dynamiques (`__component`) ;
  - les relations en types optionnels (présents seulement si populés) ;
  - les enums en unions littérales ;
  - les types génériques de réponse `{data, meta}` et d'erreur.
- Une option `--strapi-compat` génère des déclarations au format Strapi (`Schema.CollectionType`...), pour les projets TS qui s'appuient sur `@strapi/types`.
- Exécution automatique optionnelle pendant `develop` (`--types-out`).

## Hors périmètre
- Client SDK généré : on le fait à partir de l'OpenAPI (`PLG-005`).

## Critères d'acceptation
- [ ] Les types générés pour le projet de test compilent avec `tsc --strict` (homelab).
- [ ] Le JSON réel de l'API valide contre les types (test : conversion des types en zod via ts-to-zod, puis parse des réponses du banc).
- [ ] La génération est déterministe : deux exécutions donnent des fichiers identiques.

## Notes techniques
- Génération pure Go (`text/template`), sans dépendance à Node.

## Référence Strapi
- `utils/typescript/src/generators/`
- `core/strapi/src/cli/commands/ts/generate-types.ts`

## Dépendances
- `SCH-001`, `SCH-002`, `CLI-001`

## Fichiers
- `internal/cli/tsgen/{generator.go,templates/}`
