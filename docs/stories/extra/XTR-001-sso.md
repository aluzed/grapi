# XTR-001 : SSO (OIDC, SAML)

> **Clean-room** : fonction sous licence Enterprise chez Strapi. Spécification rédigée uniquement à partir de la documentation publique de Strapi et du comportement observable. Il est **interdit d'ouvrir** les dossiers `ee/`, les paquets `core/review-workflows` et `core/content-releases`, ainsi que `core/content-manager/{admin,server}/src/{history,preview}` (voir `SETUP-003`). Dans grapi, la fonction est sous MIT, sans clé de licence ni limite (nombre de workflows, de releases, durée de rétention...).

## Contexte
Strapi Enterprise permet aux administrateurs de se connecter via un fournisseur d'identité, avec des providers préconfigurés et des providers personnalisés. On peut créer automatiquement un compte admin à la première connexion avec un rôle par défaut, et verrouiller un rôle sur le SSO (« SSO lock » : les utilisateurs de ce rôle ne peuvent plus se connecter par mot de passe).

## Objectif
Connexion SSO à l'admin grapi via OIDC (générique et presets) et, en option, via SAML 2.0.

## Périmètre
- **Provider OIDC générique** : issuer (avec découverte `.well-known/openid-configuration`), client id, client secret, scopes, mapping des claims (e-mail, prénom, nom), PKCE.
- **Presets** : Google, Microsoft (Entra ID, tenant configurable), GitHub (OAuth2 non-OIDC : profil et e-mail via l'API), Keycloak, Okta, Auth0.
- **SAML 2.0 (optionnel, derrière un flag)** : métadonnées de l'IdP (URL ou XML), entity id du SP, certificat, mapping des attributs, endpoint ACS et métadonnées du SP exposées.
- **Configuration** dans `config/admin` (déclarative), avec les secrets via `env()`. Réglages modifiables dans Settings > Single Sign-On : création automatique des comptes, rôle par défaut, rôles verrouillés SSO.
- **Flux** :
  1. bouton par provider sur l'écran de connexion ;
  2. redirection, callback, vérification du state, du nonce et de la signature de l'id_token (JWKS) ;
  3. association au compte admin par e-mail, ou création si l'auto-inscription est active ;
  4. ouverture d'une session admin standard (`AUTH-003`).
- **SSO lock** : un utilisateur dont un rôle est verrouillé ne peut ni se connecter par mot de passe, ni réinitialiser son mot de passe.
- **Domaines autorisés** : liste optionnelle de domaines d'e-mail acceptés par provider.
- **Journalisation** des connexions SSO dans les audit logs (`XTR-002`).

## Hors périmètre
- Le SSO pour les utilisateurs finaux (Content API) : c'est `AUTH-005`.
- SCIM, provisioning.

## Critères d'acceptation
- [ ] Connexion réussie avec Keycloak (en conteneur sur le homelab), Google et GitHub sur une instance de test.
- [ ] Première connexion avec l'auto-inscription active : un compte est créé avec le rôle par défaut. Sans auto-inscription : refus avec un message clair.
- [ ] Un id_token mal signé, expiré ou au nonce rejoué est refusé.
- [ ] Un utilisateur d'un rôle verrouillé reçoit 403 sur la connexion par mot de passe et sur la demande de reset.
- [ ] Un e-mail hors des domaines autorisés est refusé.
- [ ] SAML (flag activé) : connexion réussie avec Keycloak en IdP SAML.
- [ ] Écran de réglages conforme aux conventions UI (`ADM-002`).

## Notes techniques
- Bibliothèques : `coreos/go-oidc/v3`, `golang.org/x/oauth2`, `crewjam/saml` pour SAML.
- Le state et le nonce sont stockés dans un cookie signé à durée courte. Ne jamais faire confiance à un e-mail non vérifié (`email_verified` est requis s'il est présent).

## Référence Strapi
- Documentation publique Strapi : « Single Sign-On (SSO) », « Configuring SSO providers ».

## Dépendances
- `AUTH-003`, `AUTH-007`, `ADM-002`, `ADM-008`, `EXT-001`

## Fichiers
- `internal/auth/sso/{oidc,presets,saml,lock}.go`
- `admin/src/pages/settings/sso/`, `admin/src/pages/auth/sso-buttons.tsx`
