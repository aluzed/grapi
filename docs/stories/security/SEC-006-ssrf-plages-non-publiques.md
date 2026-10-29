---
status: in-progress
---

# SEC-006 : Bloquer les plages non publiques oubliées dans les connexions sortantes

Priorité : **Moyenne**. Source : [revue complémentaire du 29 octobre 2026](../../security-review-20261029.md).

## Contexte

Avec la protection réseau activée, les webhooks acceptaient encore le CGNAT ; les téléchargements par URL et la validation des webhooks acceptaient une partie de `0.0.0.0/8`. L'impact dépend du réseau accessible depuis le serveur.

## Objectif

En tant qu'exploitant, je veux bloquer ces adresses non publiques afin de réduire les possibilités d'accès au réseau interne par le CMS.

## Périmètre

- Bloquer `100.64.0.0/10` et `0.0.0.0/8` sur les chemins concernés.
- Couvrir IPv4 et IPv4 encapsulé en IPv6, à la validation et lors de la connexion selon le chemin.

## Hors périmètre

Activation de `server.webhooks.blockPrivateNetworks` sur les projets (`SEC-008`).

## Critères d'acceptation

- [ ] Les tests couvrent les limites des deux plages et leurs formes IPv4 encapsulées en IPv6.
- [ ] Les plages interdites sont refusées sur les téléchargements par URL et sur les webhooks protégés.
- [ ] Les adresses publiques restent utilisables.
- [ ] La protection à la connexion reste effective après résolution DNS et lors des redirections.

## Notes techniques

Patches présents dans les filtres réseau admin et webhook. Tests sauvegardés : `TestPublicOnlyAddresses`, `TestPublicOnly`, `TestWebhookURLInProduction` et `TestPublicOnlyTransport`. Consigner séparément la couverture de la résolution et des redirections.

## Référence Strapi

Webhooks et upload distant ; durcissement grapi de la protection réseau existante.

## Dépendances

`EXT-006`, `MEDIA-006`, `SEC-015`.

## Fichiers

- `internal/webhooks/ssrf.go`
- `internal/webhooks/ssrf_test.go`
- `internal/adminapi/media_settings.go`
- `internal/adminapi/media_settings_test.go`

