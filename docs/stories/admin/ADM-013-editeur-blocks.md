---
status: done
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
- [x] Un document `blocks` créé dans Strapi s'ouvre et s'enregistre dans grapi sans changer son JSON.
- [x] Pour une série d'actions définie (titre, liste imbriquée sur 3 niveaux, lien, image, code), le JSON produit est identique à celui de Strapi 5.59.
- [x] Clavier seul : tout est faisable, y compris l'indentation des listes.
- [x] À 375 px : barre d'outils utilisable, pas de scroll horizontal de page.

## Avancement
- Éditeur Slate (mêmes versions que Strapi 5.59 : `slate` 0.94.1, `slate-react` 0.98.3, `slate-history` 0.93.0), dont le document est le JSON stocké : un document s'ouvre tel quel et n'est réécrit que s'il est modifié (`blocks/editor.test.ts`, « a document is left untouched »), vérifié dans le navigateur en enregistrant une entrée sans toucher au champ.
- Logique portée depuis le code MIT de Strapi (`BlocksInput/`) dans `admin/src/pages/content-manager/blocks/editor.ts`, sans React : conversions de blocs, Entrée (deux fois pour sortir d'une citation ou d'un bloc de code), Retour arrière, Tab (liste imbriquée avec `indentLevel`), raccourcis Markdown (`#` à `######`, `-`, `*`, `+`, `1.`, `>`, ```` ``` ````, `![`), marques et raccourcis Ctrl/Cmd+B, I, U, E et Maj+S, liens (`rel`, `target`), images (champs de Strapi, URL préfixée par l'origine comme `prefixFileUrlWithBackendUrl`), déplacement Ctrl/Cmd+Maj+flèches. 12 tests sur une série d'actions (titre, liste sur 3 niveaux, lien, image, code, marques).
- Les JSON attendus des tests sont dérivés du code de Strapi, pas rejoués dans son admin.
- Champ chargé à la demande (Slate n'alourdit pas le reste de l'admin), plein écran, poignée de glisser-déposer par bloc, annonce des déplacements aux lecteurs d'écran.

## Référence Strapi
- `core/content-manager/admin/src/pages/EditView/components/FormInputs/BlocksInput/` (code MIT).

## Dépendances
- `ADM-004`, `ADM-012`

## Fichiers
- `admin/src/pages/content-manager/BlocksField.tsx`, `admin/src/pages/content-manager/blocks/`
