---
status: todo
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
- [ ] La spec du projet de test est valide (validation `kin-openapi`) et générer un client depuis elle fonctionne (openapi-generator ou orval, sur le homelab).
- [ ] Les chemins et opérations du Content API correspondent à ceux du `full_documentation.json` de Strapi pour le même projet (diff des chemins et méthodes dans le banc `SETUP-002`).
- [ ] `/documentation` avec `restrictedAccess` demande le mot de passe.
- [ ] Une route custom avec schémas apparaît dans la spec.

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
