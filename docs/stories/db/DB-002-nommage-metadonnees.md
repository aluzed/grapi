---
status: done
---

# DB-002 : Nommage et métadonnées compatibles Strapi

## Contexte
Pour ouvrir une base Strapi existante sans migration (D3, `DB-011`), grapi doit produire **exactement** les mêmes noms de tables, de colonnes, d'index et de clés étrangères. Strapi 5 tronque les identifiants longs avec un hash. Une erreur de nommage n'est pas anodine : la synchronisation verrait une table « inconnue » et en créerait une nouvelle.

## Objectif
Transformer les schémas (`SCH-*`) en métadonnées de base (tables, colonnes, index, FK, tables de jointure), avec l'algorithme de nommage de Strapi 5 reproduit bit pour bit.

## Périmètre
- **Algorithme de nommage** :
  - longueur max `IDENTIFIER_MAX_LENGTH = 55` ;
  - longueur de hash `HASH_LENGTH = 5`, sans séparateur avant le hash ;
  - séparateur `_` ;
  - au moins 3 caractères conservés par token (`MIN_TOKEN_LENGTH`).
- **Tokens** : les noms sont compressibles. Les préfixes et suffixes ne le sont pas et passent par la table de remplacement fixe :
  - `links` en `lnk`, `order` en `ord`, `morphs` en `mph`, `index` en `idx` ;
  - `inv_fk` en `ifk`, `order_fk` en `ofk`, `order_inv_fk` en `oifk` ;
  - `id_column_index` en `idix`, `order_index` en `oidx`, `unique` en `uq`, `primary` en `pk`.
- **Étapes** :
  1. Si le nom complet, avec les tokens courts, tient dans 55 caractères, il est gardé.
  2. Sinon, les tokens compressibles se partagent la longueur disponible, et chaque token tronqué reçoit le hash de son nom complet.
  3. Le hash est `shake256` sur le nom, encodé en hexadécimal et coupé à 5 caractères. En Go : `sha3.ShakeSum256` de `golang.org/x/crypto/sha3`.
- **Fonctions** :
  - `TableName`, `JoinTableName(collection, attr)` (suffixe `links`), `MorphTableName` (suffixe `morphs`) ;
  - `ColumnName`, `JoinColumnIdName` (`<attr>_id`), `InverseJoinColumnIdName` (`inv_<attr>_id`) ;
  - `OrderColumnName` (`<singular>_ord`), `InverseOrderColumnName` (`<singular>_ord_inv`, en vérifiant l'ordre exact du préfixe `inv`) ;
  - `MorphColumn*` ;
  - `IndexName`, `FkIndexName`, `UniqueIndexName`, `PrimaryIndexName`, `InverseFkIndexName`, `OrderFkIndexName`, `OrderInverseFkIndexName`, `IdColumnIndexName`, `OrderIndexName`.
- **Métadonnées produites** pour chaque modèle :
  - table, colonnes (snake_case des attributs), clé primaire `id` ;
  - `document_id`, `locale`, `published_at` ;
  - index `<table>_documents_idx` sur `(document_id, locale, published_at)` ;
  - `created_by_id` et `updated_by_id` (join column, `useJoinTable:false`) ;
  - tables de jointure, tables morph, tables `_cmps` (`DB-007`), avec leurs index et FK (`ON DELETE CASCADE` là où Strapi les met).
- **Mode legacy** `maxLength = 0` (noms v4 non tronqués), utilisé par la migration interne 5.0.0-01. Il est exposé, mais pas utilisé par défaut.

## Hors périmètre
- Le stockage et la lecture des relations (`DB-006`), le DDL (`DB-003`).

## Critères d'acceptation
- [x] **Vecteurs de test** : chaque cas des tests de Strapi (`core/database/src/utils/identifiers/__tests__/`) est porté en test de table Go et donne la même chaîne (`internal/db/naming/strapi_cases_test.go`), en plus des 1 269 cas golden générés par le vrai `Identifiers`. `createHash("myData", 5)` vaut `93da1` (vérifié dans l'image de référence ; une version antérieure de ce ticket indiquait `03f85`, ce qui était faux).
- [x] Pour chaque fixture du banc, le schéma calculé par grapi (tables, colonnes, index, FK, dans l'ordre et avec les clés JSON de Strapi) est identique octet pour octet à celui que Strapi 5.59 stocke dans `strapi_database_schema`, sous SQLite comme sous Postgres (`internal/db/metadata/golden_test.go`). Les tables des fonctions Enterprise et IA sont exclues de la comparaison. La comparaison du schéma physique (`information_schema`) relève du DDL, donc de `DB-003`.
- [x] Même chose pour dev-freelance : les 60 tables du `strapi_database_schema` de la prod sont identiques au golden de la fixture `devfreelance` (vérifié le 2026-10-22, lecture seule), que grapi reproduit.
- [x] Un attribut au nom de 70 caractères produit un nom tronqué identique à Strapi : fixture `naming` (tables, colonnes, tables de liens et de composants au-delà de 55 caractères).

## Réalisation
- Les content types que Strapi embarque hors `src/` (admin, upload, i18n, users-permissions) et ses modèles internes (core store, webhooks) sont extraits des points d'entrée Community Edition par `compat/tools/gen-builtin-schemas.js` et embarqués dans `internal/builtin`.
- Le loader donne aux content types de plugin le `collectionName` par défaut de Strapi (`<plugin>_<singularName>`), charge les content types admin et ajoute `locale` et `localizations` à tous les content types quand i18n est présent, comme le plugin i18n de Strapi.
- Les relations `oneWay` et `manyWay` sont refusées par les métadonnées (« unknown relation »), comme dans Strapi 5.

## Notes techniques
- Le commentaire de Strapi est explicite : toute différence de sortie entraîne une perte de données à la synchronisation. Ce package est donc **figé par des tests golden**, et toute modification du golden exige une revue explicite.
- La conversion snake_case doit reproduire `lodash/snakeCase` (découpage des mots, chiffres, acronymes). Il faut porter les cas limites (`myAPIKey`, `field2Name`) en tests.

## Référence Strapi
- `core/database/src/utils/identifiers/index.ts`, `hash.ts`, `__tests__/`
- `core/database/src/metadata/` (`attribute-naming.ts`, `relations.ts`, `index.ts`)
- `core/core/src/utils/transform-content-types-to-models.ts`

## Dépendances
- `SCH-001`, `SCH-002`, `DB-001`.

## Fichiers
- `internal/db/naming/*.go`, `internal/db/metadata/*.go`, `internal/builtin/`, `compat/tools/gen-builtin-schemas.js`, `compat/fixtures/naming/`, `compat/corpus/naming.yaml`
