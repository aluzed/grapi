---
status: in-progress
---

# SEC-009 : Restreindre CORS aux origines utilisées par les applications

Priorité : **Moyenne**. Source : [revue complémentaire du 29 octobre 2026](../../security-review-20261029.md).

## Contexte

La configuration par défaut reflète toute origine et autorise les credentials. L'exploitabilité dépend notamment des cookies et des sous-domaines ; la configuration déployée n'a pas été vérifiée dans cette revue.

## Objectif

En tant qu'exploitant, je veux une liste explicite d'origines afin de limiter les requêtes lisibles par des sites tiers.

## Périmètre

- Inventorier les origines du front et de l'admin et configurer CORS dans le projet.
- Vérifier les cookies et les besoins de credentials pour ces parcours.

## Hors périmètre

Modification des défauts de compatibilité du moteur et refonte de l'authentification.

## Critères d'acceptation

- [ ] Les origines approuvées passent les prévols et les requêtes utiles avec les credentials nécessaires.
- [ ] Une origine arbitraire n'obtient pas d'autorisation CORS de lecture avec credentials.
- [ ] Le comportement est vérifié dans un navigateur sur les parcours de connexion et d'administration.
- [ ] Les origines autorisées et les paramètres de cookies sont documentés.

## Notes techniques

Le cookie admin est `SameSite=Lax` par défaut selon le rapport. Ne pas présenter ce constat de configuration comme une fuite démontrée en production.

## Référence Strapi

Middleware CORS configurable ; durcissement au niveau du projet.

## Dépendances

`API-008`, `ADM-002`.

## Fichiers

- `docs/configuration.md`
- `DEPLOY.md`
- `Configuration des middlewares du projet cible`

## Avancement

- `DEPLOY.md` donne un `config/middlewares.yaml` avec une liste explicite d'origines pour `strapi::cors`. Vérifié sur la révision de travail en chargeant cet exemple tel quel : un prévol depuis une origine listée reçoit `Access-Control-Allow-Origin`, une origine arbitraire n'en reçoit pas (test jetable, non conservé).
- Reste : inventorier les origines réelles du front et de l'admin, appliquer, puis recette navigateur des parcours de connexion et d'administration.
