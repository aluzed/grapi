---
status: done
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
- Écran « Configurer la vue » (depuis la liste, ou l'édition d'un single type) : réglages, colonnes de la liste (ordre, libellés, ajout et retrait), formulaire (ordre, largeur 4, 6, 8 ou 12, libellé, description, texte indicatif, visible, modifiable), regroupé en lignes de 12 à l'enregistrement. Le formulaire d'édition suit la mise en page (12 colonnes sur desktop, empilé sur mobile), les libellés, les descriptions, et masque ou verrouille les champs. Vérifié dans Chrome.
- Fidélité à Strapi 5.59 : la configuration par défaut est octet pour octet celle que Strapi écrit (article, homepage, tag, users-permissions.user, tirées de son export du core store, `compat/fixtures/basic/strapi-view-configurations.json`, `TestAdminDefaultConfigurationIsStrapis`). Une configuration stockée par Strapi est servie telle quelle et n'est pas réécrite (`TestAdminReadsStrapiConfiguration`). La synchronisation suit `syncConfiguration` : un champ retiré du formulaire n'y revient pas, seuls les attributs nouveaux sont ajoutés en fin ; `relationOpenMode` est conservé ; `config.attributes.<nom>.hidden` est respecté.
- Ajout d'un champ au schéma, renommage avec conservation du libellé et de la description, réglages intacts (`TestAdminConfigurationFollowsSchema`) ; configurations invalides refusées (`TestAdminConfigurationValidation`).
- Écran : retrait d'un champ du formulaire et « Ajouter un champ » (l'EditView n'affiche plus que la mise en page configurée, comme Strapi). Vérifié dans Chrome, y compris à 375 px ; toutes les commandes sont des boutons et listes natifs, utilisables au clavier.
- Reste, hors critères : la configuration des composants, le glisser-déposer, `config.settings|metadatas|layouts` du schéma comme valeurs par défaut.

## Hors périmètre
- Les vues elles-mêmes (`ADM-003`, `ADM-004`).

## Critères d'acceptation
- [x] Sur une base Strapi existante, la configuration enregistrée par Strapi est relue et appliquée telle quelle.
- [x] Ajouter un champ au schéma l'ajoute en fin de mise en page d'édition, sans toucher aux autres réglages.
- [x] Renommer un champ via le CTB conserve ses métadonnées (libellé, description).
- [x] Une configuration invalide (champ inexistant, largeur hors de la grille) est rejetée par le serveur avec une erreur de validation.
- [x] La mise en page d'édition se configure entièrement au clavier et à la souris, et reste lisible à 375 px (rangées empilées).

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
