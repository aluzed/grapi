---
status: todo
---

# ADM-013 : Éditeur Blocks complet

> Option retenue : B (admin maison), voir `docs/decisions/D1-admin.md`. Conventions UI : voir `ADM-002`.

## Contexte
Le champ `blocks` de Strapi est un éditeur visuel (Slate) : on écrit dans le rendu final, avec une barre d'outils et un menu de blocs. L'admin grapi a un éditeur par blocs fonctionnel mais sommaire (reste de `ADM-004` : pas de WYSIWYG, pas de listes imbriquées).

## Objectif
Un éditeur `blocks` équivalent à celui de Strapi, qui produit le même JSON.

## Périmètre
- Édition en place (rendu WYSIWYG) : paragraphe, titres 1 à 6, listes à puces et numérotées imbriquées, citation, code avec langage, image (médiathèque, `ADM-012`), lien.
- Marques : gras, italique, souligné, barré, code ; raccourcis clavier et raccourcis Markdown à la frappe (`# `, `- `, `1. `, `> `, ``` ``` ```).
- Menu de type de bloc, déplacement d'un bloc (glisser et clavier), suppression, plein écran.
- JSON identique à Strapi pour les mêmes actions (structure `type`, `children`, `format`, `indentLevel`, `image`...).

## Hors périmètre
- Les blocs personnalisés.

## Critères d'acceptation
- [ ] Un document `blocks` créé dans Strapi s'ouvre et s'enregistre dans grapi sans changer son JSON.
- [ ] Pour une série d'actions définie (titre, liste imbriquée sur 3 niveaux, lien, image, code), le JSON produit est identique à celui de Strapi 5.59.
- [ ] Clavier seul : tout est faisable, y compris l'indentation des listes.
- [ ] À 375 px : barre d'outils utilisable, pas de scroll horizontal de page.

## Référence Strapi
- `core/content-manager/admin/src/pages/EditView/components/FormInputs/BlocksInput/` (code MIT).

## Dépendances
- `ADM-004`, `ADM-012`

## Fichiers
- `admin/src/pages/content-manager/BlocksField.tsx`
