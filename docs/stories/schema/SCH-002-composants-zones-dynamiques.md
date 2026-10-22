---
status: done
---

# SCH-002 : Composants et zones dynamiques (schéma)

## Contexte
Un composant est une structure réutilisable, rangée par catégorie (`src/components/<catégorie>/<nom>.json`). Une zone dynamique est une liste ordonnée de composants de types différents. Ce ticket couvre leur modèle. La persistance relève de `DB-007`.

## Objectif
Représenter les composants et les attributs `component` et `dynamiczone`, avec leurs contraintes, dans le package `schema`.

## Périmètre
- `Component` :
  - uid `<catégorie>.<nom>` (par exemple `shared.seo`), `category`, `collectionName` (par défaut `components_<catégorie>_<nom_pluriel>`, à confirmer sur la référence) ;
  - `info{displayName, icon, description}`, `attributes` (ordonnés), `options`.
- Un composant n'a **ni documentId, ni timestamps, ni publishedAt, ni createdBy** : seulement `id`.
- Attribut `component` : `component` (uid), `repeatable` (bool), `min` et `max` (si repeatable), `required`.
- Attribut `dynamiczone` : `components[]` (liste d'uids), `min`, `max`, `required`.
- Composants imbriqués : un composant peut contenir des attributs `component` (profondeur non bornée par Strapi). En revanche, **une zone dynamique est interdite dans un composant**.
- Médias et relations dans un composant : autorisés (relations sans `inversedBy` ni `mappedBy`, c'est-à-dire unidirectionnelles).
- Clé de discrimination côté API : `__component` (uid du composant) sur chaque élément d'une zone dynamique.
- Registre des catégories (liste dérivée) pour l'admin (`ADM-006`).

## Hors périmètre
- Le stockage `_cmps` (`DB-007`), le populate (`API-004`), la validation des valeurs (`DOC-003`).

## Critères d'acceptation
- [ ] Les composants des fixtures (`components`, 3 niveaux d'imbrication, répétables et zone dynamique) se parsent et se resérialisent à l'identique.
- [ ] Une zone dynamique déclarée dans un composant est rejetée (erreur avec le chemin `composant.attribut`).
- [ ] Une référence à un composant inexistant (`component` ou `components[]`) est rejetée par `SCH-003`, avec le nom attendu.
- [ ] Un cycle de composants (A contient B qui contient A) est détecté et rejeté.
- [ ] `collectionName` dérivé coïncide avec celui de la référence Strapi sur les fixtures.
- [ ] `min` ou `max` sur un composant non répétable provoque une erreur de schéma.

## Notes techniques
- Le cycle se détecte par un DFS sur le graphe composant vers composant. Strapi échoue à l'exécution ; grapi échoue au chargement, avec un message explicite (une amélioration acceptée).
- Les relations depuis un composant ne peuvent pas être bidirectionnelles : le vérifier ici plutôt qu'en base.

## Référence Strapi
- `core/types/src/schema/attribute/definitions/component.ts`, `dynamic-zone.ts`
- `core/core/src/loaders/components.ts`
- `core/core/src/utils/transform-content-types-to-models.ts` (forme des composants vus par la base)

## Dépendances
- `SCH-001`.

## Fichiers
- `schema/component.go`, `schema/attribute_component.go`, `schema/attribute_dynamiczone.go`, `schema/testdata/components/`
