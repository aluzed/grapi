---
status: todo
---

# SEC-010 : Restreindre les modifications sensibles des utilisateurs finaux

Priorité : **Haute**. Source : [revue complémentaire du 29 octobre 2026](../../security-review-20261029.md).

## Contexte

La permission `user.update` donne accès à une route capable de modifier des champs sensibles, dont le rôle, sur l'utilisateur ciblé. Une permission d'action ne limite pas l'appelant à son propre compte.

## Objectif

En tant que responsable applicatif, je veux contrôler la cible et les champs modifiables afin d'empêcher une élévation de privilèges.

## Périmètre

- Auditer les droits Public, Authenticated et les rôles custom sur les routes utilisateurs.
- Retirer les permissions inutiles ; fournir une route ou une policy adaptée si l'édition de profil est nécessaire.
- Revoir aussi les écritures publiques sensibles signalées par le premier audit, notamment `subscriber.update`, si elles existent dans le projet cible.

## Hors périmètre

Changement silencieux du contrat générique users-permissions ou suppression d'un parcours de désinscription légitime.

## Critères d'acceptation

- [ ] Un utilisateur ordinaire ne peut ni modifier le rôle ou l'état d'un compte, ni modifier le profil d'un autre utilisateur.
- [ ] Le profil personnel reste éditable seulement sur une liste de champs autorisés si ce besoin existe.
- [ ] Les permissions effectives sont exportées ou consignées sans secrets.
- [ ] Toute écriture publique maintenue a une justification et une protection adaptées au parcours.

## Notes techniques

L'audit initial ne constatait pas de permission `user.update` ordinaire en production. Vérifier l'état actuel avant de qualifier le projet de vulnérable ; ces tickets suivent aussi des comportements conservés pour la compatibilité.

## Référence Strapi

Users-permissions, contrat de `AUTH-002` et `AUTH-004` ; dérogations du projet à documenter.

## Dépendances

`AUTH-001`, `AUTH-002`, `AUTH-004`.

## Fichiers

- `internal/httpapi/users.go`
- `docs/configuration.md`
- `Routes, policies et permissions du projet cible`

