# XTR-001 : SSO (OIDC, SAML)

> Clean-room (`SETUP-003`, décision D6) : ce ticket se spécifie uniquement à partir de la documentation publique de Strapi et du comportement observable. Aucun fichier source Strapi sous licence Enterprise ne doit être ouvert, y compris pour en lire le nom.

## Contexte
Strapi permet aux administrateurs de se connecter à l'admin via un fournisseur d'identité externe. La documentation publique décrit : des boutons de fournisseurs sur l'écran de connexion, une callback `/admin/connect/<uid>`, l'inscription automatique avec un rôle par défaut, et un verrouillage de l'authentification locale par rôle. Chez Strapi, les fournisseurs se déclarent en code (stratégies Passport). grapi les déclare en configuration YAML. Aucune limite de licence : tout est inclus et MIT.

## Objectif
Connexion admin via OIDC générique et des presets Google, Microsoft et GitHub, SAML 2.0 en option, avec inscription automatique et verrouillage du mot de passe par rôle.

## Périmètre
- **Configuration YAML** (`config/admin.yaml`, `EXT-001`), secrets via `${env:...}` :
  ```yaml
  auth:
    sso:
      enabled: true
      providers:
        - uid: google            # [a-z0-9-], unique, utilisé dans l'URL de callback
          preset: google         # google | microsoft | github | oidc | saml
          displayName: Google
          icon: https://...      # optionnel, domaine à autoriser dans la CSP (API-008)
          clientId: ${env:SSO_GOOGLE_CLIENT_ID}
          clientSecret: ${env:SSO_GOOGLE_CLIENT_SECRET}
          scopes: [openid, email, profile]
          allowedDomains: [example.com]   # optionnel, filtre sur le domaine de l'e-mail
  ```
- **Presets** : `google` (issuer Google), `microsoft` (`tenant` requis : id, `common` ou `organizations`), `github` (OAuth2 non OIDC : e-mail principal vérifié lu via l'API utilisateur GitHub), `oidc` générique (`issuer` avec découverte `.well-known/openid-configuration`).
- **SAML 2.0 (optionnel, build tag `saml`)** : `idpMetadataUrl` ou `idpMetadataXml`, `entityId`, certificat et clé SP, mapping d'attributs (`email`, `firstname`, `lastname`). Métadonnées SP servies sur `/admin/connect/<uid>/metadata`.
- **Flux** : `GET /admin/connect/<uid>` redirige vers le fournisseur (state, nonce, PKCE S256) ; la callback `/admin/connect/<uid>` (même chemin, ACS en POST pour SAML) valide la réponse, ouvre une session (`AUTH-003`, refresh en cookie) puis redirige vers l'admin. Erreur : redirection vers l'écran de connexion avec un code d'erreur lisible.
- **Rattachement** : identité `(provider uid, subject)` stockée et liée à l'admin. Première connexion : rattachement par e-mail vérifié (insensible à la casse). Un e-mail non vérifié est refusé.
- **Réglages modifiables dans l'admin** (core store `DB-010`, Settings > Single Sign-On) :
  - inscription automatique (désactivée par défaut) : un admin inconnu est créé avec prénom et nom issus des claims ;
  - rôle par défaut des comptes créés ;
  - rôles verrouillés : leurs utilisateurs ne peuvent ni se connecter par mot de passe, ni réinitialiser, ni changer leur mot de passe.
- **Écran de connexion** (`ADM-002`) : un bouton par fournisseur (icône + `displayName` + `title`), sous le formulaire local.
- **Garde-fous** : un admin désactivé ou bloqué est refusé même via SSO ; avertissement avant d'enregistrer un verrou contenant Super Admin ; commande de secours `grapi admin:sso-unlock` (`CLI-003`) qui vide la liste des rôles verrouillés.
- **Permissions** : `admin::sso.read` et `admin::sso.update`.
- Événements audit `admin.auth.success`, `admin.auth.error`, `admin.auth.autoRegistration` (`XTR-002`).

## Hors périmètre
- SSO pour les utilisateurs finaux du Content API (c'est `AUTH-005`).
- LDAP, Kerberos, SCIM (provisionnement), synchronisation des rôles depuis les groupes du fournisseur : à proposer séparément.
- Single logout (SLO) SAML et déconnexion OIDC côté fournisseur.

## Critères d'acceptation
- [ ] Avec un fournisseur OIDC de test (Dex ou Keycloak dans le banc), un admin existant se connecte par SSO et obtient une session refreshable.
- [ ] Callback avec un `state` altéré ou rejoué : refus, aucune session créée.
- [ ] Inscription automatique désactivée : un e-mail inconnu est refusé avec un message explicite ; activée : l'admin est créé avec le rôle par défaut et `isActive: true`.
- [ ] `allowedDomains: [example.com]` refuse `bob@other.org`.
- [ ] Un utilisateur d'un rôle verrouillé reçoit une erreur explicite sur `POST /admin/login` ; `forgot-password` n'envoie aucun e-mail mais renvoie la même réponse qu'un e-mail inconnu.
- [ ] Le preset GitHub ignore les e-mails non vérifiés et utilise l'e-mail principal vérifié.
- [ ] Changer l'e-mail chez le fournisseur ne crée pas de doublon : le rattachement se fait par `subject`.
- [ ] `grapi admin:sso-unlock` permet de nouveau la connexion par mot de passe.
- [ ] Binaire sans build tag `saml` : un fournisseur `preset: saml` fait échouer le démarrage avec une erreur claire.
- [ ] Écran Settings > SSO conforme aux conventions UI de `ADM-002` (375 px sans scroll horizontal, boutons pleine largeur).

## Notes techniques
- `coreos/go-oidc/v3` + `golang.org/x/oauth2` ; SAML via `crewjam/saml`, isolé derrière le build tag pour ne pas alourdir le binaire par défaut.
- Interface `sso.Provider { AuthURL(state) ; Exchange(ctx, req) (Identity, error) }`. Un preset n'est qu'une fabrique de `Provider` : un projet en mode framework (`EXT-002`) peut enregistrer le sien.
- State, nonce et verifier PKCE dans un cookie chiffré à durée courte (10 min), pas en mémoire, pour tenir derrière plusieurs instances.
- Table `admin_sso_identities` (`provider`, `subject`, `admin_user_id`, `email`, `last_login_at`), unique sur `(provider, subject)`.
- Écran admin : `admin/src/pages/settings/sso/` (formulaire React-Hook-Form + Zod, sélecteur multiple de rôles).

## Référence
- Single Sign-On (SSO) : https://docs.strapi.io/cms/features/sso
- SSO configuration : https://docs.strapi.io/cms/configurations/guides/configure-sso
- Spécification clean-room : aucune source Strapi sous licence Enterprise consultée.

## Dépendances
- `AUTH-003`, `AUTH-007`, `EXT-001`, `DB-010`, `API-008`, `ADM-002`, `ADM-008`, `CLI-003`, `SETUP-003`

## Fichiers
- `internal/extra/sso/` (`provider.go`, `oidc.go`, `github.go`, `saml.go`, `handlers.go`, `settings.go`)
- `admin/src/pages/settings/sso/`, `admin/src/pages/auth/` (boutons fournisseurs)
