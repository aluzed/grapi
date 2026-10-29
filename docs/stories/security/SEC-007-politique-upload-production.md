---
status: todo
---

# SEC-007 : Configurer une politique d'upload explicite en production

Priorité : **Haute**. Source : [revue complémentaire du 29 octobre 2026](../../security-review-20261029.md).

## Contexte

Sans `plugins.upload.config.security`, tous les types restent acceptés. Le correctif `SEC-003` protège une politique existante ; il ne l'active pas. Des fichiers actifs déjà stockés restent accessibles.

## Objectif

En tant qu'exploitant, je veux autoriser uniquement les formats nécessaires et les comptes légitimes afin de réduire le risque de contenu actif sur l'origine du CMS.

## Périmètre

- Définir la liste des formats utiles et les rôles autorisés à uploader dans le projet déployé.
- Inventorier les fichiers actifs existants et décider de leur traitement.
- Prévoir une origine distincte sans cookies admin si des contenus actifs sont nécessaires.

## Hors périmètre

Suppression automatique de fichiers existants et modification globale des défauts du moteur.

## Critères d'acceptation

- [ ] La configuration effective contient une liste de types autorisés adaptée aux usages du projet.
- [ ] Les rôles sans besoin d'upload ne disposent pas de cette permission.
- [ ] Un fichier autorisé est accepté et les contenus actifs interdits sont refusés, y compris avec un MIME trompeur.
- [ ] L'inventaire des anciens fichiers et la décision de traitement sont consignés sans suppression implicite.

## Notes techniques

Commencer par les formats réellement utilisés, par exemple JPEG, PNG, WebP et PDF. La configuration actuelle du projet doit être relue ; aucune exposition actuelle n'est démontrée par la revue complémentaire.

## Référence Strapi

Configuration du plugin upload ; préserver le contrat du moteur hors du projet durci.

## Dépendances

`SEC-003`, `MEDIA-001`, `AUTH-004`.

## Fichiers

- `docs/configuration.md`
- `DEPLOY.md`
- `Configuration upload et permissions du projet cible (hors dépôt si nécessaire)`

