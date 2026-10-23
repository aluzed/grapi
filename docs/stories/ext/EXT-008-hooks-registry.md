---
status: in-progress
---

# EXT-008 : Registre de hooks (series, waterfall, parallel, bail)

## Contexte
Strapi expose des hooks nommés que les plugins créent et auxquels d'autres plugins s'abonnent (par exemple `strapi::content-types.beforeSync` et `afterSync`). Quatre types : série asynchrone, cascade (chaque handler transforme la valeur), parallèle, et arrêt au premier résultat (bail).

## Objectif
Un registre de hooks typés en Go, avec les quatre sémantiques.

## Périmètre
- `Series[T]` : handlers exécutés dans l'ordre, erreur = arrêt.
- `Waterfall[T]` : la sortie d'un handler devient l'entrée du suivant.
- `Parallel[T]` : handlers concurrents, on attend tous les résultats ; première erreur renvoyée.
- `Bail[T]` : arrêt au premier handler qui renvoie une valeur non vide.
- Registre namespacé : `app.Hooks().Get("strapi::content-types.beforeSync")`, avec `Register`, `Delete`.
- Hooks internes : `content-types.beforeSync`, `content-types.afterSync`.

## Hors périmètre
- Les lifecycles de base (`DB-009`) et les middlewares de documents (`DOC-004`), qui ont leurs propres mécanismes.

## Critères d'acceptation
- [x] Tests des 4 sémantiques, y compris erreurs et annulation par `context`.
- [x] Un plugin peut s'abonner à `afterSync` pour créer des données de seed.

## Avancement
- Paquet public `hook` (`hook/registry.go`) : `Series`, `Waterfall`, `Parallel` et `Bail` génériques, désinscription par la fonction que rend `Register`, annulation par `context` ; `Registry` (`Register`, `Get`, `Delete`) exposé par `app.Hooks()`. Tests `hook/registry_test.go` (avec `-race`).
- `strapi::content-types.beforeSync` et `afterSync` (`Parallel[SyncEvent]`, comme Strapi) : `beforeSync` avant la synchronisation du schéma, `afterSync` une fois les services prêts, pour qu'un handler puisse créer des données (`TestPhases`). Strapi appelle `afterSync` juste après la synchronisation, avant ses services ; l'écart est voulu.
- `SyncEvent.OldContentTypes` : les uids que Strapi a enregistrés à son dernier démarrage (`strapi_content_types_schema` du core store), vide sur une base qu'il n'a jamais ouverte (`TestSyncHookOldContentTypes`).
- Reste : les schémas complets d'avant (grapi ne donne que les uids), et l'écriture de `strapi_content_types_schema` par grapi : liée aux migrations de changement de schéma, suivie dans `DOC-008` (le schéma stocké est le `strapi.contentTypes` complet, types EE compris ; une écriture approximative déclencherait chez Strapi une migration destructive).

## Notes techniques
- Génériques Go, pas d'`interface{}` dans l'API publique.

## Référence Strapi
- `core/utils/src/hooks.ts`, `core/core/src/registries/hooks.ts`

## Repris d'autres tickets
- `DB-003` : brancher les hooks `beforeSync` et `afterSync` de la synchro du schéma (`schemasync.Sync`), et appeler après une synchro `CHANGED` le repair `schemasync.RemoveOrphanMorphTypes` pour `component_type` puis `related_type`, comme `Strapi.ts`.

## Dépendances
- `SCH-005`

## Fichiers
- `hook/registry.go`, `hook/kinds.go`
