# XTR-002 : Audit logs

> **Clean-room** : fonction sous licence Enterprise chez Strapi. Spécification rédigée uniquement à partir de la documentation publique de Strapi et du comportement observable. Il est **interdit d'ouvrir** les dossiers `ee/`, les paquets `core/review-workflows` et `core/content-releases`, ainsi que `core/content-manager/{admin,server}/src/{history,preview}` (voir `SETUP-003`). Dans grapi, la fonction est sous MIT, sans clé de licence ni limite (nombre de workflows, de releases, durée de rétention...).

## Contexte
Strapi Enterprise enregistre les actions des administrateurs : connexions, opérations sur les contenus, les médias, les content types, les utilisateurs, les rôles, les permissions et les réglages. Ces logs se consultent dans Settings > Audit Logs, avec filtres et détail, et une rétention est configurable.

## Objectif
Tracer de façon fiable toutes les actions d'administration et les rendre consultables, filtrables et exportables.

## Périmètre
- **Événements tracés**, chacun avec : action, date, utilisateur, IP, user agent, et un payload qui contient l'objet concerné et le diff pour les mises à jour :
  - admin : `admin.auth.success`, `admin.logout`, échecs de connexion ;
  - contenu : `entry.create`, `update`, `delete`, `publish`, `unpublish`, `draft-discard` ;
  - médias : `media.create`, `update`, `delete`, et opérations sur les dossiers ;
  - modélisation : `content-type.create`, `update`, `delete` et `component.*` ;
  - utilisateurs et rôles : `user.*`, `role.*`, `permission.*` ;
  - jetons et webhooks : `api-token.*`, `transfer-token.*`, `webhook.*` ;
  - réglages et plugins : changements de réglages, `locale.*`, événements des fonctions XTR (stages, releases).
- **Stockage** dans une table dédiée, en écriture asynchrone avec une file bornée. Les logs sont insérables mais jamais modifiables via l'API.
- **Rétention** configurable en jours (`admin.auditLogs.retentionDays`, 90 par défaut, illimitée possible), avec une purge quotidienne par cron.
- **Consultation** :
  - liste paginée ;
  - filtres par action, par utilisateur et par période ;
  - recherche ;
  - détail avec le payload JSON lisible.
- **Export** CSV et JSON de la sélection filtrée.
- **Permissions** : `audit-logs.read`, `audit-logs.export`.
- **Masquage** : mots de passe, tokens et secrets toujours retirés des payloads.

## Hors périmètre
- Les actions des utilisateurs finaux sur l'API de contenu (hors admin) : elles relèvent des logs HTTP.
- L'envoi vers un SIEM externe : possible plus tard via un hook.

## Critères d'acceptation
- [ ] Chaque catégorie d'événement ci-dessus a un test qui déclenche l'action et vérifie l'entrée de log.
- [ ] Une mise à jour de document enregistre un diff limité aux champs modifiés.
- [ ] Aucun log ne contient de mot de passe, de token ni de secret (test avec des valeurs piégées).
- [ ] La purge supprime les entrées plus anciennes que la rétention, et uniquement celles-là.
- [ ] L'export CSV reproduit exactement les filtres de l'écran.
- [ ] Une panne d'écriture des logs ne fait pas échouer l'action métier : l'erreur est journalisée et comptée.
- [ ] Écran conforme aux conventions UI (`ADM-002`).

## Notes techniques
- S'abonner à l'event hub (`DOC-005`) et aux événements admin plutôt que d'instrumenter chaque handler.
- Index sur (date, action) et (user_id, date).

## Référence Strapi
- Documentation publique Strapi : « Audit Logs ».

## Dépendances
- `DOC-005`, `AUTH-001`, `AUTH-007`, `EXT-005`, `ADM-008`

## Fichiers
- `internal/plugins/auditlogs/{subscriber,store,purge,export,handlers}.go`
- `admin/src/pages/settings/audit-logs/`
