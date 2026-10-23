---
status: in-progress
---

# MIG-002 : Recette dev-freelance

## Contexte
C'est le jalon de sortie de la phase 1 : un vrai projet en production, avec une vraie base, servi par grapi sans que son front s'en aperçoive. dev-freelance tourne en prod sur Strapi 5.59 sous Bun (Dokploy, serveur un serveur privé). Son front Go (`apps/web`) consomme l'API REST de Strapi. Une méthode de vérification existe déjà : les hash et tailles de 7 endpoints (posts, categories, themes, sticky-posts, memes, projects, theme-pages) ont été comparés avant et après la mise à jour 5.59 et le refactoring des règles.

## Objectif
Démontrer que le front Go de dev-freelance fonctionne à l'identique contre grapi, sur une copie de la base de prod, et mesurer le gain de ressources.

## Périmètre
- **Environnement** : uniquement sur le homelab (`ssh homelab`). Aucun conteneur sur le Mac, et rien sur le serveur de prod.
- **Données** :
  - copie de la base de prod (dump `pg_dump` de `la base de prod`, transféré en flux serveur vers homelab) ;
  - copie des uploads (ou montage en lecture seule d'une copie) ;
  - les variables d'env transférées de la même façon, jamais affichées.
- **Deux instances côte à côte sur la même copie** : Strapi 5.59 (image de prod) et grapi, chacune sur sa propre copie de la base pour les tests d'écriture.
- **Portage des règles métier** en hooks Go (`EXT-002`), à comportement identique à `libs/row-rules.ts` et aux middlewares de `src/index.ts` :
  - slugs de post, catégorie et médium à la création de chaque ligne ;
  - slug de thème préfixé par le parent ;
  - hash d'abonné ;
  - slug régénéré si vide en update ;
  - génération IA FAQ, TL;DR et meta description sur toggle, puis toggle remis à `false` ;
  - invalidation du cache Redis `BLOG:` selon `CACHE_INVALIDATION_MAP`, et flag `WORKER:SITEMAP`.
- **Comparaison de l'API publique** :
  - le script `apicmp.sh` déjà utilisé (7 endpoints) ;
  - le corpus élargi du banc pour la fixture `devfreelance` ;
  - les 21 cas de caractérisation, passés par les chemins REST et Document Service (le chemin admin est reporté en phase 2).
- **Front** : `apps/web` pointé sur grapi (variable d'URL de l'API). Navigation automatisée sur la home, une catégorie, un article, `/themes`, un thème, `/media` et un détail média. Les réponses HTML sont comparées (diff après normalisation des horodatages).
- **Perf et mémoire** : `make bench-perf` du banc (`SETUP-002`) sur les scénarios dev-freelance, avec 2 CPU par moteur. Les résultats sont comparés à la référence du 2026-10-22 : Bun 209 req/s, p50/p99 188/281 ms, 272 Mo au repos et 530 Mo au pic ; Node 154 req/s, 430 Mo au repos et 900 Mo au pic.

## Hors périmètre
- Basculer la prod sur grapi : cela demandera une décision explicite de l'utilisateur, après la phase 2 (admin).
- L'admin.

## Critères d'acceptation
- [ ] Les hash des 7 endpoints sont **identiques** entre Strapi 5.59 et grapi sur la même copie de base : posts, categories, themes, sticky-posts, memes, projects, theme-pages.
- [x] Les 21 cas de caractérisation donnent 0 différence sur les chemins REST et Document Service.
- [x] Le corpus du banc `devfreelance` est entièrement OK, ou chaque DIFF restant est listé dans `docs/divergences.md` avec sa justification.
- [ ] Le front `apps/web` rend les 7 pages types à l'identique contre les deux moteurs.
- [x] Un upload d'image via l'API produit les formats `thumbnail` et `small` (comme en prod).
- [ ] Le rapport de perf indique req/s, p50/p99 et la mémoire au repos, au pic et après retour au repos, pour Strapi sous Bun et pour grapi.
- [x] Le homelab est nettoyé après la recette : conteneurs, images, copies de base et dossiers temporaires.

## Avancement
- 2026-10-22 : sur une copie de la base de prod (homelab, supprimée ensuite), les 41 appels GET du front (`compat/corpus/devfreelance-prod.yaml`, slugs tirés des données) donnent 44/44 réponses identiques entre Strapi 5.59 (image de référence du banc avec les schémas réels de dev-freelance) et grapi. Les contrôleurs custom de dev-freelance (JS) ne sont pas dans la comparaison : ils attendent `EXT-003`.
- Banc `devfreelance` en mode grapi : 116/116 sous SQLite et Postgres.
- Rapport de perf (2026-10-22, `docs/perf.md`, mode `perf` du banc) : sur les appels du front, grapi tient environ 5 570 req/s contre 326 pour Strapi 5.59 sous Node, p99 13 ms contre 125 ms, 32 à 43 Mio de mémoire contre 260 à 780 Mio.
- Sur la copie de prod (même jour, supprimée ensuite) : grapi 5 211 req/s, p99 19,6 ms, 28 à 47 Mio ; Strapi 242 req/s, p99 172 ms, 416 à 744 Mio.
- Les 21 cas de caractérisation, avec le code du projet des deux côtés (`rules.js` pour Strapi, `compat/fixtures/devfreelance/framework/backend` pour grapi) : 0 différence par REST (`devfreelance_rules`, 93 requêtes) et par le Document Service (`devfreelance--rules`, 87 cas, `unpublish` compris), sous SQLite et Postgres.
- Reste : le front `apps/web` contre les deux moteurs, la mesure sous Bun.

## Notes techniques
- La génération IA appelle un vrai LLM : utiliser un faux endpoint en test (même forme de réponse), et ne faire qu'un seul essai réel, comme pour la vérification des middlewares de la 5.59.
- Les CLI `cli/*.ts` de dev-freelance écrivent en SQL direct : ils doivent continuer à marcher contre une base gérée par grapi, ce qui confirme D3.

## Référence Strapi
- dev-freelance : `apps/backend/src/index.ts`, `apps/backend/src/libs/row-rules.ts`, `apps/backend/src/api/*/content-types/*/schema.json`, `apps/web/` (consommateur)
- Scripts de la session de mise à jour 5.59 : `df/apicmp.sh` et `mw/*` du scratchpad, à recopier dans `compat/fixtures/devfreelance/`.

## Dépendances
- Phase 1 complète : `DB-011`, `DOC-*`, `API-*`, `AUTH-002`, `AUTH-004`, `AUTH-008`, `MEDIA-001` à `MEDIA-003`, `EXT-001`, `EXT-002`, `SETUP-002`.

## Fichiers
- `compat/fixtures/devfreelance/` (schémas, corpus, hooks Go portés), `compat/fixtures/devfreelance/hooks/*.go`, `compat/report/mig-002-<date>.md`
