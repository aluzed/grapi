# EXT-004 : Système de plugins et extensions de plugins

## Contexte
Les plugins Strapi apportent content types, routes, controllers, services, policies, middlewares, config et une partie admin. Ils s'activent dans `config/plugins` et peuvent être étendus par le projet (`src/extensions`). En Go, un plugin tiers ne peut pas être chargé dynamiquement de façon fiable : on retient des plugins compilés (enregistrés par `init()` ou explicitement, modèle Caddy) et, plus tard, des plugins JS légers (`EXT-003`).

## Objectif
Une interface de plugin Go et le même mécanisme d'activation, de configuration et d'extension que Strapi.

## Périmètre
- Interface : `Name()`, `Register(app)`, `Bootstrap(app)`, `Destroy(app)`, `DefaultConfig()`, `ValidateConfig(cfg)`, ainsi que les content types, routes, controllers, services, policies et middlewares fournis.
- Plugins internes toujours actifs : content-manager, content-type-builder, email, upload, i18n. Plugins officiels optionnels : users-permissions, graphql, documentation, sentry.
- `config/plugins` : `{nom: {enabled, config}}` ; config utilisateur fusionnée en profondeur sur la config par défaut, puis validée.
- Extensions : `src/extensions/<plugin>/content-types/<ct>/schema.json` fusionné (superficiellement) sur le schéma du plugin, ce qui permet par exemple d'ajouter des champs à `plugin::users-permissions.user` ; extension Go `app.ExtendPlugin("users-permissions", func(p *Plugin))`.
- Noms en kebab-case, chaque phase exécutée une seule fois par plugin.
- Partie admin d'un plugin : déclarée par le plugin, consommée par `ADM-010`.

## Hors périmètre
- Les plugins npm Strapi tiers (non repris, voir les risques de `feasibility.md`).

## Critères d'acceptation
- [ ] Un plugin d'exemple (`examples/plugins/hello`) ajoute un content type, une route et un service, activable et désactivable par config.
- [ ] Le champ supplémentaire du user de dev-freelance (via `src/extensions`) est pris en compte comme chez Strapi.
- [ ] Config de plugin invalide : erreur au démarrage qui nomme le plugin et la clé.

## Notes techniques
- Pas de `plugin.Open` (paquet `plugin` de Go) : non portable, exige les mêmes versions de dépendances au bit près.
- Out-of-process (gRPC, `hashicorp/go-plugin`) ou WASM : à étudier seulement si un vrai besoin de plugins tiers non compilés apparaît.

## Référence Strapi
- `core/core/src/loaders/plugins/index.ts`, `get-enabled-plugins.ts`
- `core/core/src/domain/module/index.ts`

## Dépendances
- `SCH-004`, `SCH-005`, `EXT-001`, `EXT-002`

## Fichiers
- `plugin.go` (interface publique), `internal/plugin/loader.go`, `internal/plugin/extensions.go`, `examples/plugins/hello/`
