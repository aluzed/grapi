---
status: done
---

# AUTH-008 : API tokens et admin tokens

## Contexte
Les API tokens donnent un accès serveur à serveur au Content API. Strapi stocke un HMAC du token, et non le token lui-même, ainsi qu'une copie chiffrée qui permet de le réafficher dans l'admin. Strapi 5 a ajouté les *admin tokens* (`kind: admin`), qui appellent l'API admin avec les permissions de leur propriétaire.

## Objectif
Les mêmes tokens, avec le même format de stockage, pour qu'un token créé par Strapi fonctionne sur grapi (D3).

## Périmètre
- Modèle `admin::api-token` : name, description, type (`read-only`, `full-access`, `custom`), accessKey (HMAC), encryptedKey, lastUsedAt, expiresAt, lifespan, kind (`content-api` ou `admin`), permissions (`admin::api-token-permission`, action par token custom).
- Génération : 128 octets aléatoires en hexadécimal.
- Stockage :
  - `accessKey = HMAC-SHA512(token, admin.apiToken.salt)` en hexadécimal ;
  - `encryptedKey` = AES-256-GCM avec `admin.secrets.encryptionKey`. Il n'est disponible que si la clé est configurée, sinon on ne peut plus jamais réafficher le token.
- Durées de vie : illimitée, 7, 30 ou 90 jours (`lifespan` en ms, `expiresAt` calculé).
- Endpoints admin : CRUD `/admin/api-tokens`, `POST /admin/api-tokens/:id/regenerate`, et la même chose pour `/admin/admin-tokens`, plus `/:id/owner-permissions`.
- Stratégie `api-token` sur le Content API :
  - token `read-only` : uniquement les actions `find` et `findOne` ;
  - token `full-access` : tout ;
  - token `custom` : uniquement ses permissions ;
  - `lastUsedAt` est mis à jour au plus une fois par heure.
- Stratégie `admin-token` : l'appelant prend les permissions de son propriétaire, réduites à celles du token.
- Un token expiré donne 401.

## Hors périmètre
- Transfer tokens (`AUTH-009`).

## Critères d'acceptation
- [x] **Compat DB (D3)** : avec le même `API_TOKEN_SALT`, un token créé par Strapi 5.59 authentifie sur grapi. Ce cas est dans le banc `SETUP-002` : c'est le token full-access utilisé pour les tests dev-freelance.
- [x] Avec `ENCRYPTION_KEY`, un token créé par Strapi est réaffiché en clair par grapi (déchiffrement AES-256-GCM, même format IV et tag que Strapi).
- [x] Un token read-only reçoit 403 sur `POST /api/posts` (banc).
- [x] La régénération invalide l'ancien token (intégration).
- [x] Les comparaisons de tokens sont à temps constant (`crypto/subtle`).

## Avancement
- Gestion depuis l'admin (`/admin/api/v1/api-tokens`, page Paramètres > API tokens) : création, édition, permissions personnalisées, régénération, révocation. Schéma de clé dans `internal/auth/apitoken`, partagé avec la CLI.
- Fait : stratégie `api-token` sur le Content API (read-only, full-access, custom, expiration en 401). Création, liste et suppression en CLI (`grapi api-token:create|list|delete`, `cmd/grapi/tokens.go`) : 128 octets aléatoires, `accessKey` en HMAC-SHA512 avec le sel, `encryptedKey` au format du service de chiffrement de l'admin (`v1:iv:data:tag`, AES-256-GCM sur le SHA-256 de `admin.secrets.encryptionKey`, déchiffré par Node à l'identique), durées de vie 7, 30 ou 90 jours.
- `lastUsedAt` mis à jour à l'usage comme `updateLastUsedAt` (au plus une fois par heure), vérifié dans `TestTokensAndPermissions`.
- `grapi api-token:regenerate --id N` : nouvelle clé (HMAC et chiffrement comme à la création), l'ancienne cesse de fonctionner (`TestTokensAndPermissions`).

- Preuves (2026-10-23) : en mode `grapi`, tout le corpus s'authentifie avec les tokens `full` et `read` créés par Strapi (même `API_TOKEN_SALT`) ; le token `compat-full` chiffré par Strapi est réaffiché en clair par l'admin de grapi, identique à celui que Strapi a écrit (`remote.sh`) ; `basic / create-readonly-forbidden` donne le même 403 que Strapi ; `TestAdminAPITokens` vérifie que l'ancienne clé répond 401 après la régénération. Un token n'est jamais comparé : il est cherché par son HMAC-SHA512 en base, comme chez Strapi ; la signature des JWT passe par `hmac.Equal`, les mots de passe par bcrypt.
- Les endpoints admin (création, édition, régénération, révocation) sont faits (`internal/adminapi/tokens.go`).

## Notes techniques
- Vérifier dans `encryption.ts` le format exact de la copie chiffrée (concaténation IV, tag et ciphertext, encodage) et le reproduire. Prévoir un TU sur une valeur générée par Strapi.
- Ne jamais logger un token, ni en entier ni en partie.

## Référence Strapi
- `core/admin/server/src/services/api-token.ts`, `core/admin/server/src/services/encryption.ts`
- `core/admin/server/src/strategies/api-token.ts`, `.../admin-token.ts`
- `core/admin/server/src/routes/api-tokens.ts`, `core/admin/server/src/services/constants.ts`

## Dépendances
- `AUTH-001`, `AUTH-007`, `API-002`, `EXT-001`

## Fichiers
- `internal/auth/apitoken/{model.go,service.go,crypto.go,strategy.go,routes.go,crypto_test.go}`
