# PLG-007 : Serveur MCP

## Contexte
Strapi 5 récents embarquent un serveur MCP (Model Context Protocol), activé par `server.mcp.enabled`. Il expose des outils dérivés des content types (lister, lire, créer, mettre à jour des documents), pour qu'un agent IA puisse manipuler le contenu.

## Objectif
Un serveur MCP qui expose des outils générés à partir des content types, soumis aux mêmes permissions que l'API.

## Périmètre
- Transport HTTP streamable sur un chemin configurable (défaut `/mcp`), désactivé par défaut.
- Outils générés par content type : `find`, `findOne`, `create`, `update`, `delete`, `publish` et `unpublish` (si D&P), avec un schéma JSON d'entrée dérivé des attributs.
- Authentification par API token ou admin token (`AUTH-008`), sous les mêmes permissions.
- Outils de découverte : liste des content types et de leurs schémas.

## Hors périmètre
- Les outils d'administration (création de content types via MCP).

## Critères d'acceptation
- [ ] Un client MCP (Claude Code sur le homelab) liste les outils et crée un brouillon de post via MCP.
- [ ] Un token read-only ne voit que les outils de lecture.
- [ ] Désactivé par défaut : `/mcp` répond 404 sans configuration.

## Notes techniques
- SDK Go officiel `modelcontextprotocol/go-sdk`.
- Vérifier dans `core/core/src/services/mcp/` et `content-manager/server/src/mcp/` les noms d'outils de Strapi, pour les reprendre à l'identique quand c'est pertinent.

## Référence Strapi
- `core/core/src/services/mcp/`
- `core/content-manager/server/src/mcp/`

## Dépendances
- `DOC-001`, `AUTH-008`, `SCH-005`

## Fichiers
- `internal/plugins/mcp/{server.go,tools.go,schema.go}`
