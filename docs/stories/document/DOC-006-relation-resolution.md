---
status: in-progress
---

# DOC-006 : Résolution des relations par documentId

## Contexte
En base, une relation relie des **lignes** (ids). En entrée, l'API accepte des documentIds. Il faut donc traduire documentId + locale + status en ids de lignes, et faire l'inverse en sortie.

## Objectif
Accepter toutes les formes d'entrée relationnelle de Strapi 5 et les résoudre vers les bonnes lignes.

## Périmètre
- Forme courte : un identifiant, une liste, ou `null` (traité comme `set`). Une valeur purement numérique est un `id`, sinon un `documentId`.
- Forme longue : `{connect, disconnect, set}` ; chaque élément peut porter `documentId` ou `id`, `locale`, `status`, `position: {before|after|start|end}`.
- Résolution de locale : si source et cible sont localisées, la locale cible est forcée à celle de la source ; si seule la cible l'est, `relation.locale` ou la locale de la source ; sinon aucune.
- Résolution de status : un brouillon se lie aux brouillons des cibles, la version publiée aux versions publiées (recâblage à la publication, `DOC-002`).
- `__type` pour les cibles morphTo, `__component` pour les zones dynamiques.
- Mode `api.documents.strictRelations` : en 5.59, il impose les relations et médias `required` sur les écritures hors brouillon (`relationRequiredFails` du validateur d'entités).

## Hors périmètre
- Le stockage et l'ordre (`DB-006`).

## Critères d'acceptation
- [ ] Les 4 formes (id, documentId, liste, longue) donnent le même état final que Strapi.
- [ ] `connect` avec `position.before` d'un documentId inexistant : même erreur que Strapi.
- [ ] Doublons et cycles de positions détectés (erreur).
- [ ] Banc de compatibilité sur un post lié à des thèmes et une catégorie, en FR et EN.

## Avancement
- Formes courtes et longues, `connect`/`disconnect`/`set`, positions (`before`, `after`, `start`, `end`, ancre inconnue), cibles `__type` des morphTo et relations entre types localisés couvertes par les bancs `relations` (78/78) et `i18n-dp` (92/92) en mode grapi, SQLite et Postgres, contrôle inverse compris.
- `api.documents.strictRelations` (`internal/document/validate.go`, `Service.StrictRelations`) : relations et médias `required` imposés hors brouillon, avec les règles de Strapi (absence en création, valeur qui vide le champ en mise à jour, `disconnect` seul sur un to-one, `connect: []` sans effet). Test `TestStrictRelations`.
- Reste : doublons et cycles de positions au banc, scénario FR/EN post, thèmes et catégorie.

## Notes techniques
- Cache par requête documentId vers ids (id-map) pour éviter N requêtes.

## Référence Strapi
- `core/core/src/services/document-service/transform/id-map.ts`, `id-transform.ts`
- `core/core/src/services/document-service/transform/relations/` (dont `utils/map-relation.ts`, `utils/i18n.ts`)

## Dépendances
- `DOC-001`, `DB-006`

## Fichiers
- `document/relations/idmap.go`, `document/relations/input.go`, `document/relations/locale.go`
