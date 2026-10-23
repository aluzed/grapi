# API admin (`/admin/api/v1`)

L'API de l'admin de grapi (décision D1, option B : `docs/decisions/D1-admin.md`). C'est un contrat propre à grapi, versionné par le segment `v1`. Il n'imite pas les routes internes de l'admin Strapi. Il travaille sur les tables admin d'une base Strapi 5 (`admin_users`, `admin_roles`, `admin_permissions`, `strapi_sessions`) : un admin créé par Strapi se connecte sur grapi avec son mot de passe.

Le préfixe suit `admin.url` (`/admin` par défaut). Le reste de `/admin` sert la SPA (`admin/`), ou une page d'attente si le binaire a été compilé sans elle.

## Tests

`admin_test.go` couvre l'API de bout en bout, dont un test de fumée sur tous les fixtures du banc (`TestAdminSmokeFixtures`). Avec le tag `integration` et `GRAPI_TEST_PG_DSN` (`make ci`, Postgres du homelab), la même suite tourne aussi sur Postgres.

`make e2e-invite` vérifie l'invitation d'un admin avec un vrai serveur SMTP : Mailpit tourne sur le homelab (tunnel SSH), grapi ici sur une copie de la fixture `basic` ; l'e-mail reçu fournit le lien d'inscription, l'invité s'inscrit puis se connecte ; puis mot de passe oublié par e-mail, connexion avec le nouveau mot de passe, déconnexion ; l'e-mail de test des réglages doit aussi arriver dans Mailpit.

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
- Cookie : `admin.auth.cookie.{secure, sameSite, domain}`. Sans `secure` explicite, le cookie est `Secure` derrière HTTPS (ou `X-Forwarded-Proto: https` avec `server.proxy.koa`).
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
| `POST /permissions/check` | oui | `{permissions: [{action, subject, field}]}` : `data` est un booléen par entrée, pour l'admin connecté |
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

### Releases

Tables propres à grapi (`grapi_releases`, `grapi_release_actions`), créées au démarrage. Strapi les ignore.

| Méthode et chemin | Action | Rôle |
|---|---|---|
| `GET /releases?status=done` | `admin::releases.read` | Releases en attente (par défaut) ou publiées, avec `count` |
| `POST /releases` | `admin::releases.create` | `name`, `scheduledAt` (RFC 3339, stocké en UTC), `timezone` (IANA, `UTC` par défaut, gardé pour l'affichage) |
| `GET /releases/{id}` | `admin::releases.read` | La release et ses `actions`, avec l'état recalculé de chaque entrée : `ready`, `not-ready` (+ `errors`), `already-published`, `already-unpublished` |
| `PUT /releases/{id}` | `admin::releases.update` | Mêmes champs. Une nouvelle date redonne sa chance à une release `failed`. 409 si publiée. |
| `DELETE /releases/{id}` | `admin::releases.delete` | Les entrées restent telles quelles |
| `POST /releases/{id}/actions` | `admin::releases.actions.create` | `type` (`publish`, `unpublish`), `uid`, `documentId`, `locale`. 400 si le type n'a pas Draft & Publish ou si le document y est déjà ; exige aussi `publish` sur le type et la locale. |
| `DELETE /releases/{id}/actions/{actionId}` | `admin::releases.actions.delete` | |
| `POST /releases/{id}/publish` | `admin::releases.publish` | Exige `publish` sur le type et la locale de chaque entrée. Tout ou rien dans une transaction. 400 avec la liste des entrées fautives si la release est bloquée. |
| `GET /releases/settings`, `PUT /releases/settings` | `admin::releases.settings.read`, `.update` | `{defaultTimezone}` (IANA ou `null`), le fuseau d'une release créée sans `timezone` |
| `GET /releases/document/{uid}/{documentId}` | `admin::releases.read` | Releases en attente qui contiennent le document |

Statuts : `empty`, `ready`, `blocked`, `done`, et `failed` (publication planifiée refusée, rien n'a changé, message dans `error`). L'état `not-ready` vient d'une publication d'essai dans une transaction annulée : les événements partant au commit, elle ne déclenche aucun webhook.

Le planificateur tourne dans le serveur : au démarrage (une échéance passée pendant un arrêt s'exécute tout de suite), toutes les 30 s, et dès qu'une release reçoit une date passée. Avec plusieurs instances, la mise à jour `released_at is null` garantit une seule exécution. Webhook `releases.publish` : `{event, createdAt, isPublished, release: {id, name, releasedAt, scheduledAt, timezone, createdAt, actions: {count}}}`, `isPublished: false` sur un échec. Audit `release.*` et `release-action.*` ; les publications planifiées ont l'origine `scheduler`. Un document supprimé (admin, API de contenu ou Document Service) ou une locale supprimée sort des releases en attente.

## Règles
- Mot de passe : 8 caractères au moins, une minuscule, une majuscule, un chiffre, 72 octets au plus (limite de bcrypt). Hash bcrypt de coût 10, comme Strapi.
- Un admin inactif ou bloqué ne peut ni se connecter ni utiliser un token déjà émis.
- Au premier démarrage sur une base sans rôle admin, grapi crée Super Admin, Editor et Author (codes `strapi-super-admin`, `strapi-editor`, `strapi-author`). Editor reçoit les permissions du Content Manager sur les content types `api::` et celles de la médiathèque ; Author les mêmes sans `publish`, limitées à ses propres entrées (`admin::is-creator`). Une base qui a déjà des rôles n'est jamais modifiée.

### Content Manager

Les droits viennent des actions `plugin::content-manager.explorer.{read,create,update,delete,publish}` sur le content type. Leurs conditions (par exemple `admin::is-creator` du rôle Author) filtrent les documents : un document hors filtre répond 404. Les champs non lisibles sont retirés des réponses, les champs non modifiables ignorés à l'écriture.

| Méthode et chemin | Rôle |
|---|---|
| `GET /content-manager/models` | Content types lisibles : `uid`, `kind`, `info`, `draftAndPublish`, `localized`, `attributes` lisibles, `mainField`, `listColumns`, `labels`, `settings` (taille de page, tri par défaut, recherche, filtres, actions groupées), `editLayout` et `editMeta` (lignes et libellés du formulaire), `canConfigure` et `permissions` de l'utilisateur, d'après la configuration de vue effective. |
| `GET /content-manager/components` | Composants : `uid`, `category`, `info`, `attributes`, `fields` (ordre du schéma) |
| `GET /content-manager/locales` | Locales i18n, avec `isDefault` |
| `GET /content-manager/configuration/{uid}` | Configuration de vue effective : celle du core store (`plugin_content_manager_configuration_content_types::<uid>`, format Strapi) synchronisée avec le schéma comme le fait Strapi (`syncConfiguration` : seuls les attributs absents des métadonnées stockées sont ajoutés, en fin de liste et de formulaire ; un champ retiré de la mise en page n'y revient pas), ou celle par défaut de Strapi, identique à celle qu'il écrit (4 colonnes dont `id`, lignes de 12 colonnes, métadonnées de `id` et `documentId`, `relationOpenMode`, champs `config.attributes.<nom>.hidden` exclus) |
| `PUT /content-manager/configuration/{uid}` | `{settings, metadatas, layouts}` (action `configure-view`). Validée : colonnes listables, champs éditables placés une fois, tailles 4, 6, 8 ou 12 (12 imposé pour JSON, richtext, blocks, composant, zone dynamique), lignes de 12 au plus. |
| `GET /content-manager/collection-types/{uid}?page=&pageSize=&sort=&_q=&filters=&locale=&status=` | Liste. `filters` est un objet JSON au format des filtres REST, `status` vaut `draft` (défaut) ou `published`. Chaque document porte `status` : `draft`, `published` ou `modified`. |
| `GET /content-manager/relations/{uid}/{field}?_q=&page=&pageSize=&locale=` | Documents que la relation `field` peut viser (hors morph) : `{id, documentId, label}`, `label` étant le `mainField` de la cible, avec `meta.pagination` |
| `GET /content-manager/collection-types/{uid}/{documentId}?locale=&status=` | Un document chargé en profondeur : relations, médias, composants et zones dynamiques à tous les niveaux |
| `POST /content-manager/collection-types/{uid}` | `{data, locale}`. Renseigne `createdBy` et `updatedBy`. Les relations s'écrivent comme dans l'API REST (`{set: [{documentId}]}` remplace la liste dans cet ordre, `connect` et `disconnect` sont acceptés). |
| `PUT /content-manager/collection-types/{uid}/{documentId}` | `{data, locale}` (ou `?locale=`). Renseigne `updatedBy`. |
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

Les actions groupées traitent chaque document séparément et renvoient `{count, documentIds, errors: [{documentId, message, details?}]}` : un document invalide ou hors des droits n'empêche pas les autres. Pour un document invalide, `details.errors` donne les champs en cause, au même format qu'une erreur de validation.

### Médiathèque

Routes présentes quand le plugin upload est actif. Les droits sont `plugin::upload.read` (lecture), `plugin::upload.assets.create` (upload, dossiers) et `plugin::upload.assets.update` (édition, déplacement, suppression, remplacement). La condition `is-creator` de `assets.update` limite aux fichiers de l'admin. Un fichier uploadé depuis l'admin porte `createdBy` et `updatedBy`.

| Méthode et chemin | Rôle |
|---|---|
| `GET /upload/files?folder=&_q=&type=&sort=&from=&to=&page=&pageSize=&all=` | Fichiers d'un dossier (`folder` : id, racine par défaut ; `all=true` : tous). `type` vaut `image`, `video`, `audio` ou `file`. Tri par `name`, `createdAt`, `updatedAt`, `size`, direction `asc` ou `desc` sans tenir compte de la casse (défaut `createdAt:desc`). `from` et `to` (RFC 3339) bornent la date d'envoi. |
| `GET /upload/files/{id}` | Un fichier, avec son dossier |
| `PUT /upload/files/{id}` | `name`, `alternativeText`, `caption`, `folder` (id) ou `rootFolder: true` |
| `DELETE /upload/files/{id}` | Supprime le fichier et ses formats chez le provider |
| `POST /upload?folder=` | Multipart `files` (et `fileInfo`), même traitement que l'API de contenu : optimisation, formats, provider. Avec `?id=`, met à jour les infos ou remplace le fichier en gardant son id. |
| `GET /upload/folders?parent=&all=` | Sous-dossiers (racine par défaut), avec `filesCount` et `childrenCount` ; `all=true` : tous les dossiers, triés par chemin |
| `PUT /upload/folders/{id}` | Renomme (`name`, unique dans le dossier parent) |
| `DELETE /upload/folders/{id}` | Supprime le dossier, ses sous-dossiers et leurs fichiers, comme Strapi : `{files, folders}` |
| `POST /upload/actions/bulk-delete` | `{fileIds, folderIds}` : fichiers (dans les droits) et dossiers avec leur contenu |
| `POST /upload/actions/bulk-move` | `{fileIds, folderIds, destination}` (`null` : racine). Tout ou rien : un dossier déplacé réécrit le `path` de ses sous-dossiers et le `folderPath` de leurs fichiers ; refusé vers lui-même, un descendant, ou à côté d'un dossier du même nom. Renvoie `{files, folders}`. |
| `GET /upload/files/{id}/related` | Entrées lisibles qui utilisent le fichier : `[{uid, contentType, kind, documentId, locale, field, title, published}]` (un usage dans un composant, même imbriqué, est rattaché à son entrée) |
| `GET /upload/folder-structure` | Arbre des dossiers `[{id, name, children}]` |
| `POST /upload/actions/upload-from-urls` | `{urls, folder}` : le serveur télécharge chaque URL (http, https ; 200 Mo et 60 s au plus ; adresses privées refusées sauf `plugins.upload.config.uploadFromUrls.allowPrivateNetworks`) puis les envoie comme un upload. 400 avec l'URL fautive dans `details`. |
| `GET\|PUT /upload/settings` | `{sizeOptimization, responsiveDimensions, autoOrientation}` (`plugin::upload.settings.read`), appliqués aux uploads suivants |
| `GET\|PUT /upload/configuration` | `{pageSize, sort}` de la médiathèque (`configure-view-settings` pour écrire) |
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

### Webhooks

Table `strapi_webhooks`, partagée avec le runner qui envoie les événements. Actions `admin::webhooks.{read,create,update,delete}`.

| Méthode et chemin | Rôle |
|---|---|
| `GET /webhooks` | `[{id, name, url, headers, events, isEnabled}]` |
| `GET /webhooks/events` | Événements disponibles (`entry.*`, `media.*`) |
| `GET /webhooks/{id}` | Un webhook |
| `POST /webhooks` | `name`, `url` (http ou https ; en production, refusée si elle pointe vers une adresse locale ou privée, comme Strapi), `headers`, `events`, `isEnabled` (vrai par défaut) |
| `PUT /webhooks/{id}` | Mêmes champs, remplace le webhook |
| `DELETE /webhooks/{id}` | Supprime le webhook |
| `POST /webhooks/{id}/trigger` | Envoie l'événement `trigger-test` et renvoie `{statusCode, message}` (`message` en cas d'échec réseau) |

### Users-permissions

Les rôles et réglages des utilisateurs finaux (API de contenu). Actions `plugin::users-permissions.{roles,advanced-settings,email-templates,providers}.*`.

| Méthode et chemin | Rôle |
|---|---|
| `GET /users-permissions/roles` | Rôles avec `usersCount` |
| `GET /users-permissions/actions` | Actions attribuables (`api::article.article.find`, actions des plugins) |
| `GET /users-permissions/routes` | Routes de l'API de contenu par action : `{"api::article.article.find": [{method, path}]}` |
| `GET /users-permissions/roles/{id}` | Un rôle avec ses `permissions` (liste d'actions) |
| `POST /users-permissions/roles` | `name` (unique), `description`, `permissions`. Le `type` est le nom en snake_case. |
| `PUT /users-permissions/roles/{id}` | `name`, `description`, `permissions` (remplace la liste) |
| `DELETE /users-permissions/roles/{id}` | Ses utilisateurs passent dans Public. Public et Authenticated ne se suppriment pas. |
| `GET` et `PUT /users-permissions/advanced` | Réglages avancés (`unique_email`, `allow_register`, `email_confirmation`, `email_confirmation_redirection`, `email_reset_password`, `default_role`) |
| `GET` et `PUT /users-permissions/email-templates` | Modèles `email_confirmation` et `reset_password` |
| `GET` et `PUT /users-permissions/providers` | Providers OAuth et `email` |

Un `PUT` de réglages ne met à jour que les clés connues du core store, en fusionnant les objets (un provider garde son `callbackUrl` si le corps ne le donne pas) ; une clé inconnue est refusée.

### E-mail

| Méthode et chemin | Action | Rôle |
|---|---|---|
| `GET /email/settings` | `plugin::email.settings.read` | `{provider, configured, defaultFrom, defaultReplyTo}` (la configuration reste dans `config/plugins`) |
| `POST /email/test` | `plugin::email.settings.read` | `{to}` : envoie un e-mail de test ; l'erreur du fournisseur est renvoyée telle quelle |

### Content-Type Builder

Lecture avec `plugin::content-type-builder.read`. L'écriture n'est possible que sous `grapi develop`, qui recharge l'application quand `src/` change (403 sinon).

| Méthode et chemin | Rôle |
|---|---|
| `GET /content-type-builder/schema` | `contentTypes` et `components` lus depuis leurs fichiers (`attributes` en liste ordonnée avec `name`), plus les types des plugins en lecture seule (`editable: false`), et `develop` |
| `GET /content-type-builder/status` | `{develop, schemaHash}` : le hash change quand le schéma rechargé change |
| `POST /content-type-builder/update-schema` | Lot `{contentTypes: [{action, uid, data}], components: [...]}` avec `action` `create`, `update` ou `delete`. |

Règles du lot :
- Tout ou rien : le lot est appliqué à une copie de `src/` puis chargé ; une erreur (nom réservé ou pas en kebab-case, nom déjà pris, relation vers un type inconnu…) le rejette sans rien écrire.
- Fichiers au format Strapi, écrits de façon atomique : `src/api/<nom>/content-types/<nom>/schema.json` et `src/components/<catégorie>/<nom>.json`.
- `singularName` et `pluralName` ne changent pas à la mise à jour : ils font l'uid et la table.
- Un attribut scalaire avec `renamedFrom` est renommé : une migration SQL renomme sa colonne (valeurs conservées) et sa configuration de vue suit. Relations, médias et composants ne se renomment pas.
- Une relation `oneToOne`, `oneToMany`, `manyToOne` ou `manyToMany` avec `targetAttribute` est écrite des deux côtés (`inversedBy` ici, `mappedBy` sur la cible).
- Supprimer un type retire aussi les relations qui le visent ; supprimer un composant retire ses usages (attributs et zones dynamiques). Les tables restent en base.

### Historique des contenus

Conception propre à grapi (la fonction est Enterprise chez Strapi). Une version est enregistrée à chaque création, enregistrement ou publication depuis l'admin, dans la table `grapi_history_versions` (hors schéma Strapi). Lire demande `read` sur le content type, restaurer `update`.

| Méthode et chemin | Rôle |
|---|---|
| `GET /content-manager/history/{uid}/{documentId}?locale=` | Versions, de la plus récente à la plus ancienne : `{id, status, locale, createdAt, createdBy}` |
| `GET /content-manager/history/versions/{id}` | Une version : `data` (champs lisibles), `unknownFields` (disparus du schéma depuis), `missingFields` (ajoutés depuis), `danglingReferences` (relations et médias supprimés depuis : `{path, target, documentId|fileId}`) |
| `GET /content-manager/history/versions/{id}/diff?against=current\|<id>` | `{changes: [{path, before, after}]}` contre le brouillon actuel (défaut) ou une autre version du document. Composants et zones dynamiques comparés par position (`links.1.url`), relations par ensemble de `documentId`, médias par id. |
| `POST /content-manager/history/versions/{id}/restore` | Remplace le brouillon de la locale par la version, sans toucher la version publiée, et enregistre une nouvelle version. Les relations et médias supprimés depuis sont ignorés. |

Réglages : `admin.history.enabled` (vrai par défaut), `admin.history.retentionDays` (90 par défaut, 0 pour tout garder ; purge au démarrage puis au plus une fois par heure), `admin.history.maxVersionsPerDocument` (par document et locale, 0 par défaut : pas de limite). Supprimer un content type dans le CTB supprime ses versions. `admin.history.origins` (défaut `[admin-panel]`) ajoute les écritures d'autres origines : `admin-token` (API de contenu avec un API token), `content-api` (API de contenu, utilisateurs et public), `document-service` (code du projet, cron, planificateur).

### Journaux d'audit

Conception propre à grapi (la fonction est Enterprise chez Strapi). Une ligne immuable par écriture faite depuis l'admin, dans `grapi_audit_logs` (hors schéma Strapi) ; jamais de mot de passe, hash, jeton ou clé, URL de webhook réduite au schéma et à l'hôte.

| Méthode et chemin | Action | Rôle |
|---|---|---|
| `GET /audit-logs?page=&pageSize=&action=&user=&from=&to=&origin=&uid=&documentId=&locale=` | `admin::audit-logs.read` | Entrées, de la plus récente à la plus ancienne (`action` répétable, dates ISO). Origines : `admin-panel`, `scheduler`, `cli`. `entry.update` porte `changes: [{path, before, after}]` (sans champs privés ni mots de passe). |
| `GET /audit-logs/{id}` | `admin::audit-logs.read` | Une entrée avec son `payload` |
| `GET /audit-logs/export.csv` | `admin::audit-logs.export` | Export CSV en flux des entrées filtrées (au plus `admin.auditLogs.exportMaxRows`, 1 000 000 par défaut) ; l'export est lui-même journalisé |

Réglages : `admin.auditLogs.enabled` (vrai par défaut), `admin.auditLogs.retentionDays` (90 par défaut, 0 pour tout garder).
