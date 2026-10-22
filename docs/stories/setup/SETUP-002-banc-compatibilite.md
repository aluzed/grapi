---
status: done
---

# SETUP-002 : Banc de compatibilité contre Strapi 5.59

## Contexte
La promesse de grapi est d'offrir les mêmes comportements observables que Strapi. Sans mesure automatique, la compatibilité se dégradera ticket après ticket. Cette méthode a déjà fait ses preuves sur dev-freelance : 21 cas passés par 3 chemins (admin, REST, `strapi.documents()`) sur une copie de la base de prod, puis un diff des sorties, et des hash de 7 endpoints comparés avant et après déploiement.

## Objectif
Construire un banc qui envoie le même corpus de requêtes à une instance Strapi 5.59 de référence et à grapi, sur le même projet et les mêmes données initiales, puis produit un diff lisible. Le banc mesure aussi le débit et la mémoire.

## Périmètre
- **Environnement : uniquement sur `ssh homelab`** (32 cœurs). Jamais de `docker build` ni de `docker run` sur le Mac. Le Makefile lance tout via `ssh homelab` ou `docker --context homelab`.
- **Instance de référence** :
  - Strapi 5.59.0 sous Node 24 (image `node:24.22.1-slim`) ;
  - Postgres 16 et SQLite ;
  - une instance par projet fixture, sur des ports fixes ;
  - versions figées dans `compat/reference/versions.env`.
- **Projets fixtures** (`compat/fixtures/<nom>/`), avec le même `src/` lu par les deux moteurs :
  - `basic` : un type de chaque attribut scalaire, single type, collection type ;
  - `relations` : les 10 types de relation, dont morph, self-relation et relations bidirectionnelles ;
  - `components` : composants imbriqués (3 niveaux), répétables, zone dynamique ;
  - `i18n-dp` : draft & publish combiné à i18n sur des types localisés et non localisés ;
  - `devfreelance` : copie des schémas de dev-freelance (post, category, theme, theme-page, medium, project, comment, subscriber, sticky-post, etc.).
- **Seed** : un script de données initiales identiques des deux côtés, passé par l'API (pas de SQL), pour que la référence produise ses propres ids.
- **Corpus de requêtes** (`compat/corpus/*.yaml`) :
  - chaque cas décrit : méthode, chemin, query, body, token, étapes ;
  - il couvre les filtres (tous les opérateurs), le populate (`*`, profond, `on`, `count`), sort, fields, la pagination page et start, locale et status, strictParams ;
  - il couvre aussi les erreurs (400, 401, 403, 404, 405, 413), les permissions par rôle, la création et la publication.
- **Normalisation JSON avant diff** :
  - `id` : remplacés par un alias stable (`#post:1`) déduit de l'ordre de création ;
  - `documentId` : correspondance référence vers grapi établie à la création, puis réécriture ;
  - dates (`createdAt`, `updatedAt`, `publishedAt`) : remplacées par `<ts>` ;
  - les hash, les URL d'upload et les jetons sont masqués ;
  - l'ordre des clés est ignoré, mais l'ordre des tableaux est conservé (il fait partie du contrat).
- **Rapport** : `compat/report/<date>.md`, avec par cas OK, DIFF (diff JSON coloré) ou SKIP (fonction pas encore implémentée et marquée comme telle), plus un résumé par domaine.
- **Mode perf** :
  - scénarios k6 (image `grafana/k6`, sur le homelab) : liste paginée avec populate, détail, création, filtre profond ;
  - 2 CPU par moteur (cgroup `cpus=2`) ;
  - mesures : req/s en saturation, p50/p99, mémoire au repos et au pic (`memory.current` et `memory.stat` anon du cgroup, relevés toutes les secondes) ;
  - comparaison avec la référence dev-freelance du 2026-10-22 : Bun 209 req/s pour 272 Mo au repos, Node 154 req/s pour 430 Mo.
- **CI** : le job « compat » est déclenché manuellement et la nuit. Il fait un rsync puis un ssh vers le homelab, et publie le rapport en artefact.

## Hors périmètre
- Comparer l'API admin, sauf si D1 = A (`ADM-001`).
- Les tests unitaires de chaque package (propres à chaque ticket).

## Critères d'acceptation
- [ ] `make bench-compat FIXTURE=basic` tourne de bout en bout depuis le Mac sans aucun conteneur local, et produit le rapport.
- [ ] Contre la référence elle-même (Strapi contre Strapi), le diff est vide : la normalisation est correcte.
- [ ] Une divergence volontaire (champ renommé dans la réponse) apparaît comme DIFF avec le chemin JSON exact.
- [ ] Le corpus initial couvre au moins 150 cas, répartis sur les 5 fixtures.
- [ ] `make bench-perf` produit req/s, p50/p99 et mémoire (repos, pic, après 5 min de repos) pour les deux moteurs.
- [ ] Le homelab est nettoyé en fin de run : conteneurs, volumes et dossiers temporaires supprimés.

## Notes techniques
- Le runner du banc est écrit en Go (`compat/cmd/compat`) pour réutiliser le parseur qs de `API-003` et générer des query strings identiques des deux côtés.
- La correspondance des documentId se fait dans l'ordre des réponses de création. Toute création non déterministe (par exemple un clone) doit déclarer explicitement ses alias dans le cas.
- Attention au piège déjà rencontré : un `pkill -f` dont le motif apparaît dans la ligne de commande ssh tue la session. Utiliser `pgrep -f "[m]otif"`.
- Les hooks de compression de sortie (RTK) tronquent le JSON : le runner écrit les réponses dans des fichiers et ne les affiche jamais.

## Référence Strapi
- `packages/utils/api-tests/` : helpers de tests d'API de Strapi, une source d'inspiration pour le corpus (MIT).
- `tests/api/` à la racine du clone, s'il est présent : scénarios d'API existants, utilisables comme spécification.

## Dépendances
- `SETUP-001`.

## Fichiers
- `compat/cmd/compat/`, `compat/reference/{Dockerfile,versions.env,start.sh}`, `compat/fixtures/*/`, `compat/corpus/*.yaml`, `compat/normalize/`, `compat/k6/*.js`, `compat/report/`, `Makefile` (cibles `bench-compat`, `bench-perf`)
