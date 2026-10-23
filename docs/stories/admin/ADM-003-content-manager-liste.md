---
status: in-progress
---

# ADM-003 : Content Manager : vue liste

> Option retenue : B (admin maison), voir `docs/decisions/D1-admin.md`. Conventions UI : voir `ADM-002`.

## Contexte
Le Content Manager liste les documents de chaque collection type. On y trouve la recherche, les filtres, le tri, la pagination, le choix des colonnes, le statut draft/published, la locale et les actions groupées : publier, dépublier, supprimer.

## Objectif
Offrir pour chaque collection type une vue liste générique pilotée par le schéma et par la configuration de vue (`ADM-005`).

## Périmètre
- Menu de gauche du Content Manager : collection types et single types, triés par nom d'affichage, avec une recherche dans le menu. Seuls les types que l'utilisateur peut lire apparaissent.
- Table :
  - colonnes configurées (`ADM-005`), avec le champ principal des relations affiché (`mainField`) ;
  - miniature pour les champs média ;
  - badge de statut : Brouillon, Publié, Modifié.
- Recherche plein texte (`_q`), constructeur de filtres par attribut avec les opérateurs adaptés au type (texte, nombre, date, booléen, énumération, relation), tri par colonne, pagination avec taille de page configurable.
- Sélecteur de locale si le type est localisé (`PLG-001`) et sélecteur de statut (brouillon ou publié).
- État de la vue (filtres, tri, page, locale) dans l'URL, donc partageable.
- Actions groupées : publier, dépublier, supprimer, avec une modale récapitulative. La publication groupée affiche les documents invalides et les relations en brouillon (équivalent de `bulkFindForValidation` et `countManyEntriesDraftRelations`).
- Actions de ligne dans `EntityActionsMenu` : modifier, dupliquer (clone), publier ou dépublier, supprimer.
- Sur mobile, la table devient une liste de cartes empilées.
- Bouton « Créer une entrée » en en-tête (`w-full md:w-auto`).

## Hors périmètre
- La vue édition (`ADM-004`).
- La configuration des colonnes elle-même (`ADM-005`).

## Critères d'acceptation
- [x] Les résultats d'une combinaison filtres + tri + recherche + locale + statut sont identiques à l'appel REST équivalent sur le Document Service.
- [x] Recharger la page restaure exactement la vue (état dans l'URL).
- [x] La publication groupée de 50 documents, dont 3 invalides, publie les 47 valides et liste les 3 en erreur avec le motif.
- [x] Chaque suppression, unitaire ou groupée, passe par la modale de confirmation.
- [ ] Les permissions sont respectées :
  - un utilisateur sans `delete` ne voit pas l'action supprimer ;
  - un champ non lisible n'apparaît pas en colonne ;
  - la condition `is-creator` filtre les lignes.
- [ ] À 375 px, la liste est en cartes, sans scroll horizontal.

## Avancement
- API (`docs/admin-api.md`, Content Manager) : modèles lisibles avec colonnes, `mainField` et réglages (configuration de vue Strapi lue dans le core store), liste paginée avec recherche `_q`, filtres JSON, tri, locale et statut, statut draft/published/modified par document, suppression unitaire, actions groupées (supprimer, publier, dépublier) avec rapport partiel. Filtre RBAC des conditions et champs lisibles appliqués. Testé de bout en bout (`TestAdminContentManager`) : publication groupée avec un document invalide, Author limité à ses entrées.
- Interface (`admin/src/pages/content-manager/`) : menu des types avec recherche (select sur mobile), table avec tri par colonne, recherche, onglets Brouillons / Versions publiées, sélecteur de locale, pagination et taille de page, état complet dans l'URL, badges de statut, sélection et actions groupées avec confirmation et rapport partiel, menu engrenage par ligne, cartes sur mobile, bouton « Créer une entrée ». Vérifié dans Chrome sur le binaire, y compris le rechargement d'une URL de liste.
- Constructeur de filtres : champ, opérateur selon le type (texte, nombre, date, booléen, énumération, relation filtrée sur le champ principal de la cible, média vide ou non), filtres combinés en ET, affichés en pastilles retirables, conservés dans l'URL (`admin/src/pages/content-manager/filters.ts`, testé). Vérifié dans Chrome, rechargement compris.
- Tests (`admin_cm_test.go`) : la liste admin avec filtres, tri, recherche, locale et statut est identique au Document Service (`TestAdminListMatchesDocumentService`) ; 50 documents dont 3 invalides publient les 47 valides et nomment le champ fautif des 3 autres (`TestAdminBulkPublishFifty`).
- Reste : la miniature et le `mainField` des relations vérifiés sur un vrai jeu de données, le rapport détaillé de publication groupée (relations en brouillon), les tests à 375 px automatisés.

## Notes techniques
- La liste s'appuie sur un endpoint admin qui réutilise le Document Service avec la sanitization RBAC (`AUTH-001`), et non sur l'API de contenu publique.
- Les actions groupées sont traitées en une transaction par document. Le rapport d'erreurs est partiel, pas tout ou rien.
- **Si l'option A est retenue** : implémenter `/content-manager/collection-types/:model` (GET avec les paramètres du CM), `actions/bulkPublish`, `bulkUnpublish`, `bulkDelete`, `actions/bulkFindForValidation` et `actions/countManyEntriesDraftRelations` selon `content-manager/shared/contracts/collection-types.ts`.

## Référence Strapi
- `core/content-manager/server/src/routes/admin.ts`, `core/content-manager/server/src/controllers/collection-types.ts`
- `core/content-manager/admin/src/pages/ListView/`
- `core/content-manager/shared/contracts/collection-types.ts`

## Dépendances
- `ADM-002`, `ADM-005`, `DOC-001`, `DOC-002`, `DB-005`, `AUTH-001`, `PLG-001`

## Fichiers
- `admin/src/pages/content-manager/list/`, `admin/src/features/filters/`
- `internal/httpapi/admin/contentmanager/list.go`, `internal/httpapi/admin/contentmanager/bulk.go`
