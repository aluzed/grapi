---
status: todo
---

# ADM-015 : Vue édition au niveau de Strapi (panneau, états, garde-fous)

> Option retenue : B (admin maison), voir `docs/decisions/D1-admin.md`. Conventions UI : voir `ADM-002`.

## Contexte
Au-delà des champs, la vue édition de Strapi a un panneau latéral (actions et informations), prévient avant de perdre des modifications et rend l'état du document lisible. L'admin grapi a les actions en tête de page, sans ces garde-fous.

## Objectif
Une vue édition aussi agréable et sûre que celle de Strapi pour écrire au quotidien.

## Périmètre
- **Panneau latéral** (desktop) : statut (Brouillon, Publié, Modifié), « Enregistrer », « Publier », « Plus » (dépublier, abandonner le brouillon, supprimer), locale ; bloc « Informations » : créé et modifié le, par qui. Sur mobile, ces éléments passent en tête, empilés, boutons en pleine largeur.
- **Modifications non enregistrées** : indicateur visible, confirmation avant de quitter la page ou de changer de locale, « Enregistrer » désactivé sans modification.
- **Raccourcis** : Ctrl/Cmd+S enregistre.
- **Erreurs** : récapitulatif en tête quand la validation échoue, avec un lien vers chaque champ fautif.
- **Aides des champs** : description, requis, longueur min/max et compteur, comme Strapi.

## Critères d'acceptation
- [ ] Quitter une vue modifiée (lien, retour, changement de locale) demande confirmation ; après enregistrement, plus de confirmation.
- [ ] Ctrl/Cmd+S enregistre le document.
- [ ] Une publication refusée liste les champs fautifs en tête et chaque lien amène au champ.
- [ ] « Informations » affiche les dates et l'auteur, identiques à l'API admin.
- [ ] À 375 px : tout est empilé, boutons en pleine largeur, aucun scroll horizontal.

## Référence Strapi
- `core/content-manager/admin/src/pages/EditView/components/Panels.tsx`, `DocumentActions.tsx`, `Header.tsx` (code MIT).

## Dépendances
- `ADM-004`

## Fichiers
- `admin/src/pages/content-manager/EditView.tsx`
