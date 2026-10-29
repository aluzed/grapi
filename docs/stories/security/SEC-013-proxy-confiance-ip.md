---
status: todo
---

# SEC-013 : Fiabiliser l'IP client derrière le proxy de confiance

Priorité : **Moyenne**. Source : [revue complémentaire du 29 octobre 2026](../../security-review-20261029.md).

## Contexte

Une chaîne de proxy mal déclarée peut regrouper tous les clients sous la même IP ou laisser un client influencer la clé du rate limit avec des en-têtes forgés.

## Objectif

En tant qu'exploitant, je veux que les limites utilisent l'IP réelle du client afin de prévenir leur contournement et les blocages collectifs.

## Périmètre

- Décrire la chaîne de proxy réelle et configurer `server.proxy.koa`, `ipHeader` et `maxIpsCount` en conséquence.
- Fermer l'accès public au port interne du CMS et vérifier la réécriture des en-têtes par le proxy.

## Hors périmètre

Reconfiguration générale du réseau et compteur de limitation distribué.

## Critères d'acceptation

- [ ] Un en-tête forgé par le client ne change pas l'identité utilisée par le rate limit.
- [ ] Deux clients distincts conservent des compteurs indépendants derrière le proxy.
- [ ] Le port interne est inaccessible depuis l'extérieur de la chaîne de confiance.
- [ ] La configuration et les tests correspondent au nombre réel de proxys ; `maxIpsCount: 1` n'est utilisé que pour un seul proxy.

## Notes techniques

Le code lit la configuration structurée `server.proxy.koa` ; ne pas utiliser le simple booléen `proxy: true`. Les correctifs du premier audit conservent aussi l'en-tête IP lors des mutations GraphQL.

## Référence Strapi

Résolution d'IP compatible Koa ; voir `AUTH-006`.

## Dépendances

`AUTH-006`, `API-008`.

## Fichiers

- `internal/clientip/clientip.go`
- `internal/clientip/clientip_test.go`
- `docs/configuration.md`
- `DEPLOY.md`
- `Configuration réseau et proxy du projet cible`

