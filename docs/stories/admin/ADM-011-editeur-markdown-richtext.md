---
status: done
---

# ADM-011 : Éditeur Markdown des champs `richtext`

> Option retenue : B (admin maison), voir `docs/decisions/D1-admin.md`. Conventions UI : voir `ADM-002`.

## Contexte
Dans l'admin Strapi, un champ `richtext` (« Rich text (Markdown) ») s'édite dans un vrai éditeur : barre d'outils, insertion d'images depuis la médiathèque, aperçu, plein écran. L'admin grapi n'affiche aujourd'hui qu'une zone de texte. Les projets qui écrivent leurs articles en Markdown en dépendent : c'est le premier frein pour passer l'édition sur grapi.

## Objectif
Éditer un champ `richtext` dans grapi avec le même confort que dans Strapi, en produisant le même Markdown.

## Périmètre
- **Barre d'outils**, comme Strapi : titres (H1 à H6 dans un menu), gras, italique, souligné, barré, listes à puces et numérotées, bloc de code, image depuis la médiathèque, lien, citation. Chaque bouton a une icône, un `title` et un `aria-label` (libellé visible sur grand écran). Raccourcis Ctrl/Cmd+B, I et U en plus (Strapi n'en a pas), qui écrivent le même Markdown que les boutons.
- **Même Markdown que Strapi** (relevé dans son code, `Wysiwyg/utils/utils.js`) : `**gras**`, `_italique_`, `<u>souligné</u>`, `~~barré~~`, `[texte](link)`, bloc de code et citation (`>texte`) sur une nouvelle ligne après la ligne courante, listes `- ` et `1. ` (une action répétée sur une sélection de liste l'enlève), titres qui remplacent le titre existant. Sans sélection, le texte d'exemple (`Bold`, `Code`...) est inséré et sélectionné. Comme Strapi, une action répétée sur une sélection déjà formatée la reformate (pas de bascule), et il n'y a pas de code en ligne. Pour un lien, la cible `link` est sélectionnée, prête à être remplacée par l'URL (pas de modale, le Markdown reste celui de Strapi).
- **Entrée** prolonge une liste ou une citation et renumérote les éléments suivants, comme l'éditeur Strapi ; **Tab** indente dans une liste ou un bloc de code et quitte le champ ailleurs.
- **Image** : le bouton ouvre le sélecteur de la médiathèque (`ADM-012`, upload compris) ; comme Strapi, chaque fichier choisi donne `![alt](url)` (lien `[alt](url)` pour un fichier qui n'est pas une image), avec l'URL du fichier et le texte alternatif ou à défaut le nom, un fichier par ligne.
- **Aperçu** : bascule Markdown / Aperçu, rendu par markdown-it avec les options et extensions de l'aperçu Strapi (tableaux, liens automatiques, notes de bas de page, emoji...) plus les listes de tâches ; HTML brut désinfecté par DOMPurify. Le moteur de rendu se charge au premier aperçu.
- **Plein écran** : éditeur et aperçu côte à côte sur desktop, empilés sur mobile ; Échap ou le bouton « Réduire » referme.
- **Confort** : hauteur ajustée au contenu avec un maximum, défilement interne, compteur de caractères si `maxLength` est fixé ; les actions passent par les commandes d'édition du navigateur, donc Ctrl/Cmd+Z les annule.
- **États** : lecture seule si le champ n'est pas modifiable (permissions par champ, `ADM-004`), erreur de validation sous l'éditeur.

## Hors périmètre
- Le champ `blocks` (`ADM-013`).
- Les custom fields d'éditeurs tiers (CKEditor...).

## Critères d'acceptation
- [x] Pour chaque action de la barre d'outils, le Markdown produit est identique à celui de l'éditeur Strapi 5.59 sur la même sélection (cas listés dans `admin/src/lib/markdown.test.ts`, relevés dans le code de l'éditeur Strapi).
- [x] Un article existant s'ouvre, s'édite et s'enregistre sans changer un octet du Markdown des parties non touchées.
- [x] L'insertion d'une image depuis la médiathèque (fichier existant ou téléversé à la volée) produit `![alt](url)` au curseur.
- [x] L'aperçu rend titres, listes, tableaux, code et images ; un `<script>` dans le contenu n'est pas exécuté.
- [x] Clavier seul : tous les boutons sont atteignables, les raccourcis fonctionnent, le plein écran se ferme par Échap.
- [x] À 375 px : barre d'outils sur plusieurs lignes ou en défilement horizontal interne, sans scroll de page ; plein écran empilé.

## Notes techniques
- Une `textarea` outillée plutôt qu'un WYSIWYG : le format stocké reste le Markdown tel que l'auteur l'a écrit. Les actions sont des fonctions pures (`admin/src/lib/markdown.ts`) sur le texte et la sélection.
- Aperçu : `admin/src/lib/markdown-render.ts` (markdown-it et DOMPurify), chargé à la demande.

## Référence Strapi
- `core/content-manager/admin/src/pages/EditView/components/FormInputs/Wysiwyg/` (barre d'outils, actions de formatage, aperçu, plein écran) ; code MIT.

## Dépendances
- `ADM-004`, `ADM-012`

## Fichiers
- `admin/src/pages/content-manager/fields.tsx`, `admin/src/pages/content-manager/MarkdownField.tsx`, `admin/src/lib/markdown.ts`, `admin/src/lib/markdown-render.ts`
