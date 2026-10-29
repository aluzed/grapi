---
status: in-progress
---

# SEC-005 : Protéger les valeurs historiques des champs supprimés

Priorité : **Moyenne**. Source : [revue complémentaire du 29 octobre 2026](../../security-review-20261029.md).

## Contexte

Après suppression d'un attribut du schéma, sa valeur historique apparaissait dans `unknownFields` hors du nettoyage de `data`, y compris pour un lecteur sans droit sur ce champ.

## Objectif

En tant que responsable des données, je veux préserver la confidentialité des anciens champs lorsque le schéma évolue.

## Périmètre

- Réserver aux super admins les valeurs historiques sans schéma exploitable.
- Filtrer les noms de `missingFields` selon les permissions de lecture et conserver le nettoyage de `data`.

## Hors périmètre

Purge physique de l'historique et politique de rétention.

## Critères d'acceptation

- [ ] Un lecteur ordinaire ne reçoit aucune valeur d'un champ supprimé via `unknownFields`.
- [ ] `missingFields` ne révèle pas les noms d'attributs actuels illisibles.
- [ ] Un super admin conserve l'accès aux valeurs anciennes utiles à l'administration.
- [ ] Lecture, diff et restauration ne réintroduisent pas ces valeurs dans une réponse non autorisée.

## Notes techniques

Patch présent dans `getVersion`, avec filtrage des métadonnées. `TestAdminHistoryAfterFieldRemoval` contient une régression ; compléter la preuve sur les autres chemins avant clôture.

## Référence Strapi

Content history propre à grapi ; ne pas consulter les sources Enterprise de Strapi.

## Dépendances

`SEC-002`, `AUTH-001`, `XTR-003`, `SEC-015`.

## Fichiers

- `internal/adminapi/history.go`
- `internal/adminapi/history_diff.go`
- `admin_test.go`

