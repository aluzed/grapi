# API admin (`/admin/api/v1`)

L'API de l'admin de grapi (décision D1, option B : `docs/decisions/D1-admin.md`). C'est un contrat propre à grapi, versionné par le segment `v1`. Il n'imite pas les routes internes de l'admin Strapi. Il travaille sur les tables admin d'une base Strapi 5 (`admin_users`, `admin_roles`, `admin_permissions`, `strapi_sessions`) : un admin créé par Strapi se connecte sur grapi avec son mot de passe.

Le préfixe suit `admin.url` (`/admin` par défaut). Le reste de `/admin` sert la SPA (`admin/`), ou une page d'attente si le binaire a été compilé sans elle.

## Format
- Succès : `{"data": ...}`.
- Erreur : `{"data": null, "error": {"status", "name", "message", "details"}}`. Une erreur de validation porte `details.errors: [{path: [...], message}]`.
- Les routes authentifiées attendent `Authorization: Bearer <access token>`. Sans token valide : 401 `UnauthorizedError`.

## Sessions
- Une connexion ouvre une session dans `strapi_sessions` (origine `admin`) et renvoie un access token (30 minutes par défaut) dans le corps.
- Le refresh token est dans le cookie httpOnly `grapi_admin_refresh`, limité au chemin `/admin/api/v1/auth`.
  - Avec `rememberMe`, la session est de type `refresh` et le cookie est persistant.
  - Sans, la session est de type `session` et le cookie meurt avec le navigateur.
- `POST /auth/access-token` fait tourner le refresh token (nouveau cookie) et renvoie un nouvel access token. Rejouer un refresh token déjà utilisé révoque toutes les sessions de cet appareil, sauf dans la minute qui suit sa rotation et tant que le token suivant n'a pas lui-même tourné : la requête reçoit alors le même token suivant. Les onglets d'un navigateur partagent le cookie et peuvent renouveler en même temps.
- Durées : `admin.auth.sessions.{accessTokenLifespan, maxRefreshTokenLifespan, idleRefreshTokenLifespan, maxSessionLifespan, idleSessionLifespan}`, en secondes, avec les défauts de Strapi (30 min, 30 j, 14 j, 1 j, 2 h).
- Cookie : `admin.auth.cookie.{secure, sameSite, domain}`. Sans `secure` explicite, le cookie est `Secure` derrière HTTPS (ou `X-Forwarded-Proto: https` avec `server.proxy`).
- Secret : `admin.auth.secret`, sinon `ADMIN_JWT_SECRET`. S'il manque, grapi en génère un et l'ajoute au `.env`.

## Routes

| Méthode et chemin | Auth | Rôle |
|---|---|---|
| `GET /auth/init` | non | `{hasAdmin}` |
| `POST /auth/register-admin` | non | Crée le premier Super Admin (`firstname`, `lastname`, `email`, `password`), refusé dès qu'un admin existe. Renvoie `{token, user}`. |
| `POST /auth/login` | non | `email`, `password`, `rememberMe`, `deviceId` facultatif. Renvoie `{token, user}`. |
| `POST /auth/access-token` | cookie | `{token}` et un nouveau cookie |
| `POST /auth/logout` | cookie | Ferme les sessions de l'appareil du cookie |
| `POST /auth/forgot-password` | non | `email`. Répond toujours `{}` ; envoie le lien `<admin.absoluteUrl>/auth/reset-password?code=` si le compte existe et est actif. |
| `POST /auth/reset-password` | non | `resetPasswordToken`, `password`. Token valable `admin.forgotPassword.expiresIn` (1 h). Ferme les autres sessions et renvoie `{token, user}`. |
| `GET /auth/registration-info?registrationToken=` | non | `{email, firstname, lastname}` d'une invitation |
| `POST /auth/register` | non | `registrationToken`, `userInfo: {firstname, lastname, password}`. Active le compte et renvoie `{token, user}`. |
| `GET /users/me` | oui | L'admin connecté, avec ses rôles |
| `PUT /users/me` | oui | `firstname`, `lastname`, `username`, `email`, `preferedLanguage` (`en`, `fr`), `currentPassword` + `password`. Changer le mot de passe ferme les sessions des autres appareils. |
| `GET /users/me/permissions` | oui | `[{action, subject, properties, conditions}]`. Le Super Admin reçoit toutes les actions enregistrées. |
| `GET /users/me/sessions` | oui | Sessions actives, `isCurrent` pour celle de la requête |
| `DELETE /users/me/sessions/{id}` | oui | Révoque une session (et son appareil) |
| `DELETE /users/me/sessions` | oui | Révoque toutes les sessions des autres appareils |
| `GET /information` | oui | `{apiVersion, develop, languages, version, goVersion, database, environment, contentTypes, edition}` |
| `GET /homepage` | oui | Widgets de l'accueil sur les types lisibles : `counts` (documents par type), `lastEdited` et `lastPublished` (5 derniers) |

### Admins et rôles

Chaque route exige l'action indiquée (403 `ForbiddenError` sinon). Le Super Admin a toutes les actions.

| Méthode et chemin | Action | Rôle |
|---|---|---|
| `GET /users?page=&pageSize=&search=&sort=champ:asc` | `admin::users.read` | `{data, meta.pagination}`. `search` cherche dans prénom, nom, e-mail et username. |
| `GET /users/{id}` | `admin::users.read` | Un admin |
| `POST /users` | `admin::users.create` | Invite : `firstname`, `lastname`, `email`, `roles` (ids, au moins un). Le compte est inactif jusqu'à `auth/register`. Envoie le lien d'invitation et renvoie `{user, registrationToken, registrationUrl}` (201). |
| `PUT /users/{id}` | `admin::users.update` | `firstname`, `lastname`, `username`, `email`, `isActive`, `blocked`, `roles`, `password`. Désactiver, bloquer ou changer le mot de passe ferme ses sessions. |
| `DELETE /users/{id}` | `admin::users.delete` | Supprime l'admin et ses sessions |
| `POST /users/batch-delete` | `admin::users.delete` | `{ids}` |
| `GET /roles` | `admin::roles.read` | Rôles avec `usersCount` |
| `GET /roles/{id}` | `admin::roles.read` | Un rôle |
| `POST /roles` | `admin::roles.create` | `name` (unique sans tenir compte de la casse), `description`. Le `code` est généré. |
| `PUT /roles/{id}` | `admin::roles.update` | `name`, `description` |
| `DELETE /roles/{id}` | `admin::roles.delete` | Refusé pour le Super Admin et pour un rôle encore attribué |
| `GET /roles/{id}/permissions` | `admin::roles.read` | Les permissions du rôle |
| `PUT /roles/{id}/permissions` | `admin::roles.update` | `{permissions: [{action, subject, properties, conditions}]}` remplace tout. Chaque permission est validée contre le catalogue (action connue, sujet accepté, propriétés `fields` et `locales` seulement si l'action les prend, conditions connues). Refusé pour le Super Admin. |
| `GET /permissions` | `admin::roles.read` | Catalogue : `sections.{contentTypes, plugins, settings}` et `conditions` |

Il reste toujours au moins un Super Admin actif : le supprimer, le désactiver, le bloquer ou lui retirer ce rôle est refusé s'il est le dernier.

## Règles
- Mot de passe : 8 caractères au moins, une minuscule, une majuscule, un chiffre, 72 octets au plus (limite de bcrypt). Hash bcrypt de coût 10, comme Strapi.
- Un admin inactif ou bloqué ne peut ni se connecter ni utiliser un token déjà émis.
- Au premier démarrage sur une base sans rôle admin, grapi crée Super Admin, Editor et Author (codes `strapi-super-admin`, `strapi-editor`, `strapi-author`). Editor reçoit les permissions du Content Manager sur les content types `api::` et celles de la médiathèque ; Author les mêmes sans `publish`, limitées à ses propres entrées (`admin::is-creator`). Une base qui a déjà des rôles n'est jamais modifiée.

### Content Manager

Les droits viennent des actions `plugin::content-manager.explorer.{read,create,update,delete,publish}` sur le content type. Leurs conditions (par exemple `admin::is-creator` du rôle Author) filtrent les documents : un document hors filtre répond 404. Les champs non lisibles sont retirés des réponses, les champs non modifiables ignorés à l'écriture.

| Méthode et chemin | Rôle |
|---|---|
| `GET /content-manager/models` | Content types lisibles : `uid`, `kind`, `info`, `draftAndPublish`, `localized`, `attributes` lisibles, `mainField`, `listColumns`, `labels`, `settings` (taille de page, tri par défaut, recherche, filtres, actions groupées) et `permissions` de l'utilisateur. Lit la configuration de vue de Strapi (`plugin_content_manager_configuration_content_types::<uid>`) quand elle existe. |
| `GET /content-manager/components` | Composants : `uid`, `category`, `info`, `attributes`, `fields` (ordre du schéma) |
| `GET /content-manager/locales` | Locales i18n, avec `isDefault` |
| `GET /content-manager/collection-types/{uid}?page=&pageSize=&sort=&_q=&filters=&locale=&status=` | Liste. `filters` est un objet JSON au format des filtres REST, `status` vaut `draft` (défaut) ou `published`. Chaque document porte `status` : `draft`, `published` ou `modified`. |
| `GET /content-manager/relations/{uid}/{field}?_q=&page=&pageSize=&locale=` | Documents que la relation `field` peut viser (hors morph) : `{id, documentId, label}`, `label` étant le `mainField` de la cible, avec `meta.pagination` |
| `GET /content-manager/collection-types/{uid}/{documentId}?locale=&status=` | Un document chargé en profondeur : relations, médias, composants et zones dynamiques à tous les niveaux |
| `POST /content-manager/collection-types/{uid}` | `{data, locale}`. Renseigne `createdBy` et `updatedBy`. Les relations s'écrivent comme dans l'API REST (`{set: [{documentId}]}` remplace la liste dans cet ordre, `connect` et `disconnect` sont acceptés). |
| `PUT /content-manager/collection-types/{uid}/{documentId}` | `{data, locale}`. Renseigne `updatedBy`. |
| `DELETE /content-manager/collection-types/{uid}/{documentId}?locale=` | Supprime le document (dans la locale) |
| `POST /content-manager/collection-types/{uid}/actions/bulk-delete` | `{documentIds, locale}` |
| `POST /content-manager/collection-types/{uid}/actions/bulk-publish` | `{documentIds, locale}` (action `publish`) |
| `POST /content-manager/collection-types/{uid}/actions/bulk-unpublish` | `{documentIds, locale}` (action `publish`) |

| `GET /content-manager/single-types/{uid}?locale=&status=` | Le document du single type, `null` s'il n'existe pas encore |
| `PUT /content-manager/single-types/{uid}` | `{data, locale}` : crée le document (action `create`) ou le met à jour (`update`) |
| `DELETE /content-manager/single-types/{uid}?locale=` | Supprime le document |
| `POST /content-manager/single-types/{uid}/actions/publish` et `unpublish` | `{locale}` |

| `POST /content-manager/uid/generate` | `{contentTypeUID, field, data, locale}` : slug du champ cible (`targetField`) de `data`, ou du nom du modèle, rendu unique par un suffixe `-1`, `-2`… (options slugify de l'attribut respectées) |
| `POST /content-manager/uid/check-availability` | `{contentTypeUID, field, value, locale}` : `{isAvailable, suggestion}` |

`GET /content-manager/relations/{uid}/{field}` accepte `?component=<uid>` pour une relation définie dans un composant.

Une erreur de validation du document porte `details.errors: [{path, message}]` avec le chemin complet (`links.0.label`).

Les actions groupées traitent chaque document séparément et renvoient `{count, documentIds, errors: [{documentId, message}]}` : un document invalide ou hors des droits n'empêche pas les autres.

### Médiathèque

Routes présentes quand le plugin upload est actif. Les droits sont `plugin::upload.read` (lecture), `plugin::upload.assets.create` (upload, dossiers) et `plugin::upload.assets.update` (édition, déplacement, suppression, remplacement). La condition `is-creator` de `assets.update` limite aux fichiers de l'admin. Un fichier uploadé depuis l'admin porte `createdBy` et `updatedBy`.

| Méthode et chemin | Rôle |
|---|---|
| `GET /upload/files?folder=&_q=&type=&sort=&page=&pageSize=&all=` | Fichiers d'un dossier (`folder` : id, racine par défaut ; `all=true` : tous). `type` vaut `image`, `video`, `audio` ou `file`. Tri par `name`, `createdAt`, `updatedAt`, `size` (défaut `createdAt:desc`). |
| `GET /upload/files/{id}` | Un fichier, avec son dossier |
| `PUT /upload/files/{id}` | `name`, `alternativeText`, `caption`, `folder` (id) ou `rootFolder: true` |
| `DELETE /upload/files/{id}` | Supprime le fichier et ses formats chez le provider |
| `POST /upload?folder=` | Multipart `files` (et `fileInfo`), même traitement que l'API de contenu : optimisation, formats, provider. Avec `?id=`, met à jour les infos ou remplace le fichier en gardant son id. |
| `GET /upload/folders?parent=` | Sous-dossiers (racine par défaut), avec `filesCount` et `childrenCount` |
| `POST /upload/folders` | `name`, `parent`. Le `pathId` suit le plus grand existant, le `path` est celui du parent plus ce numéro, comme Strapi. Nom unique dans le dossier parent. |

### Locales (i18n)

Présentes quand le plugin i18n est actif. La liste des locales est `GET /content-manager/locales` (tout admin connecté).

| Méthode et chemin | Action | Rôle |
|---|---|---|
| `GET /i18n/iso-locales` | `plugin::i18n.locale.read` | Liste ISO des locales proposables |
| `POST /i18n/locales` | `plugin::i18n.locale.create` | `code` (ISO), `name` (nom ISO par défaut), `isDefault` |
| `PUT /i18n/locales/{id}` | `plugin::i18n.locale.update` | `name`, `isDefault` |
| `DELETE /i18n/locales/{id}` | `plugin::i18n.locale.delete` | Supprime la locale et, comme Strapi, toutes les entrées écrites dans cette locale. Refusé pour la locale par défaut. |

`PUT /content-manager/collection-types/{uid}/{documentId}` avec une `locale` où le document n'existe pas encore crée cette version (action `create`). Les champs non localisés sont partagés entre les versions.

### API tokens

Tokens de l'API de contenu (`kind` `content-api`). Actions `admin::api-tokens.{read,create,update,regenerate,delete}`. La clé suit le schéma de Strapi (`internal/auth/apitoken`) : 128 octets aléatoires en hexadécimal, stockés en HMAC-SHA512 avec `admin.apiToken.salt`, et chiffrés avec `admin.secrets.encryptionKey` quand il est défini (le token peut alors être réaffiché).

| Méthode et chemin | Rôle |
|---|---|
| `GET /api-tokens` | Tokens, sans leur clé |
| `GET /api-tokens/actions` | Actions qu'un token personnalisé peut recevoir (`api::article.article.find`…) |
| `GET /api-tokens/{id}` | Un token avec ses `permissions`, et sa clé si elle est stockée chiffrée |
| `POST /api-tokens` | `name` (unique), `description`, `type` (`read-only`, `full-access`, `custom`), `lifespan` (7, 30, 90 jours ou 0 pour illimité), `permissions` (type `custom`). Renvoie la clé une seule fois (`accessKey`). |
| `PUT /api-tokens/{id}` | `name`, `description`, `type`, `permissions` |
| `POST /api-tokens/{id}/regenerate` | Nouvelle clé (`accessKey`), l'ancienne cesse de fonctionner |
| `DELETE /api-tokens/{id}` | Révoque le token |
