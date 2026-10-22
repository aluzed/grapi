# grapi : étude de faisabilité

Référence étudiée : Strapi **5.59.0** (clone `../strapi`, commit `265c6ef`, 2026-10-22).

Objectif : un CMS headless en Go qui offre **les mêmes fonctionnalités** que Strapi 5, y compris celles que Strapi réserve à son édition Enterprise, le tout sous licence MIT. On ne reproduit pas l'implémentation : on reproduit les comportements observables (API, modèle de contenu, règles métier).

## 1. Verdict

**Faisable**, avec trois réserves structurantes qui demandent une décision avant d'écrire du code (section 4) :

1. Le code utilisateur de Strapi est du JS/TS chargé à chaud. Go ne peut pas l'exécuter nativement.
2. L'admin est une SPA React énorme dont l'API interne n'est pas versionnée.
3. La couche données (relations, draft & publish, i18n) concentre la vraie difficulté. Le reste est du travail de volume, pas de recherche.

Rien n'est bloquant techniquement. Toutes les briques ont un équivalent Go mûr.

## 2. Ce que couvre Strapi 5.59 (inventaire)

| Domaine | Contenu | Difficulté Go |
|---|---|---|
| Modèle de contenu | content types (collection/single), 20 types d'attributs, composants, zones dynamiques, custom fields, options (required, unique, private, min/max, regex, default...) | Moyenne |
| Base de données | Postgres, MySQL/MariaDB, SQLite. Nommage dérivé (snake_case, 55 car. + hash, tables `_lnk`, `_mph`, `_cmps`, colonnes d'ordre), synchro du schéma au boot (diff 3 voies), migrations utilisateur et internes, transactions imbriquées, lifecycles | **Difficile** |
| Moteur de requêtes | 25 opérateurs de filtre, filtres imbriqués à travers relations et composants, tri profond, `_q`, populate profond avec fragments `on` | **Difficile** |
| Relations | 10 types (dont morph), connect/disconnect/set avec `position` before/after/start/end, ordre fractionnaire | **Difficile** |
| Document Service | documentId (cuid2), draft & publish (2 lignes par locale, recâblage des relations à la publication), clone, discardDraft, publicationFilter, middlewares, événements | **Difficile** |
| API REST | format `{data, meta}`, erreurs `{data:null, error}`, pagination page/start, strictParams, sanitize input/output/query selon permissions | Moyenne |
| Serveur | routes déclaratives, pipeline auth/policies/middlewares, 13 middlewares globaux, policies, cron, webhooks, event hub, core store, health | Facile à moyenne |
| Auth utilisateurs finaux | users-permissions : local, JWT ou sessions + refresh, confirmation e-mail, reset, 17 providers OAuth, rôles Public/Authenticated, permissions par action, rate limit | Moyenne |
| Auth admin | utilisateurs admin, sessions + refresh en cookie, invitations, rôles, RBAC (actions, sujets, champs, locales, conditions), API tokens, admin tokens, transfer tokens | Moyenne à difficile (RBAC) |
| Médias | fichiers, dossiers, formats responsive (sharp), providers local / S3 / Cloudinary, URLs signées | Moyenne |
| Plugins officiels | i18n, email (5 providers), GraphQL (schéma dynamique), documentation/OpenAPI, Sentry, MCP | Facile à difficile (GraphQL) |
| Admin | Content Manager, Content-Type Builder, Media Library, Settings, configuration des vues, preview, extensions UI des plugins | **Très gros volume** |
| CLI | develop/start/build, generate, listes, gestion des admins, config dump/restore, export/import/transfer, types TS | Facile à moyenne |
| Fonctions Enterprise de Strapi | SSO, audit logs, content history, review workflows, content releases, advanced preview | Moyenne chacune |

Détail et fichiers sources de référence : dans chaque ticket de [`stories/`](stories).

## 3. Équivalents Go

| Besoin | Choix proposé | Pourquoi |
|---|---|---|
| HTTP | `net/http` (routeur Go 1.22+) ou `go-chi/chi` | Standard, pas de framework lourd |
| SQL | `jackc/pgx` (Postgres), `modernc.org/sqlite` (SQLite sans CGO), `go-sql-driver/mysql` | Drivers de référence |
| Construction SQL | Builder maison au-dessus de `database/sql` (éventuellement `doug-martin/goqu` pour la partie dialecte) | Le schéma est dynamique : pas d'ORM à génération de code (ent, sqlc, gorm-gen) |
| documentId | cuid2 (implémentation maison, 30 lignes) | Format identique à Strapi |
| JWT, mots de passe | `golang-jwt/jwt/v5`, `x/crypto/bcrypt` | Compatibles avec les hash bcryptjs de Strapi |
| OAuth | `golang.org/x/oauth2`, `coreos/go-oidc` | |
| GraphQL | `graphql-go/graphql` + `vektah/gqlparser` pour fusionner le SDL utilisateur | Schéma construit au runtime : gqlgen est exclu (génération de code à la compilation) |
| OpenAPI | `getkin/kin-openapi`, Swagger UI embarqué via `embed` | |
| Images | `davidbyttow/govips` (libvips, CGO) | sharp est lui-même basé sur libvips : mêmes résultats, mêmes perfs. Voir décision D4 |
| Stockage | `aws-sdk-go-v2` (S3, R2, MinIO), `cloudinary-go` | |
| E-mail | `wneessen/go-mail`, SDK SES / Mailgun / SendGrid | |
| Cron | `robfig/cron/v3` | |
| CLI | `spf13/cobra` | |
| Logs | `log/slog` | |
| JS/TS embarqué (optionnel) | `evanw/esbuild` (transpile TS, bundle les paquets npm purs) + `dop251/goja` (moteur JS en Go pur) | esbuild est écrit en Go : on peut charger des `.ts` sans Node. Voir décision D2 |

## 4. Décisions à prendre avant d'implémenter

### D1. Admin : réutiliser la SPA Strapi ou faire la nôtre ?

- **Option A, réutiliser `@strapi/admin`** (MIT hors dossiers `ee/`) en étant compatible fil à fil avec ses endpoints internes (`/admin/*`, `/content-manager/*`, `/content-type-builder/*`, `/upload/*`, ...). Les contrats TypeScript existent (`*/shared/contracts/*.ts`).
  - Pour : toute l'UI est gratuite.
  - Contre :
    - cette API interne change à chaque mineure (entre 5.38 et 5.59 sont arrivés les sessions, les admin tokens, MCP, des endpoints IA) : on courrait derrière un projet qui ne nous doit rien ;
    - le bundle admin se construit avec Node à partir d'un projet Strapi ;
    - certains dossiers du Content Manager (preview, history) sont sous licence EE et doivent être exclus du bundle ;
    - on hérite de leurs choix UI.
- **Option B, admin maison** (SPA React embarquée dans le binaire via `embed`), qui couvre les mêmes fonctions sur une API admin à nous, documentée et versionnée.
  - Pour : binaire unique, API stable, MIT intégral, conventions UI maîtrisées (mobile-first, etc.).
  - Contre : c'est le plus gros chantier du projet, à peu près autant que tout le serveur.
- **Recommandation : B**, mais après le moteur headless. Le moteur et l'API de contenu n'en dépendent pas, donc la décision peut attendre la phase 2. En revanche, **l'API de contenu publique (REST, GraphQL) doit être compatible fil à fil avec Strapi** : c'est ce qui permet de brancher un front existant sans toucher une ligne.

### D2. Code utilisateur (lifecycles, controllers, policies, middlewares, cron, config)

Chez Strapi, tout cela est du JS/TS chargé avec `require()`. Trois niveaux, cumulables :

1. **Déclaratif** : schémas JSON (format Strapi inchangé), routes, configuration, webhooks.
2. **Mode framework Go** : grapi s'importe comme une bibliothèque, et le projet est un `main.go` qui enregistre ses hooks, controllers, services, policies et routes en Go, compilés dans le binaire. C'est le modèle PocketBase. C'est la voie performante et typée.
3. **Hooks JS embarqués** : esbuild (Go) transpile le TS et bundle les dépendances npm en JS pur, puis goja l'exécute. Ça permet de reprendre les fichiers `config/*.ts`, `lifecycles.ts`, des middlewares simples, avec un helper `env()` fourni.
   - Limites : pas d'API Node (`fs`, `net`, `crypto` natif) hors polyfills, et pas de modules natifs.
   - Performances : goja est environ 10 à 20 fois plus lent que V8, ce qui suffit pour des hooks, pas pour du calcul.
   - Exemple concret : dans dev-freelance, `slugify` et `uuid` passeraient, mais pas le client Redis ni le SDK Anthropic. Ces parties-là iraient en Go.

**Recommandation : 1 + 2 en phase 1, 3 en phase 3** (c'est un accélérateur de migration, pas le cœur).

### D3. Compatibilité avec une base Strapi existante

Proposition : grapi ouvre **directement** une base Strapi 5 (mêmes noms de tables et colonnes, mêmes tables internes `strapi_*`, mêmes hash bcrypt, même format de documentId). Migrer, c'est alors changer de binaire, sans transfert de données.

- Coût : reproduire l'algorithme de nommage (troncature à 55 caractères + hash) et le modèle de stockage exact. C'est déjà nécessaire pour être fidèle.
- Validation : sur une copie de la base de dev-freelance, sur le homelab.
- **Recommandation : oui**, au minimum pour Postgres et SQLite. MySQL en phase 3.

### D4. CGO pour les images

libvips via govips impose CGO, donc un binaire lié dynamiquement (image Docker avec libvips). Le Go pur (`disintegration/imaging` + `golang.org/x/image`) produit un binaire statique, mais il est plus lent et n'encode pas le WebP ni l'AVIF.

**Recommandation** : interface `ImageProcessor`, implémentation vips par défaut (Docker), implémentation Go pure en repli derrière un build tag.

### D5. Nom et marque

« Strapi » est une marque de Strapi Solutions SAS. Une description du type « A Strapi-compatible headless CMS written in Go » relève d'un usage descriptif, mais il faut éviter de laisser croire à un produit officiel. Ajouter au README : « Not affiliated with or endorsed by Strapi Solutions SAS ». Pas de logo Strapi, et pas de nom de paquet ou de domaine contenant « strapi ».

### D6. Clean-room

Les fonctionnalités ne sont pas protégeables, le code oui. Règles :

- on lit le code MIT de Strapi comme spécification (c'est autorisé), mais on ne le traduit pas ligne à ligne ;
- on ne lit **jamais** le code sous licence Enterprise. Les fonctions payantes se spécifient à partir de la documentation publique et du comportement observé. La licence EE couvre plus que les dossiers `ee/` (vérifié sur les fichiers `LICENSE` de la 5.59) :
  - `core/admin/ee/`, `core/core/src/ee/` ;
  - les paquets entiers `core/review-workflows/` et `core/content-releases/` ;
  - `core/content-manager/{admin,server}/src/history/` et `core/content-manager/{admin,server}/src/preview/`, preview simple comprise ;
- chaque ticket « XTR » le rappelle.

Ces règles sont détaillées dans `SETUP-003`.

## 5. Risques

| Risque | Impact | Parade |
|---|---|---|
| Sémantique des relations et du draft & publish mal reproduite | Données incohérentes | Banc de compatibilité (`SETUP-002`) : mêmes requêtes envoyées à Strapi 5.59 et à grapi, diff JSON, avant chaque merge |
| Strapi évolue (5.60, 6.x) | Écart de compatibilité | On fige une version de référence par release de grapi, et le banc dit exactement ce qui diverge |
| Volume de l'admin | Délai | Phase 2 dédiée, le moteur est utilisable sans admin dès la phase 1 (API + CLI) |
| Performance de goja | Hooks JS lents | Hooks JS réservés à la logique légère, Go pour le reste |
| Écosystème de plugins npm | Pas de reprise des plugins tiers | Assumé : plugins Go (compilés) ou JS légers |

## 6. Phasage proposé

| Phase | Contenu | Jalon de sortie |
|---|---|---|
| 0 | Repo, CI, banc de compatibilité, règles clean-room, décisions D1 à D6 | Banc qui tourne contre Strapi 5.59 seul |
| 1 | Moteur headless : schémas, base, Document Service, API REST, users-permissions, API tokens, médias locaux, config, mode framework Go, CLI de base | **Le front Go de dev-freelance tourne contre grapi sur une copie de sa base Strapi, réponses identiques** |
| 2 | Admin (auth admin, RBAC, Content Manager, CTB, Media Library, Settings), i18n, e-mail, webhooks, cron | Gérer dev-freelance entièrement depuis l'admin grapi |
| 3 | GraphQL, OpenAPI, S3, Cloudinary, OAuth, export/import/transfer, hooks JS, plugins, MySQL | Parité Community |
| 4 | SSO, audit logs, content history, review workflows, releases | Parité Enterprise, en MIT |

Le Go doit aussi tenir sa promesse de ressources. Le banc mesure le débit et la mémoire face à Strapi (référence dev-freelance du 2026-10-22, 2 CPU : Bun 209 req/s pour 272 Mo au repos, Node 154 req/s pour 430 Mo).
