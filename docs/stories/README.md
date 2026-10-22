# grapi : backlog des stories

Les tickets découlent de [`../feasibility.md`](../feasibility.md), qui est l'étude de faisabilité et contient les décisions D1 à D6. La référence fonctionnelle est **Strapi 5.59.0** : un clone se trouve dans `../../../strapi` (soit `~/Documents/dev/strapi`).

Règle d'or : **mêmes fonctionnalités, mêmes comportements observables, implémentation libre**. L'API de contenu publique (REST, GraphQL) est compatible fil à fil avec Strapi. L'admin et son API ne le sont pas forcément (décision D1).

## Arborescence cible du code

```
cmd/grapi/            binaire + CLI (cobra)
grapi.go              point d'entrée du mode framework (App, Register/Bootstrap/Destroy)
schema/               modèle de schéma public (content types, attributs, composants)
document/             Document Service public (utilisable depuis les hooks Go)
hook/                 types publics des hooks (lifecycles, middlewares, policies)
internal/config/      configuration déclarative + env()
internal/registry/    registres namespacés (api::, plugin::, admin::, global::)
internal/db/          dialectes, nommage, métadonnées, synchro, migrations, requêtes, relations
internal/httpapi/     serveur, routage, qs, paramètres, sanitize, core API, erreurs, middlewares, policies
internal/auth/        permissions, users-permissions, sessions, tokens, admin, OAuth, rate limit
internal/media/       upload, images, providers
internal/plugins/     i18n, email, graphql, openapi, sentry, mcp
internal/jsrt/        hooks JS (esbuild + goja), phase 3
internal/transfer/    export / import / transfer
internal/extra/       fonctions Enterprise chez Strapi (SSO, audit, history, workflows, releases), tables grapi_*
admin/                SPA admin (si D1 = B), embarquée via embed
compat/               banc de compatibilité contre Strapi de référence
```

## Répartition

| Dossier | Préfixe | Objet |
|---|---|---|
| [`setup/`](./setup) | `SETUP` | Repo, CI, banc de compatibilité, clean-room, packaging |
| [`schema/`](./schema) | `SCH` | Modèle de schéma, chargement du projet, registres, custom fields |
| [`db/`](./db) | `DB` | Dialectes, nommage, synchro, migrations, requêtes, relations, composants, lifecycles |
| [`document/`](./document) | `DOC` | Document Service, draft & publish, validation, middlewares, événements |
| [`api/`](./api) | `API` | Serveur HTTP, routage, paramètres REST, sanitize, core API, erreurs, middlewares, policies |
| [`auth/`](./auth) | `AUTH` | Permissions, utilisateurs finaux, sessions, OAuth, admin, tokens |
| [`ext/`](./ext) | `EXT` | Config, mode framework Go, hooks JS, plugins, cron, webhooks, logs |
| [`media/`](./media) | `MEDIA` | Upload, images, providers, dossiers |
| [`plugins/`](./plugins) | `PLG` | i18n, e-mail, GraphQL, OpenAPI, Sentry, MCP |
| [`admin/`](./admin) | `ADM` | Admin : Content Manager, CTB, Media Library, Settings |
| [`cli/`](./cli) | `CLI` | CLI, générateurs, export/import/transfer, types TS |
| [`extra/`](./extra) | `XTR` | Fonctions payantes chez Strapi, réimplémentées en clean-room sous MIT |
| [`migration/`](./migration) | `MIG` | Passer un projet Strapi existant sur grapi, recette dev-freelance |

## Index des tickets

### setup
- `SETUP-001` Bootstrap du repo Go (module `github.com/aluzed/grapi`, layout, lint, CI, README, LICENSE, avertissement marque)
- `SETUP-002` Banc de compatibilité contre Strapi 5.59 (mêmes requêtes, diff JSON normalisé, mesures perf et mémoire)
- `SETUP-003` Règles clean-room et licences des dépendances
- `SETUP-004` Packaging : binaire, image Docker, admin embarqué, choix CGO

### schema
- `SCH-001` Modèle de content type et attributs (format `schema.json` Strapi, uid, champs automatiques, noms réservés)
- `SCH-002` Composants et zones dynamiques (schéma)
- `SCH-003` Validation de cohérence des schémas
- `SCH-004` Layout de projet et chargement (`src/api`, `src/components`, `src/extensions`, schémas des plugins)
- `SCH-005` Registres namespacés (content types, composants, services, controllers, policies, middlewares, hooks)
- `SCH-006` Custom fields

### db
- `DB-001` Dialectes (Postgres, SQLite, MySQL plus tard), pool, transactions portées par `context`
- `DB-002` Nommage et métadonnées compatibles Strapi (tables `_lnk`, `_mph`, `_cmps`, colonnes d'ordre, troncature + hash)
- `DB-003` Synchronisation du schéma au démarrage (introspection, diff 3 voies, `strapi_database_schema`)
- `DB-004` Migrations utilisateur et internes
- `DB-005` Query builder et opérateurs de filtre (filtres imbriqués, tri profond, `_q`, count)
- `DB-006` Relations : stockage, populate, connect/disconnect/set, positions, ordre fractionnaire
- `DB-007` Persistance des composants et zones dynamiques
- `DB-008` Relations polymorphes (morph) et champs média
- `DB-009` Lifecycles de la base
- `DB-010` Core store (`strapi_core_store_settings`)
- `DB-011` Ouverture d'une base Strapi 5 existante sans migration

### document
- `DOC-001` Document Service : API et documentId
- `DOC-002` Draft & Publish (publish, unpublish, discardDraft, recâblage des relations, status, publicationFilter)
- `DOC-003` Validation des entrées et valeurs par défaut (règles levées en brouillon, appliquées à la publication, unicité)
- `DOC-004` Middlewares du Document Service
- `DOC-005` Événements `entry.*` et event hub
- `DOC-006` Résolution des relations par documentId (locale, status)
- `DOC-007` Single types

### api
- `API-001` Serveur HTTP, contexte de requête, `/_health`, arrêt propre
- `API-002` Routage déclaratif et pipeline par route (auth, policies, middlewares, handler)
- `API-003` Parseur de query string compatible `qs`
- `API-004` Paramètres REST : filters, sort, fields, populate, pagination, locale, status, strictParams
- `API-005` Sanitize et validation (entrée, sortie, requête, selon les permissions)
- `API-006` Core API générée (routes, controllers, services CRUD, format `{data, meta}`, surcharges)
- `API-007` Classes d'erreur et format d'erreur
- `API-008` Middlewares globaux (logger, errors, security, cors, poweredBy, session, query, body, favicon, public, compression, ip, responseTime)
- `API-009` Policies

### auth
- `AUTH-001` Moteur de permissions (actions, sujets, champs, locales, conditions traduites en filtres)
- `AUTH-002` Utilisateurs finaux : inscription, connexion, JWT, confirmation e-mail, mot de passe, `/users/me`, CRUD
- `AUTH-003` Sessions et refresh tokens (partagé entre admin et utilisateurs finaux)
- `AUTH-004` Rôles et permissions du Content API (Public, Authenticated, rôles custom)
- `AUTH-005` Providers OAuth
- `AUTH-006` Rate limiting des routes d'authentification
- `AUTH-007` Utilisateurs admin, rôles admin, invitations
- `AUTH-008` API tokens et admin tokens
- `AUTH-009` Transfer tokens

### ext
- `EXT-001` Configuration déclarative et `env()`
- `EXT-002` Mode framework Go (grapi comme bibliothèque)
- `EXT-003` Hooks JS/TS embarqués (esbuild + goja)
- `EXT-004` Système de plugins et extensions de plugins
- `EXT-005` Cron
- `EXT-006` Webhooks
- `EXT-007` Logs (pas de télémétrie)
- `EXT-008` Registre de hooks (series, waterfall, parallel, bail)

### media
- `MEDIA-001` Modèle File/Folder et API d'upload (Content API + admin)
- `MEDIA-002` Traitement d'image (miniature, breakpoints, optimisation, orientation, animés)
- `MEDIA-003` Interface provider et provider local
- `MEDIA-004` Provider S3 (R2, MinIO, privé, URLs signées)
- `MEDIA-005` Provider Cloudinary
- `MEDIA-006` Dossiers, actions groupées, remplacement, upload depuis URL, réglages

### plugins
- `PLG-001` i18n
- `PLG-002` E-mail : service, templates, providers
- `PLG-003` GraphQL : schéma généré dynamiquement
- `PLG-004` GraphQL : shadow CRUD, extensions, auth, policies, limites
- `PLG-005` OpenAPI et documentation (Swagger UI)
- `PLG-006` Sentry
- `PLG-007` Serveur MCP

### admin
- `ADM-001` Décision et spike : SPA Strapi ou admin maison (D1)
- `ADM-002` Shell admin : connexion, navigation, profil, page d'accueil
- `ADM-003` Content Manager : vue liste
- `ADM-004` Content Manager : vue édition (champs, composants, zones dynamiques, relations, D&P, i18n)
- `ADM-005` Content Manager : configuration des vues
- `ADM-006` Content-Type Builder (écriture des schémas, rechargement à chaud, migrations de renommage)
- `ADM-007` Media Library
- `ADM-008` Settings (webhooks, tokens, utilisateurs, rôles et RBAC, locales, users-permissions, e-mail, projet)
- `ADM-009` Preview
- `ADM-010` Extensions UI (custom fields, zones d'injection, menus)

### cli
- `CLI-001` CLI de base : `new`, `develop`, `start`, `version`
- `CLI-002` Générateurs (`generate api|content-type|controller|policy|middleware|migration`)
- `CLI-003` Commandes d'inspection et d'administration (`*:list`, `admin:*`, `configuration:dump|restore`)
- `CLI-004` Export et import (format d'archive Strapi)
- `CLI-005` Transfer distant (WebSocket push/pull)
- `CLI-006` Génération des types TypeScript pour les clients

### extra
- `XTR-001` SSO (OIDC, SAML)
- `XTR-002` Audit logs
- `XTR-003` Content history
- `XTR-004` Review workflows
- `XTR-005` Content releases (dont planifiées)

### migration
- `MIG-001` Outil de migration d'un projet Strapi (schémas, config, code utilisateur)
- `MIG-002` Recette dev-freelance : le front tourne contre grapi sur une copie de la base

## Ordre de réalisation

| Phase | Tickets | Jalon |
|---|---|---|
| 0 | `SETUP-*`, `ADM-001` (décision seulement) | Banc de compatibilité opérationnel contre Strapi 5.59 |
| 1 | `SCH-*`, `DB-001` à `DB-011`, `DOC-*`, `API-*`, `EXT-001`, `EXT-002`, `EXT-007`, `EXT-008`, `AUTH-001`, `AUTH-002`, `AUTH-004`, `AUTH-008`, `MEDIA-001` à `MEDIA-003`, `CLI-001`, `MIG-002` | dev-freelance servi par grapi, réponses identiques |
| 2 | `AUTH-003`, `AUTH-006`, `AUTH-007`, `ADM-002` à `ADM-010`, `PLG-001`, `PLG-002`, `EXT-005`, `EXT-006`, `MEDIA-006`, `CLI-002`, `CLI-003` | Gestion complète depuis l'admin |
| 3 | `PLG-003` à `PLG-007`, `MEDIA-004`, `MEDIA-005`, `AUTH-005`, `AUTH-009`, `EXT-003`, `EXT-004`, `CLI-004` à `CLI-006`, `MIG-001`, MySQL | Parité Community |
| 4 | `XTR-*` | Parité Enterprise, en MIT |

## Convention de ticket

Chaque ticket suit ce format : **Contexte**, **Objectif**, **Périmètre**, **Hors périmètre**, **Critères d'acceptation**, **Notes techniques**, **Référence Strapi** (chemins dans le clone, relatifs à `strapi/packages/`), **Dépendances**, **Fichiers**.

Chaque ticket commence par un frontmatter qui porte son statut :

```yaml
---
status: todo
---
```

Valeurs possibles : `todo`, `in-progress`, `done`. Le statut change dans le même commit que le code qui le justifie : un ticket passe à `in-progress` au premier commit qui le touche, et à `done` quand tous ses critères d'acceptation sont remplis. `make check-stories` (lancé en CI) refuse un ticket sans statut valide.

## Règles transverses

- Pas d'em-dash (U+2014), nulle part.
- Commentaires de code minimaux : uniquement le *pourquoi* non évident, jamais de numéro de ticket.
- Clean-room (`SETUP-003`) : le code MIT de Strapi sert de spécification, il ne se traduit pas ligne à ligne. On ne lit jamais les dossiers `ee/`.
- Tout comportement visible depuis l'API de contenu a un cas dans le banc de compatibilité (`SETUP-002`).
- Tests unitaires sur chaque brique et tests d'intégration sur Postgres et SQLite.
- La doc est mise à jour dans le même changement que le code.
- Pas de Docker sur le Mac de dev : builds et bancs sur le homelab.
