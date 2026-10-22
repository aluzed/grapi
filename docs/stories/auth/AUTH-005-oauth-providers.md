# AUTH-005 : Providers OAuth

## Contexte
users-permissions propose la connexion via 17 providers externes. Le flux passe par `GET /api/connect/:provider`, qui redirige vers le provider, puis par le retour sur `/api/auth/:provider/callback`. Le front reçoit ensuite `access_token` et appelle `GET /api/auth/:provider/callback?access_token=...`, qui renvoie `{jwt, user}`.

## Objectif
Reproduire ce flux et les providers, avec la même configuration (clé `grant` du core store) et le même mapping de profil.

## Périmètre
- Providers : email (local), discord, facebook, google, github, microsoft, twitter, instagram, vk, twitch, linkedin, cognito, reddit, auth0, cas, patreon, keycloak.
- Configuration par provider : `enabled`, `key`, `secret`, `callback` (URL du front), `scope`, plus les champs spécifiques (subdomain pour auth0, cognito et keycloak).
- Création ou rattachement de l'utilisateur par e-mail. Le champ `provider` est renseigné. Si `unique_email` est actif et que l'e-mail existe déjà avec un autre provider, l'appel est refusé.
- Validation du callback (`callback.validate`) : l'URL de retour doit correspondre à l'origine et au chemin configurés, contre l'open redirect.
- Vérification des `id_token` par JWKS quand le provider en fournit un (Google, Microsoft, Cognito, Keycloak).
- Enregistrement de providers supplémentaires en Go (mode framework, `EXT-002`).

## Hors périmètre
- SSO admin (`XTR-001`).

## Critères d'acceptation
- [ ] Flux GitHub et Google testés de bout en bout sur une app de test (homelab), avec la même réponse `{jwt, user}` que Strapi.
- [ ] Un `callback` hors de l'origine autorisée donne une erreur (TU).
- [ ] Le mapping de profil (username, email) est identique à Strapi pour chaque provider : TU sur des fixtures de réponses de profil.
- [ ] **Compat DB (D3)** : la configuration `grant` d'une base Strapi existante est lue telle quelle.

## Notes techniques
- `golang.org/x/oauth2` (+ endpoints), `coreos/go-oidc/v3` pour les id_token, et `markbates/goth` à évaluer pour aller plus vite sur les providers exotiques.
- Twitter (OAuth 1.0a) et CAS ne passent pas par oauth2 : prévoir une implémentation dédiée.
- Le paramètre `state` doit être signé et vérifié (cookie de session, `API-008`).

## Référence Strapi
- `plugins/users-permissions/server/src/services/providers.js`, `.../providers-registry.js`
- `plugins/users-permissions/server/src/utils/oauth-connect/`, `.../verify-jwt-with-jwks.js`
- `plugins/users-permissions/server/src/controllers/auth.js` (`connect`, `callback`)

## Dépendances
- `AUTH-002`, `API-008` (session)

## Fichiers
- `internal/auth/oauth/{registry.go,flow.go,providers/*.go,jwks.go}`
