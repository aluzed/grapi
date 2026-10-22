---
status: in-progress
---

# CLI-003 : Commandes d'inspection et d'administration (`*:list`, `admin:*`, `configuration:dump|restore`)

## Contexte
La CLI Strapi fournit des listes (routes, services, policies...), la gestion des admins hors interface (indispensable quand on a perdu l'accès) et un dump et restore de la configuration du core store.

## Objectif
Les mêmes commandes, qui passent par les mêmes services que l'API.

## Périmètre
- Listes, en tableau ou en JSON (`--json`) : `routes:list`, `services:list`, `controllers:list`, `policies:list`, `middlewares:list`, `hooks:list`, `components:list`, `content-types:list`.
- Admins :
  - `admin:create-user` (email, password, firstname, lastname, en interactif ou via flags) ;
  - `admin:reset-user-password` ;
  - `admin:list-users`, `admin:delete-user`, `admin:active-user`, `admin:block-user`.
- Configuration :
  - `configuration:dump [-f fichier] [--pretty]` : toutes les clés du core store (`DB-010`) qui commencent par `plugin_`, au format JSON ;
  - `configuration:restore -f fichier -s replace|merge|keep`.
- `openapi generate -o fichier` (`PLG-005`).
- `rename:field <uid> <ancien> <nouveau>` : renomme un attribut dans le schéma, crée la migration de renommage et met à jour la configuration des vues.

## Hors périmètre
- `telemetry:*` (pas de télémétrie, `EXT-007`) et `enterprise install` (sans objet).

## Critères d'acceptation
- [ ] `admin:reset-user-password` fonctionne sur une base Strapi 5.59 existante, sans rien casser d'autre (règle projet : on ne touche jamais à la table des admins en dehors de cette opération ciblée).
- [ ] `configuration:dump` sur une base Strapi produit un fichier que `strapi configuration:restore` accepte, et inversement (banc `SETUP-002`).
- [ ] `routes:list` affiche méthode, chemin, handler et policies pour toutes les routes, y compris celles des plugins.
- [ ] Le mot de passe saisi en interactif n'est jamais affiché ni loggé.

## Avancement
- Fait : `permissions:list|grant|revoke <rôle> [actions]` sur les rôles users-permissions et `api-token:*` (`AUTH-008`), qui suffisent à administrer un projet sans admin. Test : `cmd/grapi/tokens_test.go` (projet neuf, permission accordée puis retirée, token custom).
- `grapi openapi [-o fichier]` écrit le document OpenAPI du content API sans démarrer le serveur (`PLG-005`).
- `grapi routes:list` : la table des routes au format de Strapi (`HEAD|GET` pour les GET, comme koa), test `TestRoutesList`. Strapi y liste aussi les couches de son routeur (fichiers publics, admin), que grapi sert hors table.
- `grapi configuration:dump [-f fichier] [-p]` et `configuration:restore [-f fichier] [-s replace|merge|keep]` : les entrées `plugin_*` du core store, triées par clé, valeurs brutes, et les trois importeurs de Strapi avec leurs statistiques (`TestConfigurationDumpRestore`).
- `grapi report` indique aussi GraphQL (on/off), l'accès OpenAPI et les providers d'upload et d'e-mail.
- `grapi content-types:list` et `components:list` (tableau `Name` de Strapi), `TestListCommands`.
- `grapi middlewares:list` et `hooks:list` : les middlewares que grapi implémente (sous les noms de Strapi) et les hooks intégrés. Strapi liste aussi ses middlewares absents de grapi (compression, session, favicon...) et ceux de l'admin.
- Reste : `policies:list` (les policies intégrées de Strapi sont celles de l'admin), `controllers:list`, `services:list`, les commandes admin, `rename:field`.

## Notes techniques
- Ces commandes démarrent l'application sans serveur HTTP (registres et base seulement).
- Dans `admin:create-user`, le mot de passe se valide avec les mêmes règles que `AUTH-007`.

## Référence Strapi
- `core/strapi/src/cli/commands/` (`*-list`, `admin/*`, `configuration/*`, `rename-field*`)

## Dépendances
- `CLI-001`, `AUTH-007`, `DB-010`, `SCH-005`

## Fichiers
- `internal/cli/{list.go,admin.go,configuration.go,renamefield.go}`
