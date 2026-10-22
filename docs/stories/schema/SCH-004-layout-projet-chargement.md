---
status: done
---

# SCH-004 : Layout de projet et chargement

## Contexte
Un projet Strapi suit une arborescence de conventions. Pour qu'un projet existant fonctionne avec grapi (`MIG-001`), grapi lit la même arborescence. Les fichiers JS et TS de cette arborescence (controllers, services, lifecycles) ne sont pas exécutés en phase 1 : ils sont détectés et signalés (D2).

## Objectif
Charger un projet depuis un répertoire racine et produire l'ensemble des schémas (APIs, composants, extensions, plugins) prêts à être validés puis enregistrés.

## Périmètre
- **Racine du projet** : flag `--dir` ou variable `GRAPI_APP_DIR`, par défaut le répertoire courant.
- **`src/api/<api>/content-types/<ct>/schema.json`** pour chaque API.
  - Le nom d'API est normalisé en kebab-case.
  - `<ct>` doit égaler `info.singularName` (à vérifier dans le loader Strapi).
- **`src/components/<catégorie>/<nom>.json`**.
- **`src/extensions/<plugin>/content-types/<ct>/schema.json`** : fusion **superficielle** dans le schéma du plugin. Les clés de premier niveau remplacent, et `attributes` remplace en bloc. Vérifier ce comportement exact sur la référence.
- **Schémas des plugins intégrés** (users-permissions, upload, i18n, admin) : embarqués dans le binaire et chargés avant les extensions.
- **Inventaire du code utilisateur** non exécuté en phase 1 :
  - `src/index.{js,ts}`, `src/api/*/{controllers,services,routes,policies,middlewares}/*` ;
  - `content-types/*/lifecycles.{js,ts}`, `src/middlewares`, `src/policies` ;
  - `src/extensions/*/strapi-server.*`, `config/*.{js,ts}`.
  - Ces fichiers sont listés dans un rapport (`grapi doctor`) avec l'action conseillée : réécrire en hook Go (`EXT-002`) ou attendre les hooks JS (`EXT-003`).
- **Routes déclaratives** : les `src/api/*/routes/*.json` éventuels sont lus. Les routes `.js` ou `.ts` qui se limitent à `createCoreRouter(uid)` sont reconnues par motif simple et traitées comme le routeur par défaut. Les autres sont signalées.
- **Rechargement** : `Load()` est idempotent et réutilisable par le rechargement à chaud (`CLI-001` develop, `ADM-006` CTB).

## Hors périmètre
- L'exécution du code JS et TS (`EXT-003`).
- La configuration (`EXT-001`).

## Critères d'acceptation
- [ ] Chaque fixture du banc et une copie du projet dev-freelance (`apps/backend`) se chargent sans erreur, avec le bon nombre de content types et de composants.
- [ ] Une extension de schéma de `plugin::users-permissions.user` (ajout d'un attribut) est fusionnée, et l'attribut apparaît dans le registre.
- [ ] `grapi doctor` sur dev-freelance liste `src/index.ts`, les lifecycles restants et les controllers custom, chacun avec son action conseillée.
- [ ] Un répertoire `content-types/<x>` sans `schema.json` produit une erreur claire.
- [ ] Charger deux fois le même projet donne exactement le même résultat (déterminisme, ordre stable).

## Notes techniques
- Le parcours de répertoires doit être trié : `os.ReadDir` trie déjà par nom, mais il faut le garder explicitement pour le déterminisme.
- `fs.FS` en entrée, pour tester sur des `fstest.MapFS` et embarquer les schémas des plugins.
- La reconnaissance des routeurs « triviaux » est volontairement simple (regex sur `createCoreRouter('<uid>')` sans second argument). Au-delà, on signale et on ne devine pas.

## Référence Strapi
- `core/core/src/loaders/apis.ts`, `core/core/src/loaders/components.ts`, `core/core/src/loaders/plugins/index.ts`
- `core/core/src/loaders/src-index.ts`

## Dépendances
- `SCH-001`, `SCH-002`.

## Fichiers
- `internal/registry/loader.go`, `internal/registry/loader_extensions.go`, `internal/registry/doctor.go`, `internal/plugins/*/schemas/*.json` (embarqués)
