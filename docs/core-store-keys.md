# Core store keys

The keys of `strapi_core_store_settings` in a fresh Strapi 5.59 database (reference image of the compat bench, fixture `basic`, SQLite), with the grapi ticket that reads or writes them. `value` is JSON text, even for a string (`"en"` is stored as `"\"en\""`), and `type` is JavaScript's `typeof` of the value, so arrays and `null` are `object`. Service: `internal/db/store`.

| Key | Type | Ticket |
|---|---|---|
| `core_admin_auth` | object | `ADM-008` |
| `plugin_content_manager_configuration_content_types::<uid>` (one per content type, admin and plugins included) | object | `ADM-005` |
| `plugin_content_manager_configuration_components::<uid>` (one per component, when the project has some) | object | `ADM-005` |
| `plugin_i18n_default_locale` | string | `PLG-001` (read by the document service) |
| `plugin_documentation_config` (with the documentation plugin: `restrictedAccess`, bcrypt `password`) | object | `PLG-005` |
| `plugin_upload_settings` | object | `MEDIA-002`, `MEDIA-006` (read by the upload route) |
| `plugin_upload_view_configuration` | object | `MEDIA-006` |
| `plugin_upload_metrics` | object | `MEDIA-006` |
| `plugin_upload_api-folder` | object | `MEDIA-001` (written on the first API upload: `{"id": <folder id>}`) |
| `plugin_users-permissions_advanced` | object | `AUTH-002` |
| `plugin_users-permissions_grant` | object | `AUTH-002`, `AUTH-005` |
| `plugin_users-permissions_email` | object | `AUTH-002`, `PLG-002` |
| `strapi_content_types_schema` | object | `DB-003` |
| `strapi_unidirectional-join-table-repair-ran` | boolean | `DB-003` |

Rows carry `environment` and `tag` columns; every key above has both null.
