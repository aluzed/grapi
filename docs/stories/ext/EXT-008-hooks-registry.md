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
- [ ] Tests des 4 sémantiques, y compris erreurs et annulation par `context`.
- [ ] Un plugin peut s'abonner à `afterSync` pour créer des données de seed.

## Notes techniques
- Génériques Go, pas d'`interface{}` dans l'API publique.

## Référence Strapi
- `core/utils/src/hooks.ts`, `core/core/src/registries/hooks.ts`

## Dépendances
- `SCH-005`

## Fichiers
- `hook/registry.go`, `hook/kinds.go`
