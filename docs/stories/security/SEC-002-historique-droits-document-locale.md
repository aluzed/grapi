---
status: in-progress
---

# SEC-002 : Appliquer les droits par document et par langue à l'historique

Priorité : **Haute**. Source : [revue complémentaire du 29 octobre 2026](../../security-review-20261029.md).

## Contexte

Un éditeur limité à ses documents ou à une langue pouvait consulter, comparer et restaurer les versions d'autres documents ou langues via les identifiants numériques de l'historique.

## Objectif

En tant que responsable des contenus, je veux que l'historique respecte les permissions du document actuel afin de protéger les versions passées.

## Périmètre

- Appliquer les conditions de lecture et de langue à la liste, à la fiche et à chaque côté d'une comparaison.
- Vérifier les conditions de modification avant restauration sur le document actuel.

## Hors périmètre

Nettoyage des anciens champs, suivi dans `SEC-005`.

## Critères d'acceptation

- [ ] Un rôle limité à ses documents ne voit aucune version d'un autre auteur.
- [ ] Un rôle limité à une langue ne peut ni lire, ni comparer, ni restaurer une version d'une autre langue.
- [ ] Les versions interdites sont absentes des listes et les accès directs donnent 404, y compris via `against`.
- [ ] Les opérations autorisées et celles du super admin fonctionnent sur SQLite et PostgreSQL.

## Notes techniques

Contrôles `versionAllowed` présents dans les patches de l'historique et du diff. Régression existante : `TestAdminHistoryEnforcesDocumentAndLocalePermissions`. Les résultats sauvegardés concernent l'environnement local ; PostgreSQL reste à valider.

## Référence Strapi

Fonction grapi de content history (`XTR-003`), spécifiée sans lecture du code Enterprise Strapi.

## Dépendances

`AUTH-001`, `XTR-003`, `SEC-015`.

## Fichiers

- `internal/adminapi/history.go`
- `internal/adminapi/history_diff.go`
- `admin_history_security_test.go`

