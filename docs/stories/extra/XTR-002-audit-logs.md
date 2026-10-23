---
status: in-progress
---

# XTR-002 : Audit logs

> Clean-room (`SETUP-003`, décision D6) : ce ticket se spécifie uniquement à partir de la documentation publique de Strapi et du comportement observable. Aucun fichier source Strapi sous licence Enterprise ne doit être ouvert, y compris pour en lire le nom.

## Contexte
Strapi enregistre les actions d'écriture faites dans l'admin dans un historique filtrable : action, date, utilisateur, détail (payload). La documentation publique liste les événements, l'origine (`admin-panel`, `mcp`, `scheduler`), les règles de masquage, une rétention par défaut de 90 jours, des filtres et un export CSV. Les lectures ne sont pas journalisées. grapi n'impose aucune limite de licence sur la rétention ni sur l'export.

## Objectif
Un journal d'audit immuable des actions d'administration, consultable, filtrable et exportable, avec rétention configurable.

## Périmètre
- **Événements** (noms identiques à la doc publique) :
  - `admin.auth.success|error|autoRegistration`, `admin.logout` ;
  - `content-type.create|update|delete`, `component.create|update|delete` ;
  - `entry.create|update|delete|publish|unpublish`, `entry.assignee.update`, `review-workflows.updateEntryStage` ;
  - `media.create|update|delete`, `media-folder.create|update|delete` ;
  - `locale.create|update|delete`, `locale.default.update` ;
  - `role.create|update|delete`, `permission.create|update|delete` ;
  - `user.create|update|delete`, `user.password.update`, `user.password-reset.create|confirm`, `user.invite.accept` ;
  - `token.create|update|delete|regenerate` (API, admin, transfer) ;
  - `webhook.create|update|delete`, `workflow.create|update|delete` ;
  - `release.create|update|delete|trigger`, `release-action.add|update|remove`, `release-settings.update` ;
  - `audit-log.export`.
- **Origines** : `admin-panel`, `admin-token`, `mcp` (`PLG-007`), `cli` (`CLI-003`), `scheduler` (releases planifiées, pas d'utilisateur).
- **Enregistrement** : `id`, `action`, `date` (UTC), `user` (id, nom affiché, e-mail, figés au moment de l'action), `origin`, `payload` JSON : IP, user agent, méthode et route, `uid` et `documentId` et `locale` le cas échéant, corps de requête filtré, et pour les `*.update` uniquement les champs modifiés (avant / après).
- **Masquage** : jamais de mot de passe, hash, jeton de reset ou d'enregistrement, clé de token ; URL de webhook réduite à schéma + hôte, valeurs d'en-têtes omises ; champs `private` et `password` des content types omis.
- **Immuabilité** : aucune route de modification ni de suppression unitaire. Seule la purge de rétention supprime.
- **Rétention** : `admin.auditLogs.retentionDays` (défaut 90, `0` = illimitée), purge quotidienne par cron (`EXT-005`). `admin.auditLogs.enabled` (défaut `true`).
- **API admin** : `GET /admin/api/v1/audit-logs` (pagination, tri par date décroissante), `GET .../audit-logs/:id`, `GET .../audit-logs/export.csv`.
- **Filtres** : action (multiple), utilisateur (multiple), plage de dates avec heure, origine, `uid` de content type, `documentId`.
- **Export CSV** : lignes filtrées, streamé (pas de chargement en mémoire), colonnes `date, action, origin, user_id, user_email, uid, document_id, locale, payload_json`, plafond `admin.auditLogs.exportMaxRows` (défaut 1 000 000). L'export journalise lui-même un `audit-log.export` avec les filtres utilisés.
- **Permissions** : `admin::audit-logs.read` (liste et détail), `admin::audit-logs.export` (en plus de read).
- **Écran** Settings > Audit logs : table (Action, Date, Utilisateur, Origine, Détails), filtres, modale de détail JSON, bouton « Exporter en CSV ».

## Hors périmètre
- Les écritures du Content API par des utilisateurs finaux ou des API tokens (volume, déjà couvert par les logs `EXT-007`) : option ultérieure `auditLogs.contentApi: true`.
- L'envoi vers un SIEM (syslog, OTLP) : à proposer séparément, l'interface `Sink` le permet.
- Signature ou chaînage cryptographique des entrées.

## Critères d'acceptation
- [ ] Chaque événement du périmètre produit exactement une entrée avec la bonne action et l'origine attendue (test table-driven).
- [x] Une lecture (`GET` admin) ne produit aucune entrée.
- [x] `entry.update` ne contient que les champs modifiés, avec valeurs avant et après.
- [x] Création d'un webhook avec en-tête `Authorization` : le payload ne contient ni la valeur de l'en-tête ni le chemin de l'URL.
- [x] Changement de mot de passe : aucune trace du mot de passe ni de son hash dans la base d'audit (grep sur un dump).
- [x] Publication planifiée d'une release : entrée `release.trigger` avec `origin: scheduler` et sans utilisateur.
- [x] `retentionDays: 1` : une entrée datée de J-2 est purgée au passage du cron.
- [x] Filtres combinés (action + utilisateur + plage) renvoient le même ensemble en liste et en CSV.
- [ ] Export de 200 000 lignes sous 64 Mo de RSS supplémentaire ; l'export apparaît dans le CSV suivant.
- [x] Sans `export`, la route CSV renvoie 403 et le bouton est masqué. Écran conforme aux conventions de `ADM-002`.

## Avancement
- Table `grapi_audit_logs` (hors schéma Strapi), une ligne immuable par écriture de l'admin, utilisateur figé (id, nom, e-mail), origine `admin-panel`, IP, user agent, méthode et route. Événements : connexion réussie ou échouée, déconnexion, création, mise à jour (noms des champs modifiés) et suppression d'admin, changement et réinitialisation de mot de passe, acceptation d'invitation, rôles et permissions, entrées (création, mise à jour, suppression, publication, dépublication, restauration d'historique), médias et dossiers, locales et locale par défaut, tokens, webhooks (URL réduite au schéma et à l'hôte, en-têtes omis), content types et composants du CTB, export du journal.
- API : liste paginée filtrable (actions, utilisateur, plage de dates), détail, export CSV en flux qui s'audite lui-même. Rétention `admin.auditLogs.retentionDays` (90 jours par défaut), `admin.auditLogs.enabled`. Permissions `admin::audit-logs.read` et `export`.
- Écran Paramètres > Journaux d'audit : table, filtres, détail JSON, export.
- Testé (`TestAdminAuditLogs` : événements attendus, aucune lecture, webhook sans secret ni chemin, aucun mot de passe ni hash dans tout le journal, export filtré).
- `entry.update` : champs modifiés avec valeurs avant et après (`changes: [{path, before, after}]`, même comparaison que l'historique), sans champs privés ni mots de passe. Releases : `release.*`, `release-action.*`, `release-settings.update`, et `release.trigger` du planificateur (origine `scheduler`, sans utilisateur, statut `done` ou `failed`). Origine `cli` pour `admin:create-user` et `admin:reset-user-password`.
- Colonnes `uid`, `document_id`, `locale` (ajoutées au démarrage sur une table existante), filtres `origin`, `uid`, `documentId`, `locale` ; filtre d'origine dans l'écran. Plafond `admin.auditLogs.exportMaxRows` (1 000 000). Purge par le planificateur du serveur (au démarrage puis au plus une fois par heure).
- Testé : `TestAdminAuditDetails` (avant/après, champ privé absent, purge, liste et CSV identiques, export sans permission refusé), `TestAdminReleases`, `TestAdminCommands`.
- Reste : le test table-driven de chaque événement, la mesure mémoire d'un export de 200 000 lignes, l'origine `admin-token` (l'API admin n'accepte aujourd'hui que des sessions).

## Notes techniques
- Abonnement à l'event hub (`DOC-005`) et aux hooks des services admin ; l'acteur et l'origine viennent du contexte de requête (`API-001`).
- Écriture après commit de l'action, dans une transaction séparée ; un échec d'écriture d'audit est journalisé en `error` mais ne casse pas l'action (choix documenté, réversible via `auditLogs.strict: true` qui fait échouer l'action).
- Table `grapi_audit_logs` (index sur `date`, `action`, `user_id`, `(uid, document_id)`). Pas de compatibilité avec la table d'une instance Strapi Enterprise.
- Sur Postgres, le CSV passe par un curseur serveur ; sur SQLite, pagination par clé (`id`).

## Référence
- Audit Logs : https://docs.strapi.io/cms/features/audit-logs
- Spécification clean-room : aucune source Strapi sous licence Enterprise consultée.

## Dépendances
- `DOC-005`, `AUTH-001`, `AUTH-007`, `AUTH-008`, `EXT-001`, `EXT-005`, `ADM-002`, `ADM-008`, `SETUP-003`

## Fichiers
- `internal/extra/auditlog/` (`events.go`, `recorder.go`, `redact.go`, `retention.go`, `export.go`, `handlers.go`)
- `admin/src/pages/settings/audit-logs/`
