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
- [ ] Base vide : la synchro crée un schéma identique à celui de Strapi 5.59 (dump trié de `information_schema`, sur chaque fixture).
- [ ] Deuxième démarrage sans changement : aucune requête DDL (vérifié par les logs SQL), car le hash est inchangé.
- [ ] Ajout d'un attribut : seule la colonne est ajoutée, sans perte de données.
- [ ] Une table créée à la main en base, qui n'est pas dans le schéma stocké, survit à la synchro.
- [ ] Suppression d'un attribut, sans `forceMigration` : la colonne reste, et un avertissement est loggé. Avec `forceMigration` : la colonne est supprimée.
- [ ] Sur une copie de la base dev-freelance créée par Strapi, le premier démarrage de grapi donne `UNCHANGED` (point clé de `DB-011`).
- [ ] Le contenu de `strapi_database_schema` écrit par grapi est relu sans erreur par Strapi 5.59 (aller-retour entre les deux moteurs).

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
- `internal/db/sync/inspect_postgres.go`, `inspect_sqlite.go`, `diff.go`, `apply.go`, `storage.go`, `repairs.go`, `diff_test.go`
