---
status: done
---

# DB-006 : Relations : stockage, populate, connect/disconnect/set, positions, ordre fractionnaire

## Contexte
Les relations sont la partie la plus riche de Strapi. Il y a 10 types, un stockage en table de jointure par défaut, des colonnes d'ordre à valeurs flottantes, et des écritures relatives (`connect` avant ou après une autre entrée). La relecture de l'ordre doit être identique à Strapi, sinon les listes ordonnées des fronts changent.

## Objectif
Lire, écrire et peupler toutes les relations non polymorphes, en reproduisant exactement le stockage et l'ordre de Strapi. Les relations morph sont traitées dans `DB-008`.

## Périmètre
- **Types** : `oneToOne`, `oneToMany`, `manyToOne`, `manyToMany`, `oneWay`, `manyWay`.
- **Stockage** :
  - par défaut, table de jointure `<table>_<attr>_lnk` avec `<singular>_id`, `inv_<singular>_id`, la colonne d'ordre `<singular>_ord` et l'ordre inverse `<singular>_ord_inv` ;
  - avec `useJoinTable:false`, une colonne de jointure `<attr>_id` sur la table propriétaire (cas `createdBy` et `updatedBy`).
- **Côté propriétaire** : c'est l'attribut qui porte `inversedBy`. Le côté `mappedBy` lit la même table de jointure en sens inverse.
- **Colonnes d'ordre** :
  - présentes pour tout côté « to-many » (ordre des cibles vu depuis la source) ;
  - colonne d'ordre inverse pour les relations bidirectionnelles « many-to-any » ;
  - valeurs flottantes.
- **Écritures** :
  - forme courte (id, liste d'ids, documentId ou `null`) : traitée comme `set` ;
  - forme longue `{connect, disconnect, set}` ;
  - les éléments sont `{id | documentId, locale?, status?, position?: {before | after: id} | {start: true} | {end: true}}`.
- **Calcul des positions** :
  - les `connect` sont triés topologiquement selon leurs `before` et `after` ;
  - un doublon ou un cycle donne une `ValidationError` ;
  - l'ordre fractionnaire est recalculé entre voisins (milieu des deux valeurs d'ordre, en fin de liste max + 1) ;
  - le résultat doit être identique à `relations-orderer` de Strapi.
- **Cardinalités** : `connect` sur un oneToOne ou manyToOne remplace la cible existante. Sur oneToOne bidirectionnel, l'ancien lien inverse est cassé.
- **Populate** :
  - chargement par lots (une requête par relation et par niveau, `WHERE id IN (...)`, pas de N+1) ;
  - options par relation : `fields`, `filters`, `sort`, `populate` imbriqué, `count: true` (renvoie `{count: n}`) ;
  - sans `sort`, l'ordre suit la colonne d'ordre.
- **Suppression** : une entrée supprimée retire ses lignes de jointure (FK `ON DELETE CASCADE` sur les tables `_lnk`, à vérifier).

## Hors périmètre
- La résolution documentId vers id selon la locale et le status (`DOC-006`), et le recâblage à la publication (`DOC-002`).

## Critères d'acceptation
- [x] Pour chaque type de relation, sur la fixture `relations` : création, lecture des deux côtés, `set`, `connect` et `disconnect`, puis suppression. Le contenu des tables `_lnk`, colonnes d'ordre comprises, est identique à Strapi après le même scénario (`compat/querycases/relations--write.yaml`, 53 étapes, SQLite et Postgres : `make bench-compat FIXTURE=relations--write MODE=query`).
- [x] Banc : scénario de positions (`connect` avec `before`, `after`, `start` et `end`, déplacement d'une relation existante, ancre déconnectée dans le même payload). L'ordre et les valeurs d'ordre fractionnaires sont identiques à Strapi.
- [x] `connect` avec un cycle, un doublon ou une ancre absente en mode strict : même erreur, même message que Strapi (`exactError`). Le mode non strict place la relation en fin de liste.
- [x] Le populate de 3 niveaux sur 100 entrées s'exécute en une requête par relation et par niveau (`TestPopulateQueriesDoNotGrowWithEntries` : 5 requêtes).
- [x] `populate: {tags: {count: true}}` renvoie `{count: n}` comme Strapi, pour toutes les cardinalités.

## Réalisation
- `internal/db/query` : `em.go` (entity manager : `processData`, `Create`, `CreateMany`, `Update`, `UpdateMany`, `Delete`, `DeleteMany`, relations), `em_morph.go`, `orderer.go` (`relations-orderer` et ses tests portés), `populate.go`.
- Le populate s'exécute sur les lignes brutes, puis la conversion des champs a lieu une fois, comme chez Strapi : l'ordre des tables de liens passe avant le tri racine, et le DISTINCT inclut les colonnes d'ordre.
- Une écriture et ses relations s'exécutent dans une seule transaction (Strapi en utilise deux et annule à la main en cas d'échec) : voir `docs/divergences.md`.
- Banc : le mode `query` accepte des fichiers `isolated: true`, où grapi rejoue seed et scénario sur sa propre base ; documentId et horodatages sont alors masqués dans la comparaison.

## Notes techniques
- Porter les tests de `relations-orderer` de Strapi en tests de table : c'est la spécification la plus fiable de l'ordre.
- Les valeurs flottantes d'ordre s'accumulent. Strapi ne renormalise pas : on ne renormalise pas non plus, pour garder la compatibilité des données.

## Référence Strapi
- `core/database/src/metadata/relations.ts`
- `core/database/src/entity-manager/regular-relations.ts`, `relations-orderer.ts`, `index.ts`
- `core/database/src/query/helpers/populate/apply.ts`

## Repris d'autres tickets
- `DB-005` : les écritures de l'entity manager (`create`, `createMany`, `update`, `updateMany`, `delete`, `deleteMany`) avec valeurs par défaut (`documentId`, `publishedAt`), horodatages, conversions `toDB` (`internal/db/query/fields.go`) et rattachement des relations.
- `DB-003` : le repair `processUnidirectionalJoinTables` avec `cleanComponentJoinTable`, exécuté une seule fois au démarrage puis marqué dans le core store (`strapi_unidirectional-join-table-repair-ran`, avec `DB-010`). Sur une base créée par grapi, poser directement la marque, comme le ferait une base Strapi neuve.

## Dépendances
- `DB-002`, `DB-005`.

## Fichiers
- `internal/db/relations/store.go`, `orderer.go`, `populate.go`, `write.go`, `orderer_test.go`
