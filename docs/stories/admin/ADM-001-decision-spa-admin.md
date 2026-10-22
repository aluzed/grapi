# ADM-001 : Décision et spike : SPA Strapi ou admin maison (D1)

## Contexte
L'admin de Strapi est une SPA React (`@strapi/admin`, MIT hors `ee/`) qui parle à une API interne non versionnée (`/admin/*`, `/content-manager/*`, `/content-type-builder/*`, `/upload/*`, `/i18n/*`, `/users-permissions/*`, `/email/*`). Cette API change à chaque mineure : entre 5.38 et 5.59 sont arrivés les sessions avec refresh, les admin tokens, MCP et des endpoints IA. `feasibility.md` (D1) recommande l'option B, mais on doit trancher sur des faits avant d'écrire `ADM-002` à `ADM-010`.

## Objectif
Prendre la décision A ou B, à partir de deux spikes courts, et l'acter dans `docs/decisions/D1-admin.md`.

## Périmètre
- **Option A, réutiliser la SPA Strapi** en étant compatible fil à fil avec ses endpoints.
- **Option B, admin maison** : SPA React + Vite embarquée dans le binaire via `go:embed`, sur une API admin grapi documentée et versionnée (`/admin/api/v1/*`).
- Grille de critères, chacun noté de 1 à 5 :
  - couverture fonctionnelle immédiate ;
  - coût de maintenance face aux mineures Strapi ;
  - surface de contrat à reproduire (nombre d'endpoints, formes JSON) ;
  - licence (exclusion des dossiers EE du bundle) ;
  - binaire unique (pas de Node au build ou au runtime) ;
  - respect des conventions UI du projet ;
  - performance perçue (taille du bundle, TTI) ;
  - extensibilité des plugins grapi.
- **Spike A (2 à 3 jours)** :
  1. Construire `@strapi/admin` avec un projet Node minimal (CM, CTB, upload, i18n, users-permissions, email), sans aucun code `ee/`.
  2. Servir le `build/` depuis un stub Go.
  3. Implémenter juste ce qu'il faut pour se connecter et lister un content type : `/admin/init`, `/admin/project-type` (`isEE:false`), `/admin/login`, `/admin/access-token`, `/admin/users/me`, `/admin/users/me/permissions`, `/content-manager/init`, `/content-manager/content-types/:uid/configuration`, `GET /content-manager/collection-types/:uid`.
  4. Mesurer le nombre d'endpoints appelés au démarrage et pendant ce parcours, et la complexité des contrats (`*/shared/contracts/*.ts`).
- **Spike B (2 à 3 jours)** :
  1. Squelette Vite + React + TS + Tailwind embarqué via `go:embed`, servi sous `/admin`.
  2. Login sur un endpoint stub, liste paginée générique d'un content type à partir des métadonnées de schéma exposées par grapi.
  3. Estimer le volume restant écran par écran (`ADM-002` à `ADM-010`).
- Rédiger `docs/decisions/D1-admin.md` : grille remplie, mesures, décision, conséquences sur les tickets.

## Hors périmètre
- Implémenter l'admin complet.
- L'API de contenu publique : elle reste compatible Strapi quelle que soit la décision.

## Critères d'acceptation
- [ ] Les deux spikes sont démontrés (capture ou courte vidéo) sur le homelab.
- [ ] La liste exhaustive des endpoints que la SPA Strapi appelle pendant le parcours du spike A est versionnée dans `docs/decisions/D1-admin.md`.
- [ ] La grille est remplie avec une justification par critère.
- [ ] La décision est actée, et `ADM-002` à `ADM-010` portent la mention « option retenue ».
- [ ] Aucun fichier sous licence Enterprise n'entre dans le bundle du spike A (vérifié par une liste des modules du bundle). Sont concernés : les dossiers `ee/`, les paquets `core/review-workflows` et `core/content-releases`, et les dossiers `core/content-manager/{admin,server}/src/{history,preview}`.

## Notes techniques
- **Recommandation par défaut : B** (D1).
  - API stable et à nous.
  - MIT intégral et binaire unique.
  - Conventions UI maîtrisées.
  - Pas de course derrière les mineures Strapi.
- Si A est retenu :
  - `ADM-002` à `ADM-010` deviennent des tickets « contrat serveur » (reproduire les endpoints et formes JSON des `shared/contracts`) au lieu de tickets UI ;
  - le build de l'admin exige Node en CI, et la version de `@strapi/admin` est figée par release de grapi.
- Dans les deux cas, l'API admin est protégée par `AUTH-001` (RBAC) et `AUTH-007` (sessions admin).

## Référence Strapi
- `core/admin/admin/src/StrapiApp.tsx`, `core/admin/admin/src/utils/getFetchClient.ts`
- `core/admin/server/src/routes/`, `core/admin/server/src/routes/serve-admin-panel.ts`
- `core/content-manager/shared/contracts/`, `core/content-type-builder/shared/contracts/`, `core/upload/shared/contracts/`
- `core/strapi/src/node/create-build-context.ts`

## Dépendances
- `SETUP-001`, `SETUP-004`

## Fichiers
- `docs/decisions/D1-admin.md`
- `admin/` (squelette du spike B, conservé si B est retenu)
- `compat/admin-spike/` (stub du spike A, supprimé après décision)
