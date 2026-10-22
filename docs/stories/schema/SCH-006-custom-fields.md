---
status: done
---

# SCH-006 : Custom fields

## Contexte
Un custom field est un champ dont l'interface d'édition est spécifique (par exemple un color-picker), mais qui est stocké comme un type de base. Dans un schéma, il apparaît sous la forme `{"type":"customField","customField":"plugin::color-picker.color","options":{...}}`. Au chargement, Strapi le remplace par son type sous-jacent tout en gardant l'identifiant du custom field.

## Objectif
Permettre d'enregistrer des custom fields (plugins intégrés, mode framework Go, plugins) et de les résoudre au chargement des schémas.

## Périmètre
- `customFields.Register(CustomField{Name, Plugin, Type, InputSize?, Options?})`.
  - L'uid vaut `plugin::<plugin>.<name>`, ou `global::<name>` sans plugin.
  - `Type` doit être l'un des types de base autorisés par Strapi : `biginteger`, `boolean`, `date`, `datetime`, `decimal`, `email`, `enumeration`, `float`, `integer`, `json`, `password`, `richtext`, `string`, `text`, `time`, `uid`. Vérifier la liste exacte (16 types).
- **Résolution au chargement** : l'attribut prend `type = Type` sous-jacent et garde `customField = uid` et `options`, pour l'admin. La validation des valeurs et le stockage utilisent ensuite le type sous-jacent.
- **Custom field inconnu dans un schéma** : erreur au démarrage, avec l'uid manquant et le plugin attendu.
- Sérialisation : le CTB (`ADM-006`) réécrit la forme `customField` d'origine, jamais le type résolu.
- **Custom field intégré** : `plugin::color-picker.color` (type `string`, avec validation regex `#RRGGBB` côté admin), en référence d'implémentation.
- Exposition à l'admin : liste des custom fields enregistrés (uid, type, options) pour `ADM-010`.

## Hors périmètre
- Le composant d'UI du custom field (`ADM-010`).

## Critères d'acceptation
- [ ] Un schéma qui utilise `plugin::color-picker.color` se charge, se stocke en `varchar` et se relit tel quel.
- [ ] L'enregistrement d'un custom field de type `relation` ou `media` est refusé.
- [ ] Le marshal du schéma conserve `type: "customField"` et l'uid d'origine (aller-retour identique).
- [ ] Un schéma Strapi qui référence un custom field non enregistré donne une erreur qui cite l'uid.
- [ ] Cas du banc : création et lecture d'une entrée avec un champ couleur, réponse identique à Strapi.

## Notes techniques
- La résolution se fait entre le chargement (`SCH-004`) et la validation (`SCH-003`) : la validation voit le type sous-jacent, plus l'information du custom field.
- Garder l'objet attribut d'origine dans `Attribute.Source()` pour l'aller-retour.

## Référence Strapi
- `core/core/src/registries/custom-fields.ts`
- `core/core/src/utils/convert-custom-field-type.ts`
- `plugins/color-picker/` (exemple de référence)

## Dépendances
- `SCH-001`, `SCH-005`.

## Fichiers
- `internal/registry/customfields.go`, `schema/attribute_customfield.go`, `internal/plugins/colorpicker/`
