---
status: todo
---

# ADM-010 : Extensions UI (custom fields, zones d'injection, menus)

> Option retenue : B (admin maison), sous réserve de `ADM-001`. Conventions UI : voir `ADM-002`.

## Contexte
Chez Strapi, les plugins étendent l'admin au moment du build. Ils peuvent ajouter :
- des liens de menu et des sections de réglages ;
- des champs custom avec leur composant de saisie ;
- des composants dans des zones d'injection (`editView.informations`, `editView.right-links`, `listView.actions`, `preview.actions`...) ;
- des actions de document, d'en-tête et groupées, et des panneaux latéraux ;
- des hooks et des traductions.

Cela suppose de rebuilder l'admin avec Node. grapi vise un binaire unique : les extensions UI doivent pouvoir se charger sans rebuild de la SPA.

## Objectif
Définir et implémenter un mécanisme d'extension de l'admin grapi, utilisable par les plugins internes (i18n, upload, XTR) comme par des plugins tiers, sans recompiler la SPA.

## Périmètre
- **API d'extension côté SPA** (`window.grapi.admin` ou module ESM importé) :
  - `addMenuLink` et `addSettingsLink` ;
  - `customFields.register` (composant de saisie, composant d'affichage en liste, options du CTB) ;
  - `injectComponent(zone, component)`, avec une liste de zones documentée et stable :
    - `editView.informations`, `editView.right-links`, `editView.side-panels` ;
    - `listView.actions`, `listView.bulk-actions` ;
    - `preview.actions`, `homepage.widgets` ;
  - `addDocumentAction`, `addDocumentHeaderAction`, `addBulkAction` ;
  - `registerTranslations(locale, messages)`.
- **Chargement** :
  - chaque plugin grapi peut déclarer un bundle ESM d'extension (embarqué dans le binaire pour les plugins compilés, ou servi depuis un dossier pour les plugins externes) ;
  - la SPA charge au démarrage la liste exposée par `/admin/api/v1/extensions` et importe dynamiquement chaque bundle.
- **Partage des dépendances** : React, React-Query et la librairie de composants UI de grapi sont exposés aux extensions (import map), pour éviter les doublons et garder les conventions UI.
- **Sécurité** : les bundles d'extension viennent du serveur, jamais d'une URL tierce. Le respect de la CSP est documenté.
- **Kit de développement** : un template d'extension (Vite, mode library), avec un exemple de custom field « color » équivalent au plugin color-picker de Strapi.
- Documentation `docs/admin-extensions.md`.

## Hors périmètre
- La compatibilité avec les plugins admin Strapi existants : leurs API React internes ne sont pas reproduites.

## Critères d'acceptation
- [ ] Le custom field `color` de l'exemple s'enregistre côté serveur (`SCH-006`) et côté SPA. Il apparaît dans le CTB et dans la vue édition, et sa valeur est stockée en string.
- [ ] Un plugin ajoute un lien de menu et une page sans rebuild de la SPA, juste en déposant son bundle et en redémarrant grapi.
- [ ] Un composant injecté dans `editView.right-links` reçoit le document courant et son schéma.
- [ ] Une extension qui plante n'empêche pas l'admin de fonctionner : l'erreur est isolée par un error boundary et signalée en toast.
- [ ] Le guide permet d'écrire une extension en partant de zéro, ce qui est vérifié en suivant le guide pas à pas.

## Notes techniques
- Les plugins internes de grapi (i18n, Media Library, XTR) utilisent la même API, ce qui garantit qu'elle est suffisante.
- Versionner l'API d'extension (semver) indépendamment de la SPA.
- **Si l'option A est retenue** : ce ticket se réduit au rebuild de `@strapi/admin` avec les plugins admin choisis. Les plugins tiers imposent de rebuilder avec Node.

## Référence Strapi
- `core/admin/admin/src/StrapiApp.tsx` (API register / bootstrap des plugins)
- `core/content-manager/admin/src/components/InjectionZone.tsx`
- `plugins/color-picker/admin/src/`

## Dépendances
- `ADM-002`, `SCH-006`, `EXT-004`

## Fichiers
- `admin/src/extensions/{api,loader,zones}.ts`
- `internal/plugins/extensions.go`
- `templates/admin-extension/`
- `docs/admin-extensions.md`
