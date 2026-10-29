# Audit de sécurité grapi : 29 octobre 2026

Périmètre : grapi (`main` à `3b5e360`) et le serveur privé de dev-freelance (`apps/grapi`). Méthode : lecture du code par zone (auth, exposition de données, admin, uploads et backend privé), comparaison avec le source de Strapi 5.59 (`apps/backend/node_modules/@strapi`), sondes GET anonymes comparées entre `bo.dev-freelance.fr` (Strapi) et `bo2.dev-freelance.fr` (grapi), lecture seule des permissions des rôles en prod.

Branche : `security-audit-20261029`, 14 commits, non poussée. `go vet ./...` propre, `go test ./...` : 421 tests OK. Le backend dev-freelance (`backend/`, `cmd/`) passe ses tests contre cette branche.

## Résumé

| # | Gravité | Faille | État |
|---|---|---|---|
| 1 | Critique | Content Manager : oracle sur `createdBy.resetPasswordToken` via les filtres | Corrigé |
| 2 | Critique | Content Manager : relations peuplées avec hash de mot de passe et tokens | Corrigé |
| 3 | Haute | Corps de requête sans limite (256 Mo) sans Content-Type JSON, anonyme | Corrigé |
| 4 | Haute | GIF animé décodé en entier avant la limite de pixels | Corrigé |
| 5 | Haute | Content-Type Builder : path traversal (develop seulement) | Corrigé |
| 6 | Moyenne | Sélecteur de relations : champ principal d'une cible illisible | Corrigé |
| 7 | Moyenne | GraphQL : sortie non nettoyée (champs password, sanitizers) | Corrigé |
| 8 | Moyenne | `privateAttributes` (config et schéma) ignorés | Corrigé |
| 9 | Moyenne | Médiathèque : fichier lu par id sans les conditions | Corrigé |
| 10 | Moyenne | Admin `forgot-password` sans rate limit | Corrigé |
| 11 | Basse | Multipart : nombre de champs illimité | Corrigé |
| 12 | Basse | Dotfiles servis depuis `public/` | Corrigé |
| 13 | Basse | `populate[rel]=*` imbriqué accepté (Strapi : 400) | Corrigé |
| 14 | Basse | Rate limit GraphQL : IP du proxy avec un `ipHeader` custom | Corrigé |
| 15 | Basse | API token : expiration illisible acceptée | Corrigé |
| 16+ | Info | Comportements identiques à Strapi | Signalés, non corrigés |

Les sondes en prod n'ont montré aucune fuite de champ privé sur l'API publique : grapi rejette comme Strapi les filtres, tris et `fields` sur `password`, `email`, `resetPasswordToken`, `createdBy`, et les injections dans `sort`. Le seul écart observé était le n°13.

## Détail des failles corrigées

### 1. Oracle sur les tokens de reset via les filtres du Content Manager (critique)
- Où : `internal/adminapi/cm.go` (`cmList`), `internal/document/params.go`.
- Scénario : un admin ayant seulement `explorer.read` sur un type déclenche `forgot-password` pour un super admin, puis enchaîne `GET .../content-manager/collection-types/api::post.post?filters={"createdBy":{"resetPasswordToken":{"$startsWith":"8"}}}`. Le `total` (0 ou 1) révèle le token caractère par caractère, puis `reset-password` donne une session super admin. Même oracle sur tout champ privé ou illisible.
- Strapi : `permissionChecker.sanitizedQuery.read` retire ces clés.
- Correctif : `internal/adminapi/cm_sanitize.go`. Filtres et tri ne gardent que ce que l'utilisateur peut lire, à toute profondeur ; jamais `password` ; pour `admin::user`, seulement `ADMIN_USER_ALLOWED_FIELDS`. Dans une relation, il faut le droit de lecture sur le champ de la cible (plus strict que Strapi).
- Test : `TestAdminCMFiltersOnlyWhatTheUserReads`.

### 2. Secrets dans les relations peuplées du Content Manager (critique)
- Où : `cm.go` (`deepPopulate`, `cmFindOne`, `stFind`), `history.go`.
- Scénario : un lecteur ayant accès à `post.author` lit un post et reçoit `author.password` (bcrypt), `resetPasswordToken` (en clair) et `confirmationToken` : prise de contrôle du compte utilisateur.
- Strapi : les relations reviennent en comptes ou nettoyées.
- Correctif : `visible()` nettoie maintenant à toute profondeur (relations, médias, composants, zones dynamiques, morph) selon le schéma de la cible et les droits. Liste, fiche, single type, écritures et historique y passent.
- Test : `TestAdminCMHidesRelatedSecrets`.

### 3. Corps de requête sans limite (haute, anonyme)
- Où : `internal/httpapi/body.go`, `users.go` (`readBody`), `server.go` (`readData`), `ratelimit.go`, `graphql.go`.
- Scénario : quelques `POST /api/auth/local` sans Content-Type avec 200 Mo de corps : chaque requête est lue en mémoire (jusqu'à 256 Mo) et parsée en JSON, avant même le rate limit. Le conteneur tombe.
- Strapi : koa-body ne parse que le JSON déclaré, plafonné à 1 Mo.
- Correctif : `readJSONBody` ne lit que les corps déclarés JSON (déjà plafonnés par `strapi::body`).
- Test : `TestOnlyJSONBodiesAreRead`.

### 4. GIF bombe (haute, nécessite un droit d'upload)
- Où : `internal/media/image.go` (`Probe`), `internal/httpapi/upload.go`.
- Scénario : un GIF de 2 Mo déclarant des frames de 20000x20000 : `gif.DecodeAll` alloue des gigaoctets avant le contrôle `limitInputPixels`.
- Strapi : sharp vérifie la limite avant de décoder, et ne lit que la première frame.
- Correctif : les frames sont comptées en parcourant les blocs, sans décodage. Les formats d'un GIF animé (qui décodent toutes les frames) ne sont produits que si l'ensemble tient dans `limitInputPixels`.
- Test : `TestProbeDoesNotDecodeGIFFrames`.

### 5. Path traversal dans le Content-Type Builder (haute, `grapi develop` seulement)
- Où : `internal/adminapi/ctb.go` (`ctPath`, `componentPath`, `apply`).
- Scénario : un admin avec `content-type-builder.read` envoie `uid: "api::a./../../../<chemin>"` en `delete` : `os.RemoveAll` hors du projet.
- Correctif : noms limités à `^[A-Za-z0-9_-]+$`, un type ou composant supprimé doit exister, `apply` refuse toute écriture hors de `src`.
- Test : `TestAdminCTBRefusesPathsOutOfTheProject`.
- En prod, le CTB est déjà en lecture seule.

### 6. Sélecteur de relations (moyenne)
- Où : `cm.go` (relations).
- Scénario : un lecteur sans accès aux utilisateurs liste tous les noms (ou e-mails) via `GET /content-manager/relations/api::post.post/author`. `_q` cherchait dans tous les champs texte de la cible, e-mail compris.
- Strapi : `sanitizeMainField` renvoie `documentId` si le champ principal est illisible.
- Correctif : même règle ; `_q` limité au champ principal ; avec `?component=`, un chemin lisible vers le champ est exigé.
- Tests : `TestAdminRelationPickerFollowsReadPermissions`, `TestAdminRelationPickerInComponentChecksTheField`.

### 7. Sortie GraphQL non nettoyée (moyenne)
- Où : `internal/httpapi/graphql.go`.
- Scénario : un champ `password` non marqué `private` renvoyait son hash ; les sanitizers `content-api.output` du projet n'étaient pas appliqués.
- Correctif : `sanitizeOutput` sur les requêtes racine, relations, composants, zones dynamiques et mutations. Les champs password restent dans le SDL (parité supposée avec Strapi, son plugin GraphQL n'est pas installé localement pour vérifier) mais renvoient `null`.
- Test : `TestGraphQLSanitizesOutput`.
- dev-freelance n'active pas GraphQL.

### 8. `privateAttributes` ignorés (moyenne)
- Où : `schema/attribute.go`, `internal/httpapi/sanitize.go`.
- Scénario : un projet qui masque un champ via `api.responses.privateAttributes` ou `options.privateAttributes` le voyait exposé et filtrable.
- Strapi : `getStoredPrivateAttributes` fusionne les deux.
- Correctif : `schema.MarkPrivateAttributes`, appliqué au chargement ; `IsPrivate()` en tient compte partout (REST, GraphQL, OpenAPI, typegen, webhooks).
- Test : `TestStoredPrivateAttributes`.
- dev-freelance n'utilise pas ces options.

### 9. Médiathèque : lecture par id (moyenne)
- Où : `internal/adminapi/media.go`.
- Scénario : un auteur limité à ses fichiers lit ceux des autres en énumérant les ids.
- Correctif : `fileAllowed` applique les conditions de `upload.read` à `GET /upload/files/{id}` et `/related` (404 sinon).
- Test : `TestAdminOwnFilesOnlyReadsOwnFileByID`.

### 10. Admin `forgot-password` sans rate limit (moyenne)
- Où : `internal/adminapi/auth.go`.
- Impact : flood d'e-mails, et alimentait le n°1.
- Correctif : 5 requêtes par 5 minutes par e-mail et IP, comme Strapi. La limite suit `admin.rateLimit` (grapi ne lit pas `plugin::email.ratelimit`, mêmes valeurs par défaut).
- Test : `TestAdminForgotPasswordIsRateLimited`.

### 11 à 15 (basses)
- Multipart : 1000 champs et 20 Mo de champs au plus, 413 au-delà, comme formidable (`upload.go`, `TestMultipartFieldLimits`).
- Dotfiles : un segment commençant par `.` donne 404, comme koa-send (`upload.go`, `TestPublicHidesDotfiles`).
- `populate[rel]=*` imbriqué : 400 `Invalid key related at cover.related` / `Invalid key picture at author.picture`, comme Strapi (`sanitize.go`, `TestNestedWildcardPopulate`).
- Rate limit GraphQL : l'en-tête `server.proxy.ipHeader` est recopié sur la requête rejouée (`graphql.go`, `TestGraphQLRESTKeepsClientIP`).
- API token : un `expires_at` illisible donne 401 (`auth.go`, `TestAPITokenUnreadableExpiry`).

## Non corrigé : comportements identiques à Strapi

Les corriger casserait la parité ou un usage légitime ; à traiter par configuration si besoin.

- `PUT /api/users/:id` écrit le corps brut (rôle, mot de passe) si un rôle a `user.update`. Non accordé en prod.
- Jetons de reset et de confirmation sans expiration ; JWT legacy valides après changement de mot de passe.
- Énumération d'utilisateurs (timing du login, messages de `register` et `send-email-confirmation`).
- `_q` cherche aussi dans les colonnes privées pour qui a `find` (Strapi idem).
- Filtres dans `populate[x][filters]` sans contrôle de permission de la relation (Strapi idem).
- `status=draft` lisible par qui a `find` (Strapi 5 documenté).
- Upload de `.html`/`.svg` servi en same-origin (XSS stockée) : réservé aux admins en prod, aucun rôle public n'a `upload.upload`. Recommandation : `deniedTypes` (`text/html`, `image/svg+xml`, `application/javascript`) dans `plugins.upload.config.security`.
- Remplacement, rattachement et suppression de n'importe quel fichier par qui a `upload` : idem, admins seulement en prod.
- Webhooks : protection SSRF seulement à l'enregistrement ; `server.webhooks.blockPrivateNetworks` est désactivé par défaut. Recommandation : l'activer.
- Historique de contenu sans conditions par ligne.
- GraphQL sans `depthLimit`/`maxLimit` par défaut.
- Rate limit : derrière un proxy non déclaré, tous les clients partagent l'IP du proxy ; avec `proxy: true` sans `maxIpsCount`, `X-Forwarded-For` est falsifiable. Recommandation : déclarer le proxy Traefik avec `maxIpsCount: 1`.
- Image fixe sous la limite de pixels (268 M) décodée en ~1 Go : même limite que Strapi.
- E-mails : formulaire de contact public et réponses aux commentaires sans limite (même chose dans le JS d'origine).

## Configuration de la prod (lecture seule, 29/10)

- Public et Authenticated n'ont aucun droit `upload.upload`, `post.create/update`, `comment.delete` : l'upload malveillant, l'abus du LLM et la suppression de commentaires ne sont accessibles qu'aux admins.
- `subscriber.update` est ouvert à Public et Authenticated : quiconque connaît le `documentId` d'un abonné (24 caractères aléatoires, non listables) peut changer son e-mail ou le désactiver. Même chose sous Strapi. À fermer si le front ne s'en sert pas pour la désinscription.
- `user.find`/`findOne` sont accordés à Public pour peupler `author`, et bloqués par la policy de dev-freelance (403 vérifié sur bo2).

## Backend dev-freelance

Aucune faille. Le port Go est plus strict que le JS d'origine : création de commentaire limitée aux champs utiles (le JS laissait passer `upVotes`, `hidden`), e-mails échappés, compteur de favoris recalculé. Clés Redis constantes, anonymisation protégée par token et mot de passe, LLM déclenché seulement par une écriture de post.

## Avant de déployer

- Vérifier l'admin grapi à la main (édition d'un post avec auteur, catégorie, thème, cover ; sélecteur de relations), car le nettoyage des relations peuplées change ce que reçoit le SPA.
- Puis : pousser grapi, `go mod tidy` dans `apps/grapi` si besoin, redéployer selon `apps/grapi/deploy/README.md`.
