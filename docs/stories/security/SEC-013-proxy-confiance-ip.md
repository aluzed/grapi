---
status: in-progress
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

- [x] Un en-tête forgé par le client ne change pas l'identité utilisée par le rate limit.
- [x] Deux clients distincts conservent des compteurs indépendants derrière le proxy.
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

## Avancement

- Correctif moteur : un en-tête `X-Forwarded-For` répété n'était lu que par sa première ligne, celle qu'un client peut forger, alors que Node les concatène. Si un proxy ajoute sa propre ligne au lieu de compléter la première, le client choisissait son IP. Les lignes sont maintenant jointes comme dans Node.
- `TestIPForgedBehindProxy` : avec `koa: true` et `maxIpsCount: 1`, aucune valeur forgée (même ligne ou ligne séparée) ne devient l'identité, et deux clients gardent des IP distinctes.
- `DEPLOY.md` documente `maxIpsCount` selon le nombre de proxys et la fermeture du port interne.
- Reste : décrire la chaîne réelle (Traefik seul ?) et vérifier que le port 1337 n'est pas joignable de l'extérieur.
