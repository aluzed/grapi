---
status: in-progress
---

# ADM-005 : Content Manager : configuration des vues

> Option retenue : B (admin maison), voir `docs/decisions/D1-admin.md`. Conventions UI : voir `ADM-002`.

## Contexte
Chez Strapi, chaque content type et chaque composant a une configuration de vue stockée dans le core store (`plugin_content_manager_configuration_content_types::<uid>` et `..._components::<uid>`). Elle comprend des réglages globaux, des métadonnées par champ et deux mises en page (colonnes de liste et rangées d'édition). Cette configuration est resynchronisée automatiquement quand le schéma change.

## Objectif
Permettre de configurer la liste et le formulaire de chaque type depuis l'admin, avec des valeurs par défaut générées et une synchronisation automatique au changement de schéma.

## Périmètre
- **Réglages par type** : `bulkable`, `filterable`, `searchable`, `pageSize`, `mainField`, `defaultSortBy`, `defaultSortOrder`, et le mode d'ouverture des relations (`relationOpenMode`).
- **Métadonnées d'édition par champ** : libellé, description, placeholder, visible, éditable, `mainField` pour les relations.
- **Métadonnées de liste par champ** : libellé, recherchable, triable.
- **Mise en page de liste** : choix et ordre des colonnes.
- **Mise en page d'édition** : rangées de champs avec largeurs (4, 6, 8 ou 12 colonnes), réordonnancement par glisser-déposer.
- **Génération par défaut** : quand le schéma change, la configuration est créée ou mise à jour (champs ajoutés, retirés ou renommés), sans perdre les réglages existants.
- Écran « Configurer la vue » accessible depuis la liste et l'édition (permission dédiée).
- Stockage dans le core store (`DB-010`), avec des clés compatibles Strapi pour permettre la reprise d'une base existante (`DB-011`).

## Avancement
- API (`docs/admin-api.md`) : configuration effective par type (stockée au format et sous la clé de Strapi, synchronisée avec le schéma : champs ajoutés en fin de mise en page, champs disparus retirés), configuration par défaut calquée sur Strapi (`createDefaultLayouts`, tailles de `field-sizes`), écriture validée, modèle du Content Manager alimenté par cette configuration. Testé (`TestAdminViewConfiguration`).
- Reste : l'écran « Configurer la vue », le formulaire d'édition selon `editLayout`, la configuration des composants, les renommages d'attributs.

## Hors périmètre
- Les vues elles-mêmes (`ADM-003`, `ADM-004`).

## Critères d'acceptation
- [ ] Sur une base Strapi existante, la configuration enregistrée par Strapi est relue et appliquée telle quelle.
- [ ] Ajouter un champ au schéma l'ajoute en fin de mise en page d'édition, sans toucher aux autres réglages.
- [ ] Renommer un champ via le CTB conserve ses métadonnées (libellé, description).
- [ ] Une configuration invalide (champ inexistant, largeur hors de la grille) est rejetée par le serveur avec une erreur de validation.
- [ ] La mise en page d'édition se configure entièrement au clavier et à la souris, et reste lisible à 375 px (rangées empilées).

## Notes techniques
- Algorithme de synchronisation : partir de la configuration stockée, retirer les champs disparus, ajouter les nouveaux avec des métadonnées par défaut (libellé = nom du champ), et recalculer les rangées.
- Le `mainField` par défaut est le premier attribut string non privé, sinon `documentId`.
- **Si l'option A est retenue** : endpoints `/content-manager/content-types/:uid/configuration` et `/content-manager/components/:uid/configuration` à reproduire, ainsi que `/content-manager/init`.

## Référence Strapi
- `core/content-manager/server/src/services/utils/configuration/{settings,metadatas,layouts}.ts`
- `core/content-manager/server/src/services/utils/store.ts`
- `core/content-manager/server/src/bootstrap.ts` (synchronisation au démarrage)

## Dépendances
- `ADM-002`, `DB-010`, `SCH-001`, `SCH-002`, `AUTH-001`

## Fichiers
- `admin/src/pages/content-manager/configure/`
- `internal/plugins/contentmanager/configuration.go`, `internal/plugins/contentmanager/sync.go`
