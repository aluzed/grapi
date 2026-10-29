---
status: todo
---

# SEC-014 : Adapter les limites de ressources au déploiement

Priorité : **Moyenne**. Source : [revue complémentaire du 29 octobre 2026](../../security-review-20261029.md).

## Contexte

La limite d'image par défaut autorise environ 268 millions de pixels, soit environ 1 Go pour un tampon RGBA, sans compter les traitements annexes. Le rapport demande aussi de vérifier les délais HTTP et les ressources du conteneur.

## Objectif

En tant qu'exploitant, je veux des limites cohérentes avec la capacité du serveur afin qu'une requête coûteuse ne rende pas le CMS indisponible.

## Périmètre

- Fixer les budgets mémoire, taille des uploads, pixels, concurrence et délais HTTP au niveau approprié.
- Mesurer un upload et une requête lente dans un environnement de test représentatif.

## Hors périmètre

Nouvelle campagne de charge sur la production et modification arbitraire des défauts compatibles Strapi.

## Critères d'acceptation

- [ ] La limite `sharp.limitInputPixels` et la taille maximale d'upload sont justifiées par le budget mémoire du service.
- [ ] Les limites CPU/mémoire, la concurrence et les délais de lecture utiles sont appliqués dans le déploiement.
- [ ] Une image dépassant la limite suit le contrat documenté, sans décodage coûteux ; ce contrat peut stocker le fichier sans produire de formats.
- [ ] Les essais contrôlés n'entraînent pas de dépassement du budget retenu et les usages légitimes restent possibles.

## Notes techniques

La taille d'un tampon RGBA est une estimation minimale, pas la mémoire totale du traitement. Les protections contre les corps sans limite et le décodage intégral de GIF ont déjà été traitées par le premier audit.

## Référence Strapi

Paramètres d'image compatibles sharp et protections HTTP propres au déploiement.

## Dépendances

`MEDIA-002`, `API-001`, `API-008`.

## Fichiers

- `internal/media/image.go`
- `docs/configuration.md`
- `DEPLOY.md`
- `Configuration du serveur HTTP, du proxy et du conteneur cible`

