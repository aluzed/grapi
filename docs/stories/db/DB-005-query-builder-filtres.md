---
status: done
---

# DB-005 : Query builder et opérateurs de filtre

## Contexte
Toute lecture passe par le moteur de requêtes de Strapi (`db.query(uid)`) : filtres imbriqués à travers les relations et les composants, tri profond, recherche `_q`, comptage. C'est le cœur de la difficulté de la couche données, avec les relations (`DB-006`).

## Objectif
Un query builder dynamique, piloté par les métadonnées (`DB-002`), qui traduit les paramètres de requête Strapi en SQL correct, performant et paramétré sur Postgres et SQLite.

## Périmètre
- **API** : `FindOne`, `FindMany`, `Count`. Les écritures (`Create`, `Update`, `Delete` et leurs variantes) passent par l'entity manager de Strapi, qui gère aussi les relations, les valeurs par défaut et les horodatages : elles sont faites avec `DB-006`, comme `Populate`, `Load` et `LoadPages`.
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
- **`_q`** : sur `id` et les colonnes `string`, `text`, `richtext`, `email`, `uid` et `enumeration` (sauf `searchable: false`), plus les colonnes numériques si le terme est un nombre. Strapi compare tout en texte : `col::text ILIKE '%terme%'` sous Postgres, `col LIKE '%terme%' ESCAPE '\'` sous SQLite (sensible à la casse hors ASCII). Il n'y a pas d'égalité numérique, contrairement à la première version de ce ticket. Le filtrage des champs privés relève de `API-005`.
- **Pagination** : `limit` et `offset` en SQL. Avec les jointures, la pagination porte sur les ids distincts (sous-requête), pas sur les lignes jointes.
- **Count** : `COUNT(DISTINCT id)` dès qu'une jointure existe.
- **Sécurité** : 100 % paramétré. Aucun nom d'identifiant ne vient de l'utilisateur sans validation par les métadonnées.

## Hors périmètre
- La traduction des paramètres REST en paramètres de requête (`API-004`), la sanitisation selon les permissions (`API-005`).

## Critères d'acceptation
- [x] Chaque opérateur est testé sur Postgres et sur SQLite, dont la casse et les accents pour `$containsi` et `$eqi` : banc `query` (ci-dessous), en comparaison directe avec le moteur de Strapi.
- [x] Banc : 162 cas sur les fixtures `basic`, `relations`, `components` et `devfreelance`, exécutés par `db.query(uid)` de Strapi 5.59 et par grapi sur la même base, avec des résultats identiques (même ensemble **et** même ordre, erreurs comprises), sous SQLite et sous Postgres (`make bench-compat FIXTURE=relations MODE=query DB=postgres`). Le niveau REST viendra avec `API-004`, par le mode `grapi` du banc.
- [x] Un filtre sur une relation manyToMany avec pagination renvoie le bon nombre d'entrées et le bon total (`paginate-with-join`, `count-by-tags`).
- [x] Le tri profond à travers une relation donne le même ordre que Strapi, `NULL` compris (`sort-by-writer`, `sort-by-category-paginated`, `sort-tags-by-posts-rank`, `sort-by-seo-title`).
- [x] `_q=42` trouve à la fois un `title` contenant « 42 » et `views = 42` (`search-number`).
- [x] Fuzzing (`go test -fuzz FuzzWhere`) sur l'arbre de filtres : jamais de panic, jamais de SQL invalide (3,2 millions d'exécutions sans échec).

## Constats
- Strapi refuse un `where` racine sous forme de tableau (« Where must be an object »), mais accepte les tableaux à l'intérieur.
- Dans le tri profond, Strapi demande `NULLS LAST` pour la numérotation des lignes, mais knex l'ignore dans une fonction de fenêtre : chaque dialecte applique son ordre par défaut (Postgres met les NULL en premier en `DESC`). grapi reproduit l'ordre obtenu.
- `{relation: {$not: {champ: ...}}}` lève « Undefined attribute level operator » chez Strapi ; `{relation: {$not: 1}}` fonctionne.
- `$null: "false"` vaut vrai (vérité JavaScript d'une chaîne non vide).
- SQLite : `LOWER` ne traite que l'ASCII, donc `$eqi` et `$containsi` ne trouvent pas « été » pour « ÉTÉ ».

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
- `internal/db/query/builder.go`, `where.go`, `fields.go`, `query_test.go` ; banc : `compat/querycases/*.yaml`, `compat/tools/run-queries.js`, `compat/cmd/compat/query.go`
