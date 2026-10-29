---
status: in-progress
---

# SEC-012 : Borner les requêtes GraphQL du projet

Priorité : **Moyenne**. Source : [revue complémentaire du 29 octobre 2026](../../security-review-20261029.md).

## Contexte

Lorsque GraphQL est activé, `depthLimit` et `maxLimit` sont illimités par défaut. Les alias peuvent aussi augmenter le coût des requêtes sans dépasser la profondeur.

## Objectif

En tant qu'exploitant, je veux limiter le coût des requêtes GraphQL afin de préserver la disponibilité du CMS.

## Périmètre

- Vérifier si GraphQL est utilisé ; le désactiver sinon.
- Définir profondeur, pagination et protections de débit ou de complexité selon les requêtes réelles.

## Hors périmètre

Modification globale des valeurs par défaut du plugin et développement d'un moteur de coût complet sans mesure préalable.

## Critères d'acceptation

- [ ] Si GraphQL est inutile, sa désactivation effective est vérifiée.
- [ ] S'il est actif, des limites finies de profondeur et de pagination sont configurées et les requêtes excessives sont rejetées ou bornées selon le contrat.
- [ ] Les requêtes usuelles du front passent et un scénario avec de nombreux alias valide la protection retenue.
- [ ] Les paramètres et les mesures justifiant les limites sont consignés.

## Notes techniques

Une profondeur de 8 et un maximum de 100 éléments sont des points de départ proposés dans le rapport, à ajuster. L'audit initial indiquait GraphQL désactivé sur dev-freelance ; vérifier l'état actuel.

## Référence Strapi

Plugin GraphQL : `depthLimit`, `maxLimit`, `defaultLimit` et protections du projet.

## Dépendances

`PLG-004`, `API-008`.

## Fichiers

- `internal/httpapi/graphql.go`
- `graphql_test.go`
- `docs/configuration.md`
- `Configuration GraphQL et proxy du projet cible`

## Avancement

- Nouvelle option grapi `plugins.graphql.config.maxAliases` : une opération dépassant ce nombre de champs aliasés, fragments compris, reçoit un 400 `GRAPHQL_VALIDATION_FAILED`. Défaut illimité, comme Strapi. Testée dans `TestGraphQLExtensions` avec un fragment.
- `DEPLOY.md` propose `enabled: false` ou `depthLimit: 8`, `maxLimit: 100`, `maxAliases: 20`.
- Reste : vérifier l'état de GraphQL dans le projet déployé, puis mesurer les requêtes du front pour ajuster les limites.
