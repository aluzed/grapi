---
status: todo
---

# SEC-008 : Activer la protection réseau des webhooks du projet

Priorité : **Haute**. Source : [revue complémentaire du 29 octobre 2026](../../security-review-20261029.md).

## Contexte

`server.webhooks.blockPrivateNetworks` est désactivé par défaut. La validation à l'enregistrement ne protège pas seule contre une résolution DNS différente ou une redirection ultérieure.

## Objectif

En tant qu'exploitant, je veux vérifier l'adresse au moment de la connexion afin que les webhooks ne puissent pas atteindre le réseau interne.

## Périmètre

- Activer la protection dans le projet déployé après inventaire des destinations légitimes.
- Vérifier les résolutions DNS, les redirections et les livraisons existantes.

## Hors périmètre

Refonte du moteur de webhooks et interdiction des usages internes explicitement requis sans alternative validée.

## Critères d'acceptation

- [ ] La configuration effective active `server.webhooks.blockPrivateNetworks: true`.
- [ ] Un webhook public légitime fonctionne.
- [ ] Une destination privée directe, une redirection vers le privé et un nom résolu vers le privé sont bloqués dans un environnement de test contrôlé.
- [ ] Les éventuels besoins de destinations internes ont une solution documentée avant bascule.

## Notes techniques

La correction des plages de `SEC-006` et l'activation de la protection sont deux travaux distincts. Ne pas sonder de services internes réels pour valider.

## Référence Strapi

Configuration `server.webhooks` ; protection de connexion ajoutée par grapi.

## Dépendances

`SEC-006`, `EXT-006`.

## Fichiers

- `internal/webhooks/ssrf.go`
- `docs/configuration.md`
- `DEPLOY.md`
- `Configuration serveur du projet cible`

