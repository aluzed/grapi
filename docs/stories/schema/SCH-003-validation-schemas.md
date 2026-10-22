---
status: done
---

# SCH-003 : Validation de cohérence des schémas

## Contexte
Une erreur de schéma détectée au démarrage coûte une ligne de log. La même erreur découverte à l'exécution produit des données corrompues. Strapi valide en partie au chargement et en partie au moment de l'écriture. grapi valide **tout** au chargement, avant toute synchronisation de la base (`DB-003`).

## Objectif
Valider l'ensemble des content types et des composants chargés, en un seul passage, et renvoyer toutes les erreurs d'un coup avec leur chemin.

## Périmètre
- **Unicité** : `singularName` et `pluralName` uniques dans toutes les APIs et tous les plugins, `collectionName` unique, `uid` unique. Les noms sont en kebab-case.
- **Relations** :
  - `target` existe ;
  - `inversedBy` et `mappedBy` pointent vers un attribut existant de la cible, qui pointe en retour sur nous ;
  - la combinaison des types est cohérente : oneToOne avec oneToOne, oneToMany avec manyToOne, manyToMany avec manyToMany ;
  - oneWay et manyWay n'ont pas d'inverse ;
  - morphToOne et morphToMany n'ont pas de `target`, morphOne et morphMany ont `morphBy`.
- **uid** : `targetField` existe, est de type `string` ou `text` et ne vient pas d'un composant.
- **enumeration** : valeurs uniques et non vides. Strapi refuse les valeurs qui commencent par un chiffre dans le CTB : vérifier le comportement exact du loader, puis le reproduire.
- **Bornes** : `min <= max` et `minLength <= maxLength`, chacune sur un type compatible.
- **default** : compatible avec le type (enum dans la liste, nombre dans les bornes).
- **media** : `allowedTypes` dans `images|videos|files|audios`.
- **Composants** : références valides, pas de cycle, pas de zone dynamique dans un composant (`SCH-002`).
- **i18n** : un attribut ne peut pas être `localized:true` si le type ne l'est pas.
- **Noms réservés** (`SCH-001`) et longueur des identifiants : un avertissement si le nom dépasse 55 caractères, car il sera tronqué en base (`DB-002`).
- Rapport : liste d'erreurs `{uid, attribute, code, message}`. Le démarrage échoue s'il y a au moins une erreur, et affiche toutes les erreurs.

## Hors périmètre
- La validation des valeurs des entrées (`DOC-003`).

## Critères d'acceptation
- [ ] Une batterie de schémas invalides (au moins 30 cas, un par règle) produit chacun le bon code d'erreur (tests en table).
- [ ] Un projet avec 3 erreurs indépendantes affiche les 3 erreurs dans un seul rapport.
- [ ] Les schémas des fixtures et de dev-freelance passent sans erreur.
- [ ] Pour chaque règle reprise de Strapi, un cas du banc vérifie que Strapi 5.59 refuse aussi le schéma (démarrage en erreur). Les règles plus strictes que Strapi sont listées dans `docs/divergences.md`.

## Notes techniques
- Les règles plus strictes que Strapi (cycle de composants, par exemple) sont acceptables si elles ne refusent aucun schéma que Strapi accepte et qui fonctionne réellement. Sinon, il faut un mode `--lenient`.
- Garder les codes d'erreur stables : ils seront affichés dans l'admin CTB (`ADM-006`).

## Référence Strapi
- `core/core/src/domain/content-type/validator.ts`
- `core/database/src/metadata/relations.ts` (cohérence des relations)
- `core/content-type-builder/server/src/controllers/validation/` (règles côté CTB)

## Dépendances
- `SCH-001`, `SCH-002`, `SCH-005`.

## Fichiers
- `schema/validate.go`, `schema/validate_relations.go`, `schema/validate_test.go`, `docs/divergences.md`
