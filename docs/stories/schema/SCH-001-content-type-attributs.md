---
status: done
---

# SCH-001 : Modèle de content type et attributs

## Contexte
Dans Strapi, tout part des fichiers `schema.json` des content types. grapi reprend ce format **tel quel**, pour qu'un projet Strapi existant soit lisible sans conversion (D3, `MIG-001`). Ce package est public (`schema/`) : les hooks Go du mode framework (`EXT-002`) l'utilisent.

## Objectif
Définir les types Go qui représentent un content type et ses attributs, parser et sérialiser le format `schema.json` de Strapi 5 sans perte, et calculer les propriétés dérivées.

## Périmètre
- `ContentType` :
  - `kind` (`collectionType` par défaut, ou `singleType`) ;
  - `collectionName` ;
  - `info{singularName, pluralName, displayName, description}` ;
  - `options{draftAndPublish (false par défaut), populateCreatorFields, comment, version}` et options inconnues conservées ;
  - `pluginOptions` (map libre, par exemple `i18n.localized`) ;
  - `attributes` (map **ordonnée** : l'ordre du JSON est conservé, il pilote les layouts de l'admin) ;
  - `indexes`, `foreignKeys`.
- Propriétés dérivées :
  - `uid` : `api::<api>.<singular>`, `plugin::<plugin>.<singular>`, `admin::<x>`, `strapi::<x>` ;
  - `modelType` (`contentType` ou `component`), `modelName`, `globalId` (PascalCase du singularName, préfixé par le plugin pour les types de plugin).
- **Attributs** : une seule struct `Attribute` (les champs sans objet pour un type restent nil), plus pratique en Go qu'une interface par sorte pour un schéma dynamique. Types couverts :
  - texte : `string`, `text`, `richtext`, `blocks`, `email`, `password`, `uid` (`targetField`, `options{separator, lowercase, decamelize, customReplacements, preserveLeadingUnderscore}`, `regex`), `enumeration` (`enum[]`, `enumName`) ;
  - nombres : `integer`, `biginteger` (exposé en chaîne), `float`, `decimal` ;
  - dates : `date`, `time`, `datetime`, `timestamp` ;
  - autres scalaires : `boolean`, `json` ;
  - `media` (`multiple`, `allowedTypes` parmi `images|videos|files|audios`) ;
  - `relation` (voir `DB-006`), `component`, `dynamiczone` (`SCH-002`), `customField` (`SCH-006`).
- **Options communes** :
  - `required`, `unique`, `private`, `configurable`, `default`, `min`, `max`, `minLength`, `maxLength`, `regex`, `writable`, `visible`, `searchable` ;
  - `pluginOptions`, `conditions.visible` (JSON-Logic, stocké tel quel) ;
  - `column{type, defaultTo, notNullable, unsigned, unique}`.
- **Champs ajoutés automatiquement** au chargement : `documentId`, `createdAt` et `updatedAt` (datetime), `publishedAt` (datetime, **ajouté même sans draft & publish**), `createdBy` et `updatedBy` (relation oneToOne vers `admin::user`, `useJoinTable:false`, `private` sauf si `populateCreatorFields`). `locale` et `localizations` sont ajoutés par i18n (`PLG-001`).
- **Noms réservés**, avec le comportement exact de Strapi (vérifié dans le source 5.59) :
  - au chargement, seuls `id` et `document_id` sont refusés (comparaison en snake_case) ; avec draft & publish, `status` donne un avertissement ;
  - la liste complète de `RESERVED_ATTRIBUTE_NAMES` (`created_at`, `locale`, `meta`, `strapi*`, etc.) et les noms de modèle réservés ne sont appliqués que par le Content-Type Builder (`IsReservedAttributeName`, `IsReservedModelName`, utilisés par `ADM-006`). Refuser `locale` au chargement casserait l'ouverture de projets Strapi existants ;
  - la comparaison se fait en snake_case façon lodash, qui enlève les underscores de tête : `__component` n'est donc pas refusé par ce contrôle, mais par le motif de nom du builder.
- Sérialisation aller-retour : `parse(json)` puis `marshal()` redonne un JSON équivalent (mêmes clés, même ordre d'attributs).

## Hors périmètre
- Le chargement depuis le disque (`SCH-004`), la validation croisée (`SCH-003`), le mapping SQL (`DB-002`).

## Critères d'acceptation
- [x] Tous les `schema.json` des fixtures du banc (`SETUP-002`) et de dev-freelance se parsent, puis se resérialisent à l'identique (test golden).
- [x] L'ordre des attributs est conservé au parse et au marshal.
- [x] Les champs automatiques sont présents avec les bons types et le bon caractère privé (test par cas : D&P activé ou non, `populateCreatorFields` activé ou non).
- [x] Un attribut nommé `id` ou `documentId` est rejeté au chargement, avec une erreur qui cite le modèle et l'attribut ; `locale` et `strapiFoo` se chargent, mais `IsReservedAttributeName` les refuse.
- [ ] `uid`, `globalId` et `modelName` sont identiques à ceux de Strapi sur les fixtures (vérifié contre la sortie de `strapi content-types:list` de la référence).
- [ ] Un type d'attribut inconnu provoque une erreur claire, sauf s'il est enregistré comme custom field.

## Notes techniques
- `globalId` et `snakeCase` reproduisent lodash (`upperFirst(camelCase())`, `snakeCase`), découpage des mots compris : casse, chiffres, ordinaux (`2nd`), accents retirés (deburr). Les cas de test viennent de lodash exécuté dans l'image de référence.
- Map ordonnée : décoder `attributes` avec `json.Decoder` token par token, ou avec une petite structure `OrderedMap[K,V]`. `map[string]` perdrait l'ordre.
- `default` peut être une fonction en JS : dans un `schema.json`, c'est toujours une valeur. Les défauts calculés passent par les hooks (`EXT-002`).
- Garder un champ `Extra map[string]json.RawMessage` sur chaque structure, pour ne jamais perdre une clé inconnue (forward compatibility avec Strapi).

## Référence Strapi
- `core/types/src/struct/schema.ts`, `core/types/src/schema/attribute/definitions/`, `core/types/src/schema/attribute/base.ts`
- `core/core/src/domain/content-type/index.ts` (uid, champs automatiques)
- `core/utils/src/content-types.ts` (`RESERVED_ATTRIBUTE_NAMES`, constantes)

## Dépendances
- `SETUP-001`.

## Fichiers
- `schema/contenttype.go`, `schema/attribute.go`, `schema/attributes_*.go`, `schema/reserved.go`, `schema/orderedmap.go`, `schema/testdata/`
