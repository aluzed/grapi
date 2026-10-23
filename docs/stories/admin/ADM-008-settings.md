---
status: in-progress
---

# ADM-008 : Settings (webhooks, tokens, utilisateurs, rôles et RBAC, locales, users-permissions, e-mail, projet)

> Option retenue : B (admin maison), voir `docs/decisions/D1-admin.md`. Conventions UI : voir `ADM-002`.

## Contexte
La section Settings de Strapi regroupe la configuration modifiable à chaud : informations de l'application, réglages du projet (logos), webhooks, API tokens, transfer tokens, utilisateurs et rôles admin (avec l'éditeur de permissions RBAC), locales i18n, rôles et réglages users-permissions (providers, templates d'e-mail, réglages avancés), configuration e-mail (envoi de test) et Media Library.

## Objectif
Exposer tous ces réglages dans l'admin, chaque section étant protégée par sa permission.

## Périmètre
- **Vue d'ensemble** : version de grapi, version de Go, édition (toujours « Community, tout inclus »), base utilisée, liste des plugins installés.
- **Projet** : logo du menu et logo de l'écran de connexion (upload, réinitialisation).
- **Webhooks** :
  - liste, création et édition (nom, URL, en-têtes, événements par catégorie et par content type), activation ;
  - déclenchement manuel avec affichage de la réponse ;
  - suppression unitaire et groupée.
- **API tokens** :
  - liste, création (nom, description, durée : illimitée, 7, 30 ou 90 jours ; type : lecture seule, accès complet ou personnalisé) ;
  - pour le type personnalisé, matrice des permissions par content type et par action ;
  - token affiché une seule fois puis consultable si le chiffrement est configuré ;
  - régénération, suppression.
- **Admin tokens** : mêmes écrans, liés aux permissions du propriétaire (`AUTH-008`).
- **Transfer tokens** : liste, création (push, pull, ou les deux), régénération, suppression.
- **Utilisateurs admin** :
  - liste avec filtres, invitation (e-mail + rôles, lien d'enregistrement copiable) ;
  - édition, activation et désactivation, suppression ;
  - réinitialisation de mot de passe.
- **Rôles admin** :
  - liste, création, duplication, suppression (interdite si des utilisateurs portent le rôle) ;
  - éditeur RBAC :
    - par content type : actions create, read, update, delete, publish ;
    - par champ ;
    - par locale ;
    - conditions (`is-creator`, `has-same-role-as-creator` et conditions des plugins) ;
    - permissions des plugins et des réglages ;
  - Super Admin non modifiable.
- **Locales** : liste, ajout depuis la liste ISO, locale par défaut, suppression (avec avertissement sur les documents de cette locale).
- **Users-permissions** :
  - rôles Public, Authenticated et rôles personnalisés, avec la matrice des actions par controller et la liste des routes correspondantes ;
  - providers OAuth (activation, clé, secret, URL de callback, scopes) ;
  - templates d'e-mail (confirmation, reset) avec variables documentées ;
  - réglages avancés (e-mail unique, inscription ouverte, rôle par défaut, confirmation e-mail et redirection, URL de reset).
- **E-mail** : provider actif et expéditeur par défaut en lecture seule, envoi d'un e-mail de test.
- **Media Library** : réglages de `ADM-007`.

## Avancement
- Écran Paramètres avec sous-menu filtré par permissions (select sur mobile).
- Vue d'ensemble : version de grapi, édition, version de Go, base, environnement, nombre de content types.
- Administrateurs : liste avec recherche et statut (actif, invitation en attente, bloqué), invitation (e-mail, rôles, lien d'inscription copiable), édition (identité, rôles, activation), suppression avec confirmation.
- Rôles : liste avec nombre d'utilisateurs, création, suppression (refusée si le rôle est attribué), éditeur : nom, description, matrice create, read, update, delete, publish par content type avec « Ses entrées seulement » (`is-creator`), actions des plugins et des réglages groupées par catégorie. Super Admin en lecture seule.
- Locales : liste, ajout depuis la liste ISO avec nom affiché et locale par défaut, édition, suppression confirmée (avec les entrées de la locale), défaut non supprimable.
- API tokens : liste (type, expiration, dernière utilisation), création avec durée et type, matrice des actions par content type pour le type personnalisé, clé affichée une fois avec copie, édition, régénération et suppression confirmées. La logique de clé est partagée avec la CLI (`internal/auth/apitoken`). Testé (`TestAdminAPITokens` utilise le token sur l'API de contenu) et vérifié dans Chrome.
- Webhooks : liste avec statut, création et édition (nom, URL, en-têtes, événements par catégorie, activation), envoi de test avec le statut de la réponse, suppression confirmée. Testé (`TestAdminWebhooks` : test, livraison d'un `entry.create` avec l'en-tête, désactivation) et vérifié dans Chrome.
- Users-permissions : rôles des utilisateurs finaux (liste, création, édition avec la matrice des actions par controller et par plugin, suppression qui renvoie les utilisateurs vers Public), réglages avancés, modèles d'e-mail, providers (activation, client ID, secret, URL de redirection). Testé (`TestAdminUsersPermissions` vérifie l'effet sur l'API de contenu) et vérifié dans Chrome.
- E-mail : fournisseur et expéditeurs par défaut, envoi d'un e-mail de test (`TestAdminEmailTest`).
- Vérifié dans Chrome sur le binaire.
- Critères prouvés : webhook limité aux événements cochés (`TestAdminWebhooks`), token `find` sans écriture (`TestAdminAPITokens`), rôle modifié appliqué aux sessions ouvertes (`TestAdminRoleChangeAppliesAtOnce`), Super Admin protégé et dernier actif non désactivable (`TestAdminUsersAndRoles`), e-mail de test reçu dans Mailpit (`make e2e-invite`), et toutes les suppressions confirmées, y compris le retrait d'un élément de composant, de bloc, de relation ou de média dans un formulaire.
- Éditeur de rôle : bouton « Restreindre » par content type, champs autorisés pour créer, lire et modifier, et locales autorisées pour un type localisé (tout coché : pas de restriction, `fields`/`locales` absents). Sections masquées sans permission vérifiées dans Chrome avec un Author (seule « Vue d'ensemble », URL directe sur la page « Accès refusé », API en 403). Vérifié dans Chrome sur la fixture `i18n-dp` : `update` limité à `title` et `body` et à la locale `en`, enregistré tel quel par le serveur.
- Duplication de rôle (action « Dupliquer » : nouveau rôle « … (copie) » avec les mêmes permissions, restrictions comprises, puis ouverture de son éditeur), vérifiée dans Chrome.
- Conditions « créées par l'utilisateur » (`admin::is-creator`) et « créées par son rôle » (`admin::has-same-role-as-creator`) dans l'éditeur, communes aux actions d'un type et combinées en OU comme Strapi ; vérifié dans Chrome.
- Suppression groupée des administrateurs (sélection, confirmation, `batch-delete`), vérifiée dans Chrome.
- Rôles users-permissions : la ou les routes ouvertes par chaque action s'affichent sous sa case (`GET /users-permissions/routes`, `TestAdminUPRoutes`).
- Reste : événements par content type des webhooks, transfer tokens, section projet (les sections e-mail et médiathèque sont faites).

## Hors périmètre
- Les réglages des fonctions `XTR` (SSO, audit logs, review workflows, releases) : ils sont dans leurs tickets.

## Critères d'acceptation
- [x] Chaque section est masquée sans sa permission, et l'URL directe renvoie 403.
- [x] Un webhook créé depuis l'admin reçoit les événements cochés, et uniquement ceux-là (vérifié avec un récepteur de test).
- [x] Un API token personnalisé limité à `find` sur `api::post.post` peut lister les posts et reçoit 403 sur la création.
- [x] Modifier un rôle admin met à jour les permissions des utilisateurs connectés sans reconnexion, dès la requête suivante.
- [x] L'éditeur RBAC permet de restreindre `update` à deux champs et à une locale, et le serveur l'applique.
- [x] Le Super Admin ne peut ni être supprimé ni perdre ses permissions. Le dernier Super Admin actif ne peut pas être désactivé.
- [x] L'e-mail de test arrive (Mailpit en test).
- [x] Toutes les suppressions passent par la modale de confirmation.

## Notes techniques
- La matrice RBAC est générée à partir du catalogue d'actions (`AUTH-001`) : sections, sujets, propriétés (fields, locales) et conditions disponibles. Aucune liste n'est codée en dur côté SPA.
- Les secrets (clé et secret OAuth) ne repartent jamais en clair vers la SPA après enregistrement : on affiche un champ « défini » et on peut le remplacer.
- **Si l'option A est retenue** : reproduire `/admin/webhooks`, `/admin/api-tokens`, `/admin/admin-tokens`, `/admin/transfer/tokens`, `/admin/users`, `/admin/roles`, `/admin/permissions`, `/admin/project-settings`, `/admin/information`, `/i18n/locales`, `/users-permissions/*` et `/email/*`. La forme de `/admin/permissions` (sections, subjects, properties) est le point le plus coûteux.

## Référence Strapi
- `core/admin/server/src/routes/`, `core/admin/server/src/config/admin-actions.ts`, `core/admin/server/src/config/admin-conditions.ts`
- `core/admin/admin/src/pages/Settings/`
- `plugins/users-permissions/admin/src/`, `plugins/users-permissions/server/src/controllers/`
- `plugins/i18n/admin/src/`, `core/email/admin/src/`

## Dépendances
- `ADM-002`, `AUTH-001`, `AUTH-004`, `AUTH-005`, `AUTH-007`, `AUTH-008`, `AUTH-009`, `EXT-006`, `PLG-001`, `PLG-002`

## Fichiers
- `admin/src/pages/settings/{overview,project,webhooks,api-tokens,admin-tokens,transfer-tokens,users,roles,locales,users-permissions,email,media}/`
- `admin/src/features/rbac-editor/`
