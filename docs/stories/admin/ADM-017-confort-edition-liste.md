---
status: in-progress
---

# ADM-017 : Confort de la vue d'édition et de la liste

> Conventions UI : voir `ADM-002`.

## Contexte
La recette sur dev-freelance (`ADM-016`) montre une vue d'édition indigeste : les champs se suivent dans une colonne étroite, sous leur nom technique (`hideOnHomepage`, `generateTldr`). Les cases à cocher sont dispersées et l'une déborde de la carte. Les relations sont tassées, et les actions se perdent dans la colonne de droite.

## Objectif
Un formulaire qu'on lit d'un coup d'œil et qu'on enregistre sans chercher le bouton, sur desktop comme sur mobile.

## Périmètre
- Champs rangés par nature, dans l'ordre configuré :
  - « Informations générales » : champs courts sur une grille repackée ;
  - « Contenu » : Markdown et blocks ;
  - une carte par composant ou zone dynamique ;
  - « Médias » : média multiple ;
  - « Avancé » : JSON, replié.
- Colonne de droite : « Publication » (statut, locale, dates, actions secondaires), « Médias » (média simple), « Relations », « Options » (booléens en interrupteurs).
- En-tête collant avec le titre, le statut et les boutons Publier et Enregistrer ; sur mobile, une barre Enregistrer en bas dès qu'il y a des changements.
- Le menu des types de contenu disparaît pendant l'édition d'une entrée, pour rendre la largeur au formulaire.
- Libellés lisibles quand aucun n'est configuré.
- Relation, média et Markdown s'adaptent à la largeur de leur colonne (container queries), pas à celle de l'écran.
- Liste :
  - ligne entière cliquable ;
  - cellules sur une ligne avec le texte complet au survol ;
  - colonne principale plus large ;
  - booléens en badges.

## Critères d'acceptation
- [x] Sur un article dev-freelance, aucun champ technique n'apparaît sous son nom brut, aucun élément ne déborde de sa carte.
- [x] Enregistrer et Publier restent visibles en faisant défiler le formulaire (desktop), Enregistrer en bas d'écran sur mobile quand il y a des changements.
- [x] Une relation simple remplie n'affiche plus « Créer une entrée » ; ses boutons tiennent sur la ligne de la relation dans la colonne de droite.
- [x] Tests : regroupement (`sections.test.ts`) et libellés (`humanize.test.ts`).
- [ ] Validation par l'utilisateur sur l'instance locale.

## Dépendances
- `ADM-015`, `ADM-016`
