---
status: done
---

# DB-009 : Lifecycles de la base

## Contexte
Strapi émet des événements de cycle de vie au niveau des lignes : avant et après chaque création, mise à jour, suppression, lecture et comptage. C'est le mécanisme qu'utilisait dev-freelance pour ses slugs et ses hash (aujourd'hui centralisés dans un module abonné via `strapi.db.lifecycles.subscribe`). En Strapi 5, une publication **recrée** la ligne publiée, ce qui déclenche `beforeCreate` : ce comportement doit être reproduit.

## Objectif
Un bus de lifecycles par ligne, avec la même liste d'événements, la même forme d'événement et le même moment de déclenchement que Strapi.

## Périmètre
- **Événements** : `beforeCreate`, `afterCreate`, `beforeCreateMany`, `afterCreateMany`, `beforeUpdate`, `afterUpdate`, `beforeUpdateMany`, `afterUpdateMany`, `beforeDelete`, `afterDelete`, `beforeDeleteMany`, `afterDeleteMany`, `beforeCount`, `afterCount`, `beforeFindOne`, `afterFindOne`, `beforeFindMany`, `afterFindMany`.
- **Forme de l'événement** :
  - `action`, `model` (uid, table, attributs) ;
  - `params` (`data`, `where`, `select`, `populate`), **mutable** dans les hooks `before*` ;
  - `state` (map partagée entre le `before` et le `after` d'une même opération) ;
  - `result` (dans les `after*`).
- **Abonnement** :
  - `Subscribe(Subscriber{Models: []uid (vide pour tous), BeforeCreate: fn, ...})` ou une fonction générique `func(event)` ;
  - renvoie une fonction de désabonnement ;
  - ordre d'exécution : ordre d'abonnement.
- **Abonnés intégrés** :
  - timestamps : `createdAt` posé seulement s'il manque, `updatedAt` à chaque update ;
  - équivalent des fichiers `lifecycles.js` des content types, déclarés en Go par content type (`EXT-002`), puis en JS en phase 3 (`EXT-003`).
- **Déclenchement interne** : les opérations internes du Document Service (publish qui recrée la ligne publiée, synchronisation des relations, updates sans `name`) déclenchent bien les lifecycles. Les règles de dev-freelance en dépendent : slug du post recalculé à la publication, garde `data.name` sur les updates internes des thèmes.
- **Erreur dans un hook `before*`** : l'opération est annulée (la transaction est annulée par le Document Service). Une `ApplicationError` est remontée telle quelle au client.
- **Transaction** : les hooks reçoivent le `ctx` de la transaction en cours. Toute requête qu'ils font participe à cette transaction.

## Hors périmètre
- Les middlewares du Document Service (`DOC-004`), qui interviennent au niveau document et non ligne.

## Critères d'acceptation
- [x] Chaque événement est émis exactement une fois par opération, avec la bonne forme (tests unitaires sur un content type de test).
- [x] Un `beforeCreate` qui modifie `params.data.slug` change la valeur écrite.
- [x] `state` posé dans `beforeUpdate` est lu dans `afterUpdate` de la même opération.
- [x] Publication d'un document en draft & publish : `beforeCreate` est déclenché pour la ligne publiée. Scénario de caractérisation porté depuis dev-freelance : le slug publié est recalculé depuis le titre, et une publication de thème ne modifie pas le slug du brouillon.
- [x] Banc : les 21 cas de caractérisation de dev-freelance (slugs, hash d'abonné, thèmes) donnent les mêmes sorties qu'avec Strapi 5.59, une fois les règles portées en hooks Go.
- [x] Une erreur levée dans `beforeDelete` annule la suppression, et le client reçoit l'erreur au format Strapi.

## Avancement
- Fait : `internal/db/query/lifecycles.go` : les 18 événements, `params` modifiables, `state` par abonné entre before et after, abonné timestamps exécuté en premier comme chez Strapi, `create` et `delete` déclenchant `findOne` comme l'entity manager de Strapi. La publication déclenche `beforeCreate` pour la ligne publiée (`TestPublishRunsBeforeCreate`). Exposé par `grapi.App.OnRowCreate`, `OnRowUpdate`, `OnRowWrite`, `OnRowDelete`, `OnRow`.
- Règles de ligne de dev-freelance portées en hooks Go (dans le dépôt privé du projet : slugs, hash, slug préfixé par celui du parent sur `connect`), avec un port de `slugify` 1.6.6 vérifié contre le paquet npm (49 entrées). Banc REST `devfreelance_rules` : les 21 cas (85 requêtes, lectures du brouillon et de la version publiée) identiques à Strapi qui exécute les règles d'origine, sous SQLite et Postgres.

## Notes techniques
- Les `params` sont exposés comme des structures Go typées avec un accès dynamique (`Data map[string]any`). Les hooks Go des utilisateurs manipulent des maps, comme en JS.
- Éviter le coût des hooks quand personne n'est abonné : on vérifie la liste d'abonnés par modèle et par action avant de construire l'événement.

## Référence Strapi
- `core/database/src/lifecycles/` (`index.ts`, `subscribers/`)
- `core/core/src/utils/transform-content-types-to-models.ts` (branchement des `lifecycles.js`)
- Règles de ligne de dev-freelance (cas réel, dépôt privé)

## Dépendances
- `DB-001`, `DB-005`, `SCH-005`.

## Fichiers
- `internal/db/lifecycles/bus.go`, `event.go`, `timestamps.go`, `hook/lifecycle.go` (types publics), `*_test.go`
