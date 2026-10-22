---
status: todo
---

# DB-004 : Migrations utilisateur et internes

## Contexte
Strapi exécute deux familles de migrations **avant** la synchronisation du schéma (`DB-003`) :

- les migrations **internes** (montée de version de la base), suivies dans `strapi_migrations_internal` ;
- les migrations **utilisateur** (`database/migrations/*.js`), suivies dans `strapi_migrations`.

Le CTB génère aussi des migrations de renommage d'attribut, pour ne pas perdre de données.

## Objectif
Exécuter les migrations dans le bon ordre et chacune dans sa transaction, avec le même suivi que Strapi pour rester compatible avec une base existante.

## Périmètre
- **Ordre au démarrage** : internes, puis utilisateur (si `settings.runMigrations`, vrai par défaut à vérifier), puis synchro du schéma.
- **Migrations internes de Strapi 5.0.0**, en lecture de leur effet. Elles sont nécessaires seulement pour ouvrir une base v4 migrée, ce qui est hors cible en phase 1 :
  - `5.0.0-01` (identifiants longs), `5.0.0-02` (document_id), `5.0.0-03` (locale), `5.0.0-04` (published_at), `5.0.0-05` (suppression de l'index unique sur slug), `5.0.0-06` (index document_id).
  - grapi les **reconnaît comme déjà appliquées** sur une base Strapi 5, ne les rejoue jamais, et refuse une base v4 avec un message clair.
- **Migrations internes propres à grapi** : préfixe `grapi-<version>-<nn>`, dans la même table, sans collision avec les noms Strapi.
- **Migrations utilisateur** :
  - fichiers SQL (`database/migrations/<horodatage>_<nom>.up.sql`, et `.down.sql` optionnel) ;
  - migrations Go enregistrées en mode framework (`EXT-002`) ;
  - fichiers `.js` de Strapi : exécutés en phase 3 par les hooks JS (`EXT-003`), et signalés par `grapi doctor` avant cela.
- **Exécution** :
  - tri lexicographique, une transaction par migration ;
  - nom enregistré après succès ;
  - une erreur arrête le démarrage, avec le nom de la migration et l'erreur SQL.
- **Verrou** : verrou consultatif Postgres (`pg_advisory_lock`) ou table de verrou sous SQLite, pour qu'une seule instance migre à la fois (« heartbeat » chez Strapi).
- **Migrations de renommage générées** (`ADM-006`) : renommer une colonne ou une table de jointure en conservant les données.
- **CLI** : `grapi migration:status`, `grapi migration:up` (exécution manuelle), et `grapi generate migration` (`CLI-002`).

## Hors périmètre
- Les migrations v4 vers v5 (Strapi fournit son propre outil, on s'appuie dessus).

## Critères d'acceptation
- [ ] Sur une base Strapi 5.59 fraîche, grapi ne rejoue aucune migration interne Strapi (`strapi_migrations_internal` reste inchangée).
- [ ] Une migration SQL utilisateur s'exécute une seule fois, même après plusieurs redémarrages.
- [ ] Une migration en échec est annulée (rollback) et n'est pas enregistrée. Le démarrage s'arrête avec un code non nul.
- [ ] Deux instances démarrées en même temps sur Postgres : une seule exécute les migrations (test avec 2 processus).
- [ ] Une base v4 (sans `document_id`) est refusée avec un message qui pointe vers l'outil de migration de Strapi.

## Notes techniques
- Les noms enregistrés par Strapi incluent l'extension ou non : à vérifier sur une base réelle pour ne pas rejouer les migrations utilisateur d'un projet migré.
- Les migrations Go ont une signature `func(ctx context.Context, tx db.Tx) error`.

## Référence Strapi
- `core/database/src/migrations/` (`internal.ts`, `users.ts`, `runner.ts`, `storage.ts`, `heartbeat.ts`, `file-builder.ts`)
- `core/database/src/migrations/internal-migrations/`
- `core/core/src/migrations/` (migrations côté core : draft-publish, i18n, first-published-at)

## Dépendances
- `DB-001`, `DB-003`.

## Fichiers
- `internal/db/migrate/runner.go`, `internal.go`, `user.go`, `lock.go`, `storage.go`, `cmd/grapi/migration.go`
