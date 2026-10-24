---
status: in-progress
---

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
- [x] Les types générés pour le projet de test compilent avec `tsc --strict` (homelab).
- [x] Le JSON réel de l'API valide contre les types (test : conversion des types en zod via ts-to-zod, puis parse des réponses du banc).
- [x] La génération est déterministe : deux exécutions donnent des fichiers identiques.

## Avancement
- `grapi ts:generate-types [-o dossier]` (`internal/typegen`) écrit les `types/generated/contentTypes.d.ts` et `components.d.ts` de `strapi ts:generate-types` (format `@strapi/strapi`, `Schema.Attribute.*`, modificateurs dans l'ordre de `@strapi/typescript-utils`, clés triées). `TestMatchesStrapi` compare aux fichiers que Strapi 5.59 génère sur les 7 fixtures : identiques aux blancs près (Strapi passe par prettier), hors types Enterprise. `TestDeterministic`.
- `tsc --strict` (TypeScript de l'image de référence, contre `@strapi/strapi`) passe sur les déclarations de devfreelance, components et i18n-dp, avec un fichier qui instancie `Data.ContentType<K>` et `Data.Component<K>` pour tous les UID (2026-10-22).
- `api.d.ts` (`typegen.Client`), écrit à côté des fichiers Strapi plutôt que derrière une option `--strapi-compat` : les deux formats sortent à chaque fois, sans casser les projets qui attendent ceux de Strapi. Types autonomes, sans dépendance : une interface par content type et composant (`id`, `documentId`, timestamps, `publishedAt`, `locale` des types localisés, attributs privés et types admin exclus), relations, médias et composants optionnels, unions `__component` pour les zones dynamiques, enums en unions littérales, `CollectionResponse<T>`, `SingleResponse<T>`, `ErrorResponse`, `Media`, et les tables `ContentTypes` et `Components` indexées par UID. `make typecheck-compat` type aussi les 30 réponses réelles avec `ContentTypes[uid]` ; contre-épreuve faite (mauvais type rejeté). `TestClient`.
- `grapi develop --types-out <dossier>` écrit les types au démarrage puis à chaque changement de `src/` ou `config/` (mode framework compris).
- Reste : les types d'entrée (`data` des créations et mises à jour : relations par `documentId` ou `connect`, médias par id).

- `make typecheck-compat` (`compat/scripts/typecheck.sh`) : 30 réponses réelles de Strapi sur devfreelance (posts, catégories, thèmes, pages de thème, médias), typées `Data.ContentType<uid>` avec les types générés par grapi, passent `tsc --strict` dans l'image de référence. `tsc` plutôt que ts-to-zod : le contrôle des propriétés en trop des littéraux suffit ; contre-épreuve faite, un champ inconnu et un mauvais type sont rejetés.

## Notes techniques
- Génération pure Go (`text/template`), sans dépendance à Node.

## Référence Strapi
- `utils/typescript/src/generators/`
- `core/strapi/src/cli/commands/ts/generate-types.ts`

## Dépendances
- `SCH-001`, `SCH-002`, `CLI-001`

## Fichiers
- `internal/cli/tsgen/{generator.go,templates/}`
