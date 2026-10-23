---
status: todo
---

# ADM-014 : Champ relation abouti

> Option retenue : B (admin maison), voir `docs/decisions/D1-admin.md`. Conventions UI : voir `ADM-002`.

## Contexte
Le champ relation de grapi permet de rechercher, ajouter, retirer et réordonner les cibles. Chez Strapi, il montre aussi le statut de chaque cible, charge la liste au défilement, se réordonne au glisser-déposer et ouvre la cible sans quitter le document (`relationOpenMode`). Restes notés dans `ADM-004` : statut des cibles, relations morph.

## Objectif
Gérer les relations d'un document comme dans Strapi.

## Périmètre
- Sélecteur avec recherche sur le champ principal, chargement progressif au défilement, cibles déjà liées exclues.
- Badge de statut de chaque cible (Brouillon, Publié, Modifié) et locale.
- Réordonnancement au glisser-déposer et au clavier, positions envoyées comme Strapi (`connect` avec `position`) ou en `set`.
- Ouvrir une cible dans une modale ou une nouvelle page selon `relationOpenMode` (`ADM-005`), avec retour au document sans perte.
- « Créer une entrée » de la cible depuis le champ, liée à l'enregistrement.
- Listes longues : « Afficher plus » au-delà de 10 cibles, comme Strapi.

## Hors périmètre
- Les relations morph (ticket à part si un projet en a besoin).

## Critères d'acceptation
- [ ] Une relation vers 200 cibles se charge par pages au défilement sans ralentir la saisie.
- [ ] Le badge de statut d'une cible suit sa publication.
- [ ] Ouvrir une cible en modale, la modifier, revenir : le document en cours garde ses modifications.
- [ ] Réordonner à la souris et au clavier donne le même ordre, relu par l'API REST.
- [ ] À 375 px : liste et actions empilées, modale en plein écran.

## Référence Strapi
- `core/content-manager/admin/src/pages/EditView/components/FormInputs/Relations/` (code MIT).

## Dépendances
- `ADM-004`, `ADM-005`

## Fichiers
- `admin/src/pages/content-manager/RelationField.tsx`
