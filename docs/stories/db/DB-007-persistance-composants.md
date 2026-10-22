# DB-007 : Persistance des composants et zones dynamiques

## Contexte
Chez Strapi, chaque composant a sa propre table (avec `id` seulement). Il est relié à son parent par une table `<collection>_cmps` qui sert à la fois aux composants simples, aux composants répétables et aux zones dynamiques. À l'écriture, Strapi supprime puis recrée les composants du parent.

## Objectif
Lire, écrire, peupler et filtrer les composants et les zones dynamiques, avec un stockage identique à Strapi.

## Périmètre
- **Tables** :
  - `components_<catégorie>_<nom>` (nom exact via `DB-002`) ;
  - table de liaison `<collection parent>_cmps`, avec les colonnes `entity_id`, `cmp_id`, `component_type`, `field` et `order` (flottant) ;
  - contrainte unique sur `(entity_id, cmp_id, field, component_type)`, FK `entity_id` en `ON DELETE CASCADE`.
- **Écriture** (création ou mise à jour d'un parent) :
  - pour chaque champ composant présent dans l'entrée, les composants existants du champ sont supprimés (récursivement pour les composants imbriqués), puis les nouveaux sont créés, avec un `order` de 1 à n ;
  - un champ absent de l'entrée n'est pas touché ;
  - une valeur `null` vide le champ.
- **Zone dynamique** : chaque élément porte `__component`, qui est écrit dans `component_type`. Un `__component` qui n'est pas dans la liste autorisée donne une `ValidationError`.
- **Relations et médias dans les composants** : écrits après la création de la ligne du composant, avec les mécanismes de `DB-006` et `DB-008`.
- **Populate** :
  - `populate: {seo: true}`, ou `*` ;
  - imbriqué : `populate: {blocks: {populate: '*'}}` ;
  - zone dynamique avec fragments : `populate: {blocks: {on: {'shared.quote': {fields: [...]}, 'shared.media': {populate: '*'}}}}`.
  - Chaque élément d'une zone dynamique est renvoyé avec `__component` et `id`.
- **Filtres** : filtrer par champ de composant (`filters[seo][metaTitle][$contains]=x`), par jointure sur `_cmps`. Le filtrage par zone dynamique est **interdit** (comme Strapi, voir `API-005`).
- **Suppression du parent** : suppression en cascade des lignes `_cmps` et des lignes des tables de composants (Strapi supprime explicitement les composants, à vérifier).
- **Clone** (utilisé par `DOC-001` clone et `DOC-002` publish) : copie profonde des composants avec de nouveaux ids.

## Hors périmètre
- La validation des valeurs (`DOC-003`) : min et max des répétables, `required`.

## Critères d'acceptation
- [ ] Fixture `components` : création avec 3 niveaux d'imbrication, un répétable de 5 éléments et une zone dynamique de 4 éléments de 3 types. Le contenu SQL des tables `_cmps` et des composants est identique à celui de Strapi après le même scénario, aux ids près.
- [ ] Mise à jour d'un seul champ composant : les autres champs composants du parent gardent leurs ids.
- [ ] Banc : populate `*`, populate imbriqué et fragments `on` donnent des réponses identiques à Strapi.
- [ ] Après suppression d'un parent, aucune ligne de composant orpheline ne reste (comptage SQL).
- [ ] `filters[blocks][...]` sur une zone dynamique renvoie la même erreur que Strapi.

## Notes techniques
- La stratégie « supprimer puis recréer » change les ids des composants à chaque mise à jour : c'est le comportement de Strapi et il est attendu. Ne pas « optimiser » en diff, sinon les ids divergent par rapport au banc.
- Les écritures de composants en masse doivent être groupées (insert multi-lignes), sinon une zone dynamique de 50 blocs fait 150 requêtes.

## Référence Strapi
- `core/core/src/services/document-service/components.ts`
- `core/core/src/utils/transform-content-types-to-models.ts` (tables `_cmps`)
- `core/database/src/query/helpers/populate/` (fragments `on`)

## Dépendances
- `SCH-002`, `DB-002`, `DB-005`, `DB-006`.

## Fichiers
- `internal/db/components/store.go`, `write.go`, `populate.go`, `clone.go`, `*_test.go`
