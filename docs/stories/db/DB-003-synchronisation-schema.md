---
status: done
---

# DB-003 : Synchronisation du schéma au démarrage

## Contexte
Strapi n'utilise pas de migrations pour le schéma : au démarrage, il compare le schéma attendu à la base et applique la différence. Pour ne pas toucher aux objets qu'il n'a pas créés, il fait un **diff à 3 voies** : la base réelle, le dernier schéma qu'il a stocké, et le nouveau schéma.

## Objectif
Reproduire la synchronisation de Strapi 5 : introspection, diff à 3 voies, application en DDL, persistance du schéma et de son hash.

## Périmètre
- **Introspection** par dialecte : tables, colonnes (type, nullabilité, défaut, unsigned), index (unique ou non, colonnes), FK (colonnes, table et colonnes référencées, `ON DELETE`).
- **Stockage** : table `strapi_database_schema` (`id`, `schema` en JSON, `time`, `hash`).
- **Démarrage rapide** : si le hash du schéma attendu est égal au hash stocké, et qu'aucune migration n'a tourné, il n'y a ni introspection ni DDL.
- **Diff** :
  - par table : ajoutée, supprimée, modifiée ;
  - par colonne, index et FK : ajouté, supprimé, modifié ;
  - statut global `CHANGED` ou `UNCHANGED`.
- **Règle des 3 voies** : un objet présent en base mais absent du schéma stocké précédent (donc **jamais suivi**) est ignoré. Ce n'est pas à nous de le supprimer.
- **Suppressions** : les tables et colonnes suivies qui disparaissent sont supprimées seulement si `settings.forceMigration` vaut true (le défaut de Strapi est à vérifier). Sinon, un avertissement est loggé.
- **Ordre d'application** : création des tables, puis ajout et modification des colonnes, puis index, puis FK. Pour les suppressions, l'ordre est inversé.
- **Transaction** : DDL transactionnel sur Postgres et SQLite (pas sur MySQL : à documenter pour la phase 3).
- **Tables internes** créées par la synchro : `strapi_migrations`, `strapi_migrations_internal`, `strapi_database_schema`, `strapi_core_store_settings` (`DB-010`), `strapi_webhooks` (`EXT-006`), et les tables admin et plugins.
- **Hooks** `beforeSync` et `afterSync` (registre de hooks `EXT-008`), utilisés par les plugins.
- **Repairs** de Strapi, exécutés après la synchro : suppression des types morph orphelins et correction des tables de jointure unidirectionnelles.

## Hors périmètre
- Les renommages de champs (`DB-004`, via des migrations générées par le CTB).

## Critères d'acceptation
- [x] Base vide : la synchro crée un schéma identique à celui de Strapi 5.59, sur chaque fixture. SQLite : les `CREATE TABLE` et `CREATE INDEX` de `sqlite_master` sont identiques octet pour octet (`sync_test.go`). Postgres : `information_schema.columns`, `pg_indexes` et les contraintes sont identiques (`postgres_integration_test.go`, `make test-integration-homelab`).
- [x] Deuxième démarrage sans changement : le hash est inchangé, il n'y a ni introspection ni DDL. Un diff complet sur la base synchronisée donne aussi `UNCHANGED`.
- [x] Ajout d'un attribut : seule la colonne est ajoutée, sans perte de données.
- [x] Une table créée à la main en base, qui n'est pas dans le schéma stocké, survit à la synchro.
- [x] Suppression d'un attribut : avec `forceMigration` (le défaut de Strapi, vérifié dans `core/database/src/index.ts`), la colonne est supprimée ; avec `KeepRemoved`, elle reste et un avertissement est loggé.
- [x] Sur une base créée par Strapi, le premier démarrage de grapi donne `UNCHANGED` : banc `roundtrip`, sur chaque fixture (dont `devfreelance`, dont le schéma stocké est identique à celui de la prod), en SQLite et en Postgres.
- [x] Le contenu de `strapi_database_schema` écrit par grapi est relu sans erreur par Strapi 5.59 : dans le banc `roundtrip`, Strapi démarre sur la base créée par grapi et répond au corpus exactement comme sur sa propre base. Le hash écrit par grapi est celui que Strapi calcule pour les mêmes tables (`testdata/strapi-hashes.json`, généré par le code de Strapi).

## Réalisation
- `internal/db/schemasync` : `Hash` (collation `localeCompare` reproduite), `Inspect` (SQLite, Postgres), `Diff` (fonction pure), application dans l'ordre du builder de Strapi, `Store`/`ReadStored`, `RemoveOrphanMorphTypes`.
- Sous SQLite, knex crée les tables avec clés étrangères puis les reconstruit en ajoutant ces clés une seconde fois : le `CREATE TABLE` stocké porte chaque contrainte en double. grapi écrit directement ce texte final.
- Les écarts volontaires (tables des fonctions Enterprise et IA jamais supprimées, index modifié sans `forceMigration`, reconstruction SQLite) sont dans `docs/divergences.md`.

## Reporté
- Hooks `beforeSync` et `afterSync` : à brancher quand le registre de hooks existera (`EXT-008`). Le repair des types morph orphelins est prêt, il sera appelé par la séquence de démarrage.
- Repair des tables de jointure unidirectionnelles (`processUnidirectionalJoinTables` avec `cleanComponentJoinTable`) : exécuté une seule fois, marqué dans le core store, et dépendant de la logique du Document Service. À faire avec `DB-006` et `DB-010`.
- Tables `strapi_migrations` et `strapi_migrations_internal`, et passage de `MigrationsRan` : avec `DB-004`.

## Notes techniques
- Le JSON stocké dans `strapi_database_schema` doit avoir **la même forme** que celui de Strapi (tables, colonnes, index, FK avec les mêmes clés), sinon l'aller-retour entre Strapi et grapi casse. Le format est à relever sur une base de référence.
- Comparer les types de colonnes en forme normalisée par dialecte (`character varying(255)` correspond à `varchar(255)`).
- Écrire le diff comme une fonction pure (`Diff(db, stored, desired) Changes`), testable sans base.

## Référence Strapi
- `core/database/src/schema/index.ts`, `diff.ts`, `builder.ts`, `storage.ts`, `schema.ts`
- `core/database/src/dialects/*/schema-inspector.ts`
- `core/database/src/repairs/`

## Dépendances
- `DB-001`, `DB-002`.

## Fichiers
- `internal/db/schemasync/*.go` (le package ne s'appelle pas `sync` pour ne pas masquer celui de la bibliothèque standard), `compat/cmd/compat/schemasync.go`, `compat/scripts/remote.sh` (mode `roundtrip`)
