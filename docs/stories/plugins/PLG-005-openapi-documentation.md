---
status: in-progress
---

# PLG-005 : OpenAPI et documentation (Swagger UI)

## Contexte
Strapi a deux générateurs : le plugin `documentation` (Swagger UI sur `/documentation`, fichiers JSON par version) et le paquet core `openapi`, plus récent et expérimental, qui part des schémas zod des routes et sert `/openapi.json`. grapi n'en garde qu'un seul.

## Objectif
Un générateur OpenAPI 3 unique, qui part du registre des routes et des schémas, et sert à la fois la spec et un Swagger UI.

## Périmètre
- Génération depuis les content types (schémas de requête et de réponse : `{data, meta}`, filtres, populate, pagination) et les routes enregistrées (`API-002`), y compris les routes custom qui déclarent leurs schémas.
- Deux documents : `content-api` et `admin` (l'API admin de grapi, documentée et versionnée, conformément à D1).
- Exposition :
  - `/openapi.json` (accès `disabled` ou `public`, défaut `disabled`, comme Strapi) ;
  - Swagger UI sur `/documentation`, protégeable par mot de passe (`restrictedAccess`).
- Surcharges : `RegisterOverride(partialSpec, {pluginOrigin, excludeFromGeneration})` et un hook `MutateDocumentation(func(*openapi3.T))`.
- Configuration par défaut : `info`, `servers`, schéma de sécurité `bearerAuth` en JWT, schéma `Error`.
- CLI : `grapi openapi generate -o <fichier>` (`CLI-003`).

## Hors périmètre
- Le versionnage de la doc sur disque façon plugin documentation : la spec est générée à la demande, avec un cache invalidé au rechargement.

## Critères d'acceptation
- [x] La spec du projet de test est valide (validation `kin-openapi`) et générer un client depuis elle fonctionne (openapi-generator ou orval, sur le homelab).
- [x] Les chemins et opérations du Content API correspondent à ceux du `full_documentation.json` de Strapi pour le même projet (diff des chemins et méthodes dans le banc `SETUP-002`).
- [ ] `/documentation` avec `restrictedAccess` demande le mot de passe.
- [x] Une route custom avec schémas apparaît dans la spec.

## Avancement
- `internal/openapi` : le document du paquet `@strapi/openapi` de Strapi 5.59 (`server.openapi`), généré depuis le `schema.Set` avec la sémantique zod des validateurs de routes du core (champs requis, défauts, `readOnly`, min/max, `nullable`, entrées et sorties, `$ref` des documents et composants référencés, paramètres de requête dépliés, `operationId` et tags). `TestDocumentMatchesStrapi` compare chaque opération et chaque schéma au document que Strapi sert sur les 7 fixtures du banc (`internal/openapi/testdata/oas-*.json`) : identiques, horodatages des défauts mis à part. Les routes de plugins viennent de Strapi (`plugins.json`, identiques pour tout projet) et sont filtrées sur celles que grapi sert (`Server.Serves`).
- `app.MutateOpenAPI(fn)` retouche le document avant de le servir (`mutateDocumentation`).
- Routes custom du mode framework : `app.Route(...).OpenAPI(operation)` ajoute l'opération au document (`TestOpenAPIRoute`).
- `GET <prefix>/openapi.json` avec `server.openapi['content-api'].access: public` (test `TestOpenAPIRoute`), `grapi openapi [-o fichier]` sans démarrer le serveur.
- `make openapi-client` : le document de devfreelance est valide pour Redocly (jeu `minimal` ; il ne reste que des avertissements de style comme les `summary` absents, qui sont aussi dans le document de Strapi), et openapi-typescript en tire des types dont un client openapi-fetch compile en `--strict`. kin-openapi et openapi-generator ont été écartés : leur support d'OpenAPI 3.1 est partiel, et le client typescript-fetch d'openapi-generator ne compile pas (énumérations de paramètres de requête manquantes).
- Chemins et opérations : le document est celui que Strapi 5.59 sert lui-même (`@strapi/openapi`, `TestDocumentMatchesStrapi`), qui remplace le `full_documentation.json` du plugin documentation pour le Content API.
- Reste : Swagger UI et `restrictedAccess` (plugin documentation), `RegisterOverride`, document admin (avec l'admin).

- Repérage pour Swagger UI et `restrictedAccess` (code MIT de `@strapi/plugin-documentation` 5.59.0, sans dossier `ee/`) : routes `GET /documentation` et `/documentation/v:major.:minor.:patch` (auth: false, middleware `restrict-access`), `GET|POST /documentation/login`, plus `getInfos`, `regenerateDoc`, `updateSettings`, `deleteDoc` côté admin. La page injecte la spec (`full_documentation.json` du dossier `extensions/documentation/documentation/<version>/`) et `backendUrl` dans un gabarit qui charge `swagger-ui-dist` 4.19.0 depuis `/plugins/documentation/`. `restrict-access` lit `plugin_documentation_config` (`restrictedAccess`, `password` en bcrypt) et redirige vers `/documentation/login` sans `ctx.session.documentation.logged` ; `POST /login` compare le mot de passe et pose la session. Prérequis côté grapi : la session de Koa (`koa.sess` + `koa.sess.sig`, JSON en base64 signé par keygrip avec `app.keys`), absente aujourd'hui ; et les assets `swagger-ui-dist` embarqués (licence Apache-2.0 à ajouter au NOTICE).

## Notes techniques
- `getkin/kin-openapi/openapi3`. Swagger UI embarqué en statique (`embed`), version figée.
- Les schémas de route en Go : une petite DSL ou des types Go convertis par réflexion (`invopop/jsonschema`), à décider dans le ticket `API-002`.

## Référence Strapi
- `plugins/documentation/server/src/` (config, `services/helpers/build-component-schema.ts`, `build-api-endpoint-path.ts`, `services/override.ts`)
- `core/openapi/src/` (pipeline `generate`)
- `core/core/src/services/server/openapi.ts`

## Dépendances
- `API-002`, `API-006`, `SCH-005`

## Fichiers
- `internal/plugins/openapi/{generator.go,schemas.go,override.go,handler.go,swaggerui/}`
