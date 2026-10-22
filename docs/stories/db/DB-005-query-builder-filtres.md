---
status: todo
---

# DB-005 : Query builder et opérateurs de filtre

## Contexte
Toute lecture passe par le moteur de requêtes de Strapi (`db.query(uid)`) : filtres imbriqués à travers les relations et les composants, tri profond, recherche `_q`, comptage. C'est le cœur de la difficulté de la couche données, avec les relations (`DB-006`).

## Objectif
Un query builder dynamique, piloté par les métadonnées (`DB-002`), qui traduit les paramètres de requête Strapi en SQL correct, performant et paramétré sur Postgres et SQLite.

## Périmètre
- **API** : `Query(uid)` avec `FindOne`, `FindMany`, `Count`, `Create`, `CreateMany`, `Update`, `UpdateMany`, `Delete`, `DeleteMany`, `Populate`, `Load` et `LoadPages`.
- **Paramètres** : `select`, `where`, `orderBy`, `limit`, `offset`, `populate` (délégué à `DB-006`), `_q`, `count`.
- **Opérateurs** :
  - logiques : `$and`, `$or`, `$not` ;
  - comparaisons : `$eq`, `$eqi`, `$ne`, `$nei`, `$gt`, `$gte`, `$lt`, `$lte`, `$in`, `$notIn`, `$between` ;
  - nullité : `$null`, `$notNull` ;
  - texte : `$contains`, `$notContains`, `$containsi`, `$notContainsi`, `$startsWith`, `$startsWithi`, `$endsWith`, `$endsWithi` ;
  - interne : `$jsonSupersetOf`.
- **Formes courtes** : `{field: value}` vaut `$eq`, et `{field: [a,b]}` vaut `$in`. Reproduire les cas exacts de Strapi : tableau implicite, `null`, valeurs booléennes en chaîne.
- **Coercition des valeurs** selon le type d'attribut : `"true"` devient booléen, une date ISO devient une date, un entier en chaîne devient un entier. Une erreur de type renvoie une `ValidationError`.
- **Filtres imbriqués** : `{category: {slug: {$eq: "dev"}}}` devient une jointure via la table de lien, avec `DISTINCT` ou une sous-requête `EXISTS` pour ne pas dupliquer les lignes. Ils fonctionnent sur les relations, les composants (via `_cmps`) et les médias.
- **Tri** : sur les colonnes, et **tri profond** à travers une relation (`orderBy: {category: {label: 'asc'}}`). Les `NULL` sont placés comme dans Strapi pour chaque dialecte (à relever sur la référence).
- **`_q`** : `ILIKE` (ou `LIKE` insensible à la casse sous SQLite) sur les colonnes `string`, `text`, `richtext`, `email`, `uid` et `enumeration` non privées. Si le terme est numérique, la recherche inclut aussi l'égalité sur les colonnes numériques.
- **Pagination** : `limit` et `offset` en SQL. Avec les jointures, la pagination porte sur les ids distincts (sous-requête), pas sur les lignes jointes.
- **Count** : `COUNT(DISTINCT id)` dès qu'une jointure existe.
- **Sécurité** : 100 % paramétré. Aucun nom d'identifiant ne vient de l'utilisateur sans validation par les métadonnées.

## Hors périmètre
- La traduction des paramètres REST en paramètres de requête (`API-004`), la sanitisation selon les permissions (`API-005`).

## Critères d'acceptation
- [ ] Chaque opérateur a un test d'intégration sur Postgres et sur SQLite, dont la casse et les accents pour `$containsi` et `$eqi`.
- [ ] Banc : au moins 60 cas de filtres REST sur les fixtures `basic`, `relations` et `components`, avec des réponses identiques à Strapi (même ensemble **et** même ordre).
- [ ] Un filtre sur une relation manyToMany avec pagination renvoie le bon nombre d'entrées et le bon `total` (sans doublon).
- [ ] Le tri profond sur `category.label` donne le même ordre que Strapi, `NULL` compris.
- [ ] `_q=42` trouve à la fois `title` contenant « 42 » et `views = 42`.
- [ ] Fuzzing (`go test -fuzz`) sur l'arbre de filtres : jamais de panic, jamais de SQL invalide.

## Notes techniques
- Représenter le `where` comme un AST typé (après validation), puis compiler l'AST par dialecte.
- Préférer `EXISTS` aux jointures avec `DISTINCT` pour les filtres sur relation : c'est plus simple pour la pagination et le comptage. Mesurer les deux sur la fixture dev-freelance.
- `$jsonSupersetOf` : `@>` sur Postgres, `json_each` sous SQLite.

## Référence Strapi
- `core/database/src/query/query-builder.ts`
- `core/database/src/query/helpers/{where,join,order-by,search}.ts`
- `core/utils/src/operators.ts`

## Dépendances
- `DB-001`, `DB-002`.

## Fichiers
- `internal/db/query/builder.go`, `ast.go`, `where.go`, `join.go`, `orderby.go`, `search.go`, `compile_postgres.go`, `compile_sqlite.go`, `*_test.go`
