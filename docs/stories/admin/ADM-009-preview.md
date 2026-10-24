---
status: done
---

# ADM-009 : Preview

> Option retenue : B (admin maison), voir `docs/decisions/D1-admin.md`. Conventions UI : voir `ADM-002`. Le code preview de Strapi est sous licence Enterprise (D6) : ce ticket se spécifie uniquement à partir de la documentation publique.

## Contexte
Strapi affiche le front du projet dans l'admin, depuis la vue édition. La documentation publique décrit : un bloc `preview` dans la config admin (`enabled`, `allowedOrigins`, `handler` qui construit l'URL par document et peut renvoyer `null`), un bouton « Open preview » désactivé tant que des modifications ne sont pas enregistrées, des onglets brouillon / publié, une bascule Desktop / Mobile, la copie du lien, un affichage plein écran ou côte à côte, et des messages `postMessage` (`strapiUpdate`, `previewReady`). Chez Strapi, le côte à côte est payant ; dans grapi tout est inclus.

## Objectif
Prévisualiser le document en cours d'édition dans le vrai front, en brouillon ou publié, avec rafraîchissement automatique.

## Périmètre
- **Configuration** (`config/admin.yaml`, `EXT-001`) :
  ```yaml
  preview:
    enabled: true
    allowedOrigins: [${env:CLIENT_URL}]
    handlers:
      api::post.post: "${env:CLIENT_URL}/{locale}/blog/{slug}?status={status}&token={previewToken}"
      api::homepage.homepage: "${env:CLIENT_URL}/{locale}"
  ```
  Placeholders : `{documentId}`, `{locale}`, `{status}`, `{uid}`, `{previewToken}` et tout attribut scalaire (`{slug}`), encodés pour l'URL. Content type absent : pas de preview.
- **Handler Go** (`EXT-002`) : `app.Preview().Handle(uid, func(ctx, req PreviewRequest) (string, error))`, prioritaire sur le template. Une chaîne vide masque la preview pour ce document.
- **API admin** : `GET /admin/api/v1/preview/url/:uid?documentId=&locale=&status=` renvoie `{url}` ou 204.
- **Jeton de preview** : `{previewToken}` est un jeton signé, durée 1 h, limité à la lecture de ce document (toutes locales, `status=draft` autorisé), accepté par le Content API en `Authorization: Bearer` ou `?previewToken=`.
- **Sécurité** : `allowedOrigins` alimente `frame-src` de la CSP de l'admin (`API-008`) ; une URL générée hors de ces origines est refusée (erreur explicite dans le panneau).
- **Vue édition** (`ADM-004`) : bouton « Ouvrir la preview » (icône + texte + `title`), désactivé avec `title` explicatif tant que le formulaire a des modifications non enregistrées.
- **Panneau** :
  - onglets Brouillon / Publié (si D&P ; Publié désactivé si jamais publié) ;
  - bascule Desktop / Mobile (largeur d'iframe 375 px) ;
  - « Copier le lien », « Ouvrir dans un nouvel onglet », « Fermer » ;
  - modes plein écran et côte à côte (formulaire + iframe) ; sur mobile et sous `lg`, plein écran seulement ;
  - préférence de mode mémorisée par utilisateur.
- **Mise à jour à l'enregistrement** : après chaque enregistrement, l'admin envoie `{type: 'strapiUpdate'}` à l'iframe (`targetOrigin` = origine de l'URL, jamais `*`) ; sans écouteur côté front, l'iframe est rechargée.
- **Handshake** : le front envoie `{type: 'previewReady'}` ; l'admin n'envoie des messages qu'après ce signal et ignore les messages d'origine non autorisée.

## Hors périmètre
- **Optionnel, ticket séparé** : mise à jour en direct pendant la frappe, avant enregistrement (message `grapiDraftChange` avec les valeurs du formulaire, debounce 300 ms), à activer par `preview.live: true`.
- **Optionnel** : édition visuelle (double-clic dans la preview vers le champ, content source maps dans les réponses REST).
- **Optionnel** : handler JS/TS (`EXT-003`, phase 3), reprise directe d'un `config/admin.ts` Strapi.
- Preview des médias et des composants isolés.

## Critères d'acceptation
- [x] Avec le template ci-dessus, un post `slug: hello` en `fr` brouillon ouvre `.../fr/blog/hello?status=draft&token=...`.
- [x] Un content type sans handler n'affiche pas le bouton ; un handler Go qui renvoie `""` non plus.
- [x] Le bouton est désactivé après une frappe non enregistrée, et réactivé après enregistrement.
- [x] L'onglet Publié génère `status=published` ; il est désactivé pour un document jamais publié.
- [x] Le jeton de preview lit ce document en brouillon et reçoit 403 sur un autre document ; expiré, il reçoit 401.
- [x] Une URL hors `allowedOrigins` n'est pas chargée dans l'iframe.
- [x] Un front de test qui écoute `strapiUpdate` reçoit le message après enregistrement ; un message d'une origine non autorisée est ignoré par l'admin.
- [x] Côte à côte disponible à partir de `lg`, plein écran seul en dessous ; à 375 px, aucun scroll horizontal et boutons pleine largeur.

## Avancement
- Serveur : `admin.preview` (`enabled`, `allowedOrigins`, `handlers`), `app.PreviewHandler` prioritaire, `GET /admin/api/v1/preview/url/{uid}` (`{url, origin}`, 204 sans preview, 400 hors `allowedOrigins`), `frame-src` de la CSP de l'admin, jeton de preview signé avec la clé admin (fenêtre de 10 minutes, valable 50 minutes au moins : l'URL ne change pas entre deux enregistrements), lu en `Bearer` ou `?previewToken=`, limité à `find` (filtré sur le document) et `findOne` de ce document (`TestPreview`, `TestPreviewTokenExpires`).
- Vue édition : bouton « Ouvrir l'aperçu » (désactivé avec explication tant que le formulaire a des modifications), panneau en plein écran ou côte à côte (à partir de `lg`, mode retenu dans le navigateur), onglets Brouillon / Publié (désactivé pour un document jamais publié), Bureau / Mobile (375 px), copie du lien, nouvel onglet. Après un enregistrement, `strapiUpdate` part vers le front qui a envoyé `previewReady` (origine et fenêtre vérifiées) ; sinon le cadre est rechargé ; une URL qui change (slug) recharge d'elle-même. Vérifié dans le navigateur avec un front de test sur une autre origine.
- La preview en direct pendant la frappe reste hors périmètre (ticket séparé).

## Notes techniques
- Les noms de messages `strapiUpdate` et `previewReady` sont repris de la doc publique pour qu'un front déjà branché sur Strapi marche sans changement.
- Jeton de preview : JWT HS256 signé avec la clé admin, claims `uid`, `documentId`, `exp` ; vérifié par un middleware du Content API qui force la portée lecture sur ce document.
- Composants : `admin/src/pages/content-manager/preview/PreviewPanel.tsx`, `usePreviewUrl` (React-Query), `usePreviewMessaging`.

## Référence
- Preview : https://docs.strapi.io/cms/features/preview
- Spécification clean-room : aucune source Strapi sous licence Enterprise consultée.

## Dépendances
- `ADM-002`, `ADM-004`, `EXT-001`, `EXT-002`, `API-008`, `AUTH-008`, `DOC-002`, `PLG-001`, `SETUP-003`

## Fichiers
- `internal/httpapi/admin/preview.go`, `internal/auth/previewtoken/`
- `grapi.go` (enregistrement `Preview()`)
- `admin/src/pages/content-manager/preview/`
