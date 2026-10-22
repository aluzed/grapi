---
status: todo
---

# DB-010 : Core store (`strapi_core_store_settings`)

## Contexte
Strapi range ses réglages persistés dans une table clé-valeur, `strapi_core_store_settings` :

- configuration des vues du Content Manager ;
- réglages users-permissions (`advanced`, `grant`, `email`) ;
- réglages upload ;
- locale par défaut ;
- schémas des content types (sauvegardés au bootstrap).

Pour la compatibilité de base (`DB-011`) et `configuration:dump/restore` (`CLI-003`), grapi utilise la même table et les mêmes clés.

## Objectif
Un service clé-valeur typé au-dessus de `strapi_core_store_settings`, avec le même schéma de clés que Strapi.

## Périmètre
- **Table** : `id`, `key` (varchar), `value` (text contenant du JSON sérialisé), `type` (`object`, `string`, `boolean`, `number`, `array`), `environment` (varchar nullable) et `tag` (varchar nullable).
- **API** :
  - `Store(scope{type: core|plugin, name, environment?, tag?})` avec `Get(key)`, `Set(key, value)` et `Delete(key)` ;
  - clé effective : `<type>_<name>_<key>`, par exemple `plugin_users-permissions_advanced` ou `core_admin_auth` (format exact à relever sur une base de référence) ;
  - `type` est déduit de la valeur Go (`object`, `array`, `string`, `number`, `boolean`) ;
  - `value` est stocké en JSON, et relu en JSON selon `type`.
- **Clés utilisées par grapi** (relevées sur une base Strapi 5.59 et documentées dans `docs/core-store-keys.md`) :
  - `plugin_content_manager_configuration_content_types::<uid>` et `..._components::<uid>` (`ADM-005`) ;
  - `plugin_users-permissions_advanced`, `_grant` et `_email` (`AUTH-002`, `AUTH-005`) ;
  - `plugin_upload_settings`, `_view_configuration` et `_metrics` (`MEDIA-006`) ;
  - `plugin_i18n_default_locale` (`PLG-001`) ;
  - `core_admin_project-settings`, `core_admin_auth` (`ADM-008`) ;
  - `model_def_<uid>`, si Strapi stocke encore les définitions de modèles (à vérifier : `strapi::content-types` sauvegardés au bootstrap).
- **Cache** : cache mémoire par clé, invalidé à chaque `Set` ou `Delete` de la même instance. Le multi-instance est documenté comme limite (pas de bus d'invalidation en phase 1).

## Hors périmètre
- Le contenu métier de chaque clé (traité par les tickets consommateurs).

## Critères d'acceptation
- [ ] Une valeur écrite par grapi est relue à l'identique par Strapi 5.59, et inversement (sur la fixture `basic`, pour les types `object`, `string`, `boolean`, `number` et `array`).
- [ ] Sur la copie de la base dev-freelance, grapi relit la configuration des vues du Content Manager et les réglages users-permissions existants.
- [ ] `docs/core-store-keys.md` liste toutes les clés présentes dans une base Strapi 5.59 fraîche, avec leur ticket consommateur.
- [ ] `Get` sur une clé absente renvoie `nil` sans erreur.

## Notes techniques
- La colonne `value` est du texte qui contient du JSON, même pour une chaîne (`"\"en\""`). Le relever exactement, sinon la relecture côté Strapi échoue.

## Référence Strapi
- `core/core/src/services/core-store.ts`
- `core/core/src/providers/core-store.ts`

## Dépendances
- `DB-001`, `DB-003`.

## Fichiers
- `internal/db/corestore/store.go`, `internal/db/corestore/store_test.go`, `docs/core-store-keys.md`
