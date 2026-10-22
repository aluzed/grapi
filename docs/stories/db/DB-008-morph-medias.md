---
status: done
---

# DB-008 : Relations polymorphes (morph) et champs média

## Contexte
Les relations polymorphes permettent à une relation de cibler plusieurs types. Le cas d'usage principal est le champ **média** : un fichier (`plugin::upload.file`) est rattaché à n'importe quelle entrée via la relation morph `related`. Les tables `_mph` et le couple de colonnes `*_id` et `*_type` portent ce stockage.

## Objectif
Supporter `morphOne`, `morphMany`, `morphToOne` et `morphToMany`, ainsi que les attributs `media` qui reposent dessus, avec un stockage identique à Strapi.

## Périmètre
- **morphToOne** : colonnes `<attr>_id` et `<attr>_type` sur la table propriétaire (`target_id` et `target_type` selon le nommage `DB-002`).
- **morphToMany** : table `<table>_<attr>_mph`, avec les colonnes `<singular>_id`, `<attr>_id`, `<attr>_type`, `field` et `order`.
- **morphOne et morphMany** : côté inverse, avec `morphBy`. Ils lisent la table `_mph` de la cible en filtrant sur `<attr>_type = uid` et `field = <attr>`.
- **Champ `media`** :
  - c'est un morph vers `plugin::upload.file` (`morphBy: related`), stocké dans `files_related_mph` (nom réel à vérifier), avec les colonnes `file_id`, `related_id`, `related_type`, `field` et `order` ;
  - `multiple:false` donne au plus un fichier, `multiple:true` une liste ordonnée par `order` ;
  - `allowedTypes` est contrôlé à l'écriture (`DOC-003`) à partir du mime du fichier.
- **Écriture d'un média** : id de fichier, liste d'ids, ou `null`. Réécrire un champ média remplace l'ensemble, et l'ordre suit l'ordre de l'entrée.
- **Écriture d'un morphTo générique** : chaque élément porte `__type` (uid de la cible) et `id`.
- **Populate** :
  - `populate: {cover: true}` renvoie l'objet fichier complet (avec `formats`, `url`, etc.) ;
  - pour un morphTo, chaque élément renvoyé porte `__type` ;
  - fragments `on` pour les morphTo, comme pour les zones dynamiques.
- **Filtres** : sur un champ média (`filters[cover][mime][$startsWith]=image`), oui. Sur un morphTo, c'est interdit, comme Strapi (`API-005`).
- **Repair** : suppression des lignes `_mph` dont le `*_type` ne correspond plus à aucun content type existant (`DB-003`).

## Hors périmètre
- Le modèle File et l'upload (`MEDIA-001`).

## Critères d'acceptation
- [x] Fixture `relations` : morphToOne (`comment.subject`), morphOne (`writer.pinnedComment`), morphToMany (`post.mentions`, `set` puis `connect` positionné) et morphMany (`tag.mentionedIn`). Le contenu de la table `_mph` et des colonnes `target_id` et `target_type` est identique à Strapi (`relations--write.yaml`).
- [x] Champ média simple (`post.cover`) et multiple (`project.pictures`) : création, réordonnancement, `connect`, `disconnect`, vidage, suppression de l'entrée, et écriture depuis le fichier avec `__pivot`. Réponses (populate compris) et table `files_related_mph` identiques à Strapi (`devfreelance--media.yaml`, SQLite et Postgres).
- [x] Un fichier rattaché à deux entrées de types différents est listé dans les deux (`file-in-two-types`).
- [x] Après suppression d'un content type, le repair supprime ses lignes morph orphelines (`TestRemoveOrphanMorphTypes`).
- [ ] Lecture des couvertures sur une copie de la base dev-freelance : reporté à `DB-011`, qui ouvre la copie de la prod.

## Notes techniques
- La colonne `field` est indispensable : une même entrée peut avoir plusieurs champs média vers la même table `_mph`.
- Le populate d'un morphTo regroupe les cibles par `__type`, puis fait une requête par type.

## Référence Strapi
- `core/database/src/metadata/relations.ts` (morph)
- `core/database/src/entity-manager/morph-relations.ts` (ou l'équivalent dans `entity-manager/`)
- `core/upload/server/src/content-types/file.ts`, `core/upload/server/src/register.ts` (injection du champ `related`)

## Dépendances
- `DB-002`, `DB-006`.

## Fichiers
- `internal/db/relations/morph.go`, `internal/db/relations/media.go`, `internal/db/relations/morph_test.go`
