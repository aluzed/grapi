---
status: todo
---

# AUTH-009 : Transfer tokens

## Contexte
Les transfer tokens autorisent `transfer` (`CLI-005`) à pousser ou tirer des données vers une instance distante, sur la socket `/admin/transfer/runner/push|pull`. Ils ont leurs propres permissions : `push` et `pull`.

## Objectif
La gestion des transfer tokens, symétrique des API tokens.

## Périmètre
- Modèles `admin::transfer-token` (name, description, accessKey, lastUsedAt, expiresAt, lifespan) et `admin::transfer-token-permission` (action `push` ou `pull`).
- HMAC-SHA512 avec `admin.transfer.token.salt`, plus une copie chiffrée comme dans `AUTH-008`.
- Endpoints admin : CRUD `/admin/transfer/tokens` et `/:id/regenerate`.
- Refus si `server.transfer.remote.enabled` est faux : les endpoints et la socket ne sont alors pas montés.
- Stratégie d'auth dédiée à la socket de transfert.

## Hors périmètre
- Le protocole de transfert lui-même (`CLI-005`).

## Critères d'acceptation
- [ ] Un token `pull` seul ne peut pas pousser (403 à l'init de la socket).
- [ ] **Compat DB (D3)** : un transfer token créé par Strapi 5.59 est accepté.
- [ ] Avec `transfer.remote.enabled: false`, les routes répondent 404.

## Notes techniques
- Réutiliser le code crypto de `AUTH-008` (même module, un sel différent).

## Référence Strapi
- `core/admin/server/src/services/transfer/token.ts`, `core/admin/server/src/routes/transfer.ts`
- `core/admin/server/src/strategies/` (transfer)

## Dépendances
- `AUTH-008`

## Fichiers
- `internal/auth/transfertoken/{model.go,service.go,strategy.go,routes.go}`
