---
status: done
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
- [x] Une relation vers 200 cibles se charge par pages au défilement sans ralentir la saisie.
- [x] Le badge de statut d'une cible suit sa publication.
- [x] Ouvrir une cible en modale, la modifier, revenir : le document en cours garde ses modifications.
- [x] Réordonner à la souris et au clavier donne le même ordre, relu par l'API REST.
- [x] À 375 px : liste et actions empilées, modale en plein écran.

## Avancement
- API admin : `GET /content-manager/relations/{uid}/{field}` exclut les cibles déjà liées (`idsToOmit`), relit des cibles par `documentIds` et donne `status` (brouillon, publié, modifié) et `locale` de chaque cible (`TestAdminRelations`). `settings.relationOpenMode` (`modal`, `newTab`, `page`, comme Strapi) est exposé et réglable dans la configuration de la vue.
- Correction au passage : la liste du Content Manager, le sélecteur de relations et l'accueil passaient `page` et `pageSize` au Document Service, qui (comme celui de Strapi) ne lit que `start` et `limit` : toutes les lignes revenaient. Testé sur 15 entrées.
- Champ : 10 cibles puis « Afficher plus », badge de statut et locale relus sur le serveur, glisser-déposer et flèches, sélecteur paginé au défilement (vérifié sur 200 cibles), « Créer une entrée » et ouverture d'une cible dans une fenêtre qui embarque l'admin (`?embed=1`) : la cible enregistrée prévient le champ (`postMessage`, même origine), le document en cours garde ses modifications.
- Ordre enregistré en `set`, relu par l'API REST après un glisser-déposer et un déplacement au clavier.

## Référence Strapi
- `core/content-manager/admin/src/pages/EditView/components/FormInputs/Relations/` (code MIT).

## Dépendances
- `ADM-004`, `ADM-005`

## Fichiers
- `admin/src/pages/content-manager/RelationField.tsx`
