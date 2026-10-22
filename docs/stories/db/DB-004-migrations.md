---
status: done
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
- **Ordre au démarrage** : utilisateur (si `settings.runMigrations`, vrai par défaut), puis internes, puis synchro du schéma. Vérifié dans `core/database/src/migrations/index.ts` : les fournisseurs sont `[user, internal]`, contrairement à ce qu'indiquait la première version de ce ticket.
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
- [x] Sur une base Strapi 5.59 fraîche, grapi ne rejoue aucune migration interne Strapi (`strapi_migrations_internal` reste inchangée). Sur une base que grapi crée, les 9 migrations internes de Strapi 5.59 sont enregistrées sans être exécutées, dans l'ordre d'une base Strapi neuve, pour que Strapi ne les rejoue pas.
- [x] Une migration SQL utilisateur s'exécute une seule fois, même après plusieurs redémarrages.
- [x] Une migration en échec est annulée (rollback) et n'est pas enregistrée. L'erreur cite la migration ; le code de sortie non nul viendra avec la commande de démarrage (`CLI-001`).
- [x] Deux instances démarrées en même temps sur Postgres : une seule exécute les migrations (deux pools, donc deux sessions, `postgres_integration_test.go`).
- [x] Une base v4 (sans `document_id`) est refusée avec un message qui pointe vers l'outil de migration de Strapi.

## Constats (relevés sur une base 5.59 neuve et sur la prod dev-freelance)
- Noms enregistrés : `5.0.0-rename-identifiers-longer-than-max-length`, `5.0.0-02-created-document-id`, `5.0.0-03-created-locale`, `5.0.0-04-created-published-at`, `5.0.0-05-drop-slug-fields-index`, `5.0.0-06-add-document-id-indexes`, `core::5.0.0-discard-drafts`, `admin::migrate-prefered-language-dk-to-da`, `upload::unsign-richtext-and-blocks-urls`. Le core et les plugins enregistrent leurs migrations dans la même table, préfixées par leur espace de noms : grapi utilise `grapi::`.
- Les migrations utilisateur sont enregistrées sous le nom du fichier, extension comprise. Un fichier `.sql` est exécuté d'un bloc ; il n'a pas de `down`.
- Strapi n'a pas de verrou de migration : son « heartbeat » n'est qu'un log de progression. Le verrou consultatif Postgres est un ajout de grapi.
- Sous SQLite, `time` est stocké en millisecondes depuis l'epoch, comme toutes les dates de Strapi.

## Reporté
- Commandes `grapi migration:status` et `migration:up` : les fonctions existent (`migrate.UserStatus`, `migrate.Run`), le câblage CLI attend la configuration (`EXT-001`) et `CLI-001`.
- Migrations de renommage générées par le CTB : avec `ADM-006`.
- Migrations `.js` : refusées au démarrage avec un message explicite tant que `EXT-003` n'existe pas.

## Notes techniques
- Les migrations Go ont la signature `func(ctx context.Context, d *db.DB) error` ; la transaction est portée par `ctx`.

## Référence Strapi
- `core/database/src/migrations/` (`internal.ts`, `users.ts`, `runner.ts`, `storage.ts`, `heartbeat.ts`, `file-builder.ts`)
- `core/database/src/migrations/internal-migrations/`
- `core/core/src/migrations/` (migrations côté core : draft-publish, i18n, first-published-at)

## Dépendances
- `DB-001`, `DB-003`.

## Fichiers
- `internal/db/migrate/migrate.go`, `storage.go`, tests ; `cmd/grapi/migration.go` à venir
