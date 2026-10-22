# DB-001 : Dialectes, pool, transactions portées par `context`

## Contexte
Strapi supporte Postgres, MySQL/MariaDB et SQLite via Knex. Chaque dialecte a son introspecteur de schéma. Toutes les méthodes du Document Service s'exécutent dans une transaction. Les appels imbriqués réutilisent la transaction englobante grâce à l'async-local storage, et des hooks `onCommit` et `onRollback` sont disponibles.

## Objectif
Fournir une couche d'accès SQL multi-dialecte, avec des transactions imbriquables portées par `context.Context`, et un point d'extension pour ajouter MySQL en phase 3.

## Périmètre
- **Interface `Dialect`** :
  - quoting des identifiants, placeholders (`$1` ou `?`), types de colonnes par type d'attribut ;
  - support de `RETURNING`, `ILIKE` ou son équivalent, et opérateurs JSON ;
  - limites : longueur max d'identifiant, nombre max de paramètres par requête (SQLite : 32766) ;
  - gestion des booléens, des dates et des décimaux.
- **Implémentations** :
  - `postgres` (`jackc/pgx/v5` via `pgx/stdlib`, ou natif) ;
  - `sqlite` (`modernc.org/sqlite`, mode WAL, `foreign_keys=ON`, `busy_timeout`) ;
  - `mysql` déclaré, mais pas implémenté (phase 3).
- **Configuration** (`config/database`, via `EXT-001`) :
  - `connection.client` (`postgres`, `sqlite`, `mysql`) ;
  - `connection.connection{host, port, database, user, password, ssl, schema}` ou `connectionString`, `filename` pour SQLite ;
  - `pool{min, max, acquireTimeoutMillis}`, `debug` ;
  - `settings{forceMigration, runMigrations}` (utilisés par `DB-003` et `DB-004`).
- **Transactions** :
  - `db.Tx(ctx, func(ctx) error)` : démarre une transaction, ou réutilise celle déjà présente dans `ctx` (imbrication sans savepoint, comme Strapi) ;
  - `OnCommit(ctx, fn)` et `OnRollback(ctx, fn)` : les callbacks sont exécutés après le COMMIT ou le ROLLBACK de la transaction la plus externe ;
  - toute requête qui reçoit un `ctx` avec transaction l'utilise, sinon elle prend le pool.
- **Mapping des types** par dialecte :
  - `string` en `varchar(255)`, `text` en `text`, `richtext` et `blocks` en `text` ou `jsonb` (à vérifier) ;
  - `biginteger` en `bigint` ;
  - `decimal` en `decimal(10,2)` (précision de Strapi à vérifier) ;
  - `datetime` en `timestamp(6)` ;
  - `json` en `jsonb` (Postgres) ou `text` (SQLite) ;
  - `enumeration` en `varchar` (pas de type ENUM natif).
- **Observabilité** : logs SQL en mode debug (`slog`), avec durée et nombre de lignes.

## Hors périmètre
- Le DDL et la synchronisation (`DB-003`), la construction de requêtes (`DB-005`).

## Critères d'acceptation
- [ ] Tests d'intégration sur Postgres 16 et SQLite : connexion, requête simple, transaction validée, transaction annulée.
- [ ] Une transaction imbriquée (fonction A en transaction qui appelle B en transaction) n'ouvre qu'une seule transaction réelle (vérifié par compteur).
- [ ] Un `OnCommit` enregistré dans une fonction imbriquée s'exécute une seule fois, après le commit externe. Il ne s'exécute pas en cas de rollback.
- [ ] Le mapping des types donne, pour chaque attribut des fixtures, le même type de colonne que la référence Strapi (comparaison via `information_schema` sur Postgres et `PRAGMA table_info` sur SQLite).
- [ ] SQLite en accès concurrent (100 goroutines d'écriture) ne renvoie pas d'erreur `database is locked`.

## Notes techniques
- La transaction est stockée dans `ctx` via une clé privée. Ne jamais la mettre dans une variable globale.
- SQLite : une seule connexion d'écriture (`SetMaxOpenConns(1)` sur un pool d'écriture dédié, et un pool de lecture séparé), sinon le verrouillage pose problème en WAL.
- Pas d'ORM : le schéma est dynamique (voir `docs/feasibility.md`).

## Référence Strapi
- `core/database/src/dialects/` (postgresql, mysql, sqlite, avec leurs schema-inspector)
- `core/database/src/transaction-context.ts`
- `core/database/src/schema/builder.ts` (mapping des types de colonnes)

## Dépendances
- `SETUP-001`, `EXT-001` (lecture de la config, simulable par une structure en dur au début).

## Fichiers
- `internal/db/dialect.go`, `internal/db/dialect_postgres.go`, `internal/db/dialect_sqlite.go`, `internal/db/tx.go`, `internal/db/conn.go`, `internal/db/types.go`
