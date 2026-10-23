---
status: todo
---

# ADM-011 : Éditeur Markdown des champs `richtext`

> Option retenue : B (admin maison), voir `docs/decisions/D1-admin.md`. Conventions UI : voir `ADM-002`.

## Contexte
Dans l'admin Strapi, un champ `richtext` (« Rich text (Markdown) ») s'édite dans un vrai éditeur : barre d'outils, insertion d'images depuis la médiathèque, aperçu, plein écran. L'admin grapi n'affiche aujourd'hui qu'une zone de texte. dev-freelance écrit tous ses articles dans ces champs (`content`, `faq`, `tldr`, `description`, `details`, `callToAction`) : c'est le premier frein pour passer l'édition sur grapi.

## Objectif
Éditer un champ `richtext` dans grapi avec le même confort que dans Strapi, en produisant le même Markdown.

## Périmètre
- **Barre d'outils**, comme Strapi : titres (H1 à H6 dans un menu), gras, italique, souligné, barré, code en ligne et bloc de code, citation, listes à puces et numérotées, lien (texte et URL dans une petite modale), image depuis la médiathèque. Chaque bouton a une icône, un `title` et un raccourci clavier quand Strapi en a un (Ctrl/Cmd+B, I, U...).
- **Même Markdown que Strapi** pour chaque action : `**gras**`, `_italique_`, `<u>souligné</u>`, `~~barré~~`, `` `code` ``, `> citation`, `- élément`, `1. élément`, `[texte](url)`, `![alt](url)`, sur la sélection ou au curseur ; une action répétée sur une sélection déjà formatée l'enlève.
- **Image** : le bouton ouvre le sélecteur de la médiathèque (`ADM-012`, upload compris) ; l'image insérée utilise l'URL du fichier (ou son format `large` s'il existe, comme Strapi) et son texte alternatif.
- **Aperçu** : bascule Édition / Aperçu, rendu Markdown (GFM : tableaux, listes de tâches, liens automatiques), HTML brut désinfecté.
- **Plein écran** : éditeur et aperçu côte à côte sur desktop, empilés sur mobile ; Échap ou le bouton « Réduire » referme.
- **Confort** : hauteur ajustée au contenu avec un maximum, défilement interne, compteur de caractères si `maxLength` est fixé, indentation par Tab dans les listes et le code.
- **États** : lecture seule si le champ n'est pas modifiable (permissions par champ, `ADM-004`), erreur de validation sous l'éditeur.

## Hors périmètre
- Le champ `blocks` (`ADM-013`).
- Les custom fields d'éditeurs tiers (CKEditor...).

## Critères d'acceptation
- [ ] Pour chaque action de la barre d'outils, le Markdown produit est identique à celui de l'éditeur Strapi 5.59 sur la même sélection (cas listés dans un test, relevés une fois dans Strapi).
- [ ] Un article dev-freelance existant s'ouvre, s'édite et s'enregistre sans changer un octet du Markdown des parties non touchées.
- [ ] L'insertion d'une image depuis la médiathèque (fichier existant ou téléversé à la volée) produit `![alt](url)` au curseur.
- [ ] L'aperçu rend titres, listes, tableaux, code et images ; un `<script>` dans le contenu n'est pas exécuté.
- [ ] Clavier seul : tous les boutons sont atteignables, les raccourcis fonctionnent, le plein écran se ferme par Échap.
- [ ] À 375 px : barre d'outils sur plusieurs lignes ou en défilement horizontal interne, sans scroll de page ; plein écran empilé.

## Notes techniques
- Un éditeur de texte (CodeMirror 6 ou une `textarea` outillée) plutôt qu'un WYSIWYG : le format stocké reste le Markdown tel que l'auteur l'a écrit.
- Rendu de l'aperçu côté SPA (`marked` ou `markdown-it`) avec désinfection (DOMPurify).

## Référence Strapi
- `core/content-manager/admin/src/pages/EditView/components/FormInputs/Wysiwyg/` (barre d'outils, actions de formatage, aperçu, plein écran) ; code MIT.

## Dépendances
- `ADM-004`, `ADM-012`

## Fichiers
- `admin/src/pages/content-manager/fields.tsx`, `admin/src/pages/content-manager/MarkdownField.tsx`
