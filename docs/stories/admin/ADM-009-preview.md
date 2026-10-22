# ADM-009 : Preview

> Option retenue : B (admin maison), sous réserve de `ADM-001`. Conventions UI : voir `ADM-002`.

## Contexte
Strapi 5 propose une preview : un handler configurable (`admin.preview.handler`) construit l'URL du front pour un document, et l'admin l'ouvre dans un panneau iframe ou un nouvel onglet. L'« advanced preview » (édition en direct dans l'aperçu) est réservée à l'édition Enterprise. Dans le dépôt Strapi, les dossiers `content-manager/{server,admin}/src/preview/` sont **entièrement sous licence Enterprise**, y compris la preview simple. grapi spécifie donc les deux uniquement à partir de la documentation publique et du comportement observé (clean-room, `SETUP-003`), et les publie sous MIT.

## Objectif
Prévisualiser un document (brouillon ou publié) dans son front depuis la vue édition, puis cliquer dans l'aperçu pour aller au champ correspondant.

## Périmètre
- **Configuration de la preview** :
  - `admin.preview.enabled`, `allowedOrigins` ;
  - handler de construction de l'URL, déclaratif : modèle d'URL par content type avec variables (`{documentId}`, `{slug}`, `{locale}`, `{status}`, et tout champ scalaire du document) ;
  - surcharge possible en Go (mode framework, `EXT-002`).
- **Jeton de preview** :
  - l'URL reçoit un secret partagé, ou un jeton signé à durée courte, pour que le front puisse lire le brouillon via l'API REST avec `status=draft` ;
  - le front peut vérifier ce jeton auprès de grapi.
- **Vue édition** :
  - bouton « Ouvrir l'aperçu », qui ouvre un panneau latéral (iframe) sur desktop et un nouvel onglet sur mobile ;
  - bascule brouillon / publié ;
  - rafraîchissement après chaque enregistrement.
- **Preview avancée** :
  - un script fourni par grapi (`/admin/preview/script`) s'injecte dans le front ;
  - il marque les éléments rendus à partir des champs (attributs `data-grapi-source` produits par le front, ou source maps de contenu exposées par l'API) ;
  - un clic dans l'aperçu fait défiler et sélectionner le champ dans le formulaire ;
  - la saisie dans le formulaire met l'aperçu à jour en direct (via `postMessage`).
- Content source maps (option de l'API REST) : encodage, dans les chaînes renvoyées, de l'origine (type, documentId, champ), activé uniquement pour les requêtes de preview.

## Hors périmètre
- Le rendu côté front : grapi fournit le jeton, le script et les source maps, pas le front.

## Critères d'acceptation
- [ ] Pour un content type avec le modèle `https://site/{locale}/blog/{slug}`, le bouton ouvre la bonne URL avec le jeton.
- [ ] Le jeton permet de lire le brouillon via l'API REST, expire après la durée configurée et n'autorise que la lecture.
- [ ] Une origine absente de `allowedOrigins` ne peut pas embarquer l'admin ni échanger de messages avec lui.
- [ ] En preview avancée, cliquer sur le titre dans l'aperçu sélectionne le champ `title`, et taper dans le champ met l'aperçu à jour en moins de 300 ms.
- [ ] Sans preview configurée, aucun bouton n'apparaît.

## Notes techniques
- La sécurité repose sur la CSP `frame-ancestors` (configurée par `API-008`) et sur le contrôle d'origine de `postMessage`.
- Les content source maps utilisent un encodage invisible (caractères zero-width ou stéganographie de type vercel/stega) : à choisir et documenter.
- **Si l'option A est retenue** : la preview de la SPA Strapi n'est pas utilisable du tout, car son code est sous licence EE. Il faut l'exclure du bundle et fournir la nôtre via une extension.

## Référence Strapi
- **Ne pas ouvrir** `core/content-manager/server/src/preview/` ni `core/content-manager/admin/src/preview/` (licence Enterprise).
- `core/core/src/services/content-source-maps.ts`
- Documentation publique Strapi : « Preview » et « Live preview »

## Dépendances
- `ADM-004`, `API-004`, `API-008`, `EXT-001`, `EXT-002`

## Fichiers
- `admin/src/features/preview/`
- `internal/plugins/preview/{config,token,script}.go`, `internal/httpapi/sourcemaps.go`
