---
status: todo
---

# XTR-004 : Review workflows

> Clean-room (`SETUP-003`, décision D6) : ce ticket se spécifie uniquement à partir de la documentation publique de Strapi et du comportement observable. Aucun fichier source Strapi sous licence Enterprise ne doit être ouvert, y compris pour en lire le nom.

## Contexte
Strapi propose des circuits de relecture : un workflow, associé à un ou plusieurs content types, est une suite ordonnée d'étapes (nom, couleur). Chaque entrée a une étape courante et un assigné. Des permissions par étape disent quels rôles peuvent faire sortir une entrée d'une étape ou l'y amener. Un workflow peut exiger une étape précise avant publication. L'événement webhook `review-workflows.updateEntryStage` est public. Strapi limite le nombre de workflows et d'étapes ; grapi n'a aucune limite.

## Objectif
Des workflows de relecture configurables, appliqués par le serveur (pas seulement par l'UI), intégrés au Content Manager, aux releases, à l'audit et aux webhooks.

## Périmètre
- **Workflow** : nom, content types associés (un content type appartient à au plus un workflow), étapes ordonnées, `stageRequiredToPublish` (id d'étape ou vide = publication libre).
- **Étape** : nom (unique dans le workflow), couleur (palette fixe de 12 couleurs accessibles, contraste AA avec le texte du badge), position, `permissions` : rôles autorisés à faire **sortir** de l'étape et rôles autorisés à faire **entrer** dans l'étape, avec « Appliquer à toutes les étapes ». Super Admin toujours autorisé.
- **Workflow par défaut** créé au premier démarrage, sans content type : To do, Ready to review, In progress, Reviewed.
- **Règles de gestion** :
  - associer un content type à un workflow place toutes ses entrées existantes à la première étape ;
  - une entrée créée arrive à la première étape ; la duplication remet à la première étape ;
  - supprimer une étape déplace ses entrées vers l'étape précédente (ou la suivante si c'était la première) ; la dernière étape d'un workflow et le dernier workflow ne sont pas supprimables ;
  - changer un content type de workflow remet ses entrées à la première étape du nouveau, après confirmation ;
  - duplication d'une étape : copie complète, permissions comprises.
- **Étape et assigné appartiennent au brouillon uniquement** : stockés par `(uid, documentId, locale)`, ils décrivent le travail en cours. La version publiée ne porte ni étape ni assigné, et le Content API n'expose jamais ces champs. Justification : la publication insère une copie du brouillon avec un nouvel `id` (`DOC-002`) ; attacher l'étape à la ligne publiée créerait un état dupliqué, figé et sans signification (la relecture porte sur la prochaine version, pas sur celle en ligne). Après publication, l'étape du brouillon est conservée telle quelle.
- **Publication** : si `stageRequiredToPublish` est défini, publier exige que l'entrée soit **exactement** à cette étape (une étape ultérieure est refusée), erreur `ValidationError` explicite. Contrôle dans un middleware du Document Service, donc valable pour l'admin, les releases (`XTR-005`, entrée « pas prête ») et le Content API ; contournable en Go par une option de contexte explicite.
- **Assigné** : un admin actif, ou personne. Changement réservé aux utilisateurs ayant `update` sur l'entrée.
- **Content Manager** (`ADM-003`, `ADM-004`) : colonnes Étape et Assigné, filtres et tri par étape et par assigné ; dans l'édition, sélecteurs d'étape (étapes non autorisées désactivées, avec `title` expliquant pourquoi) et d'assigné.
- **Webhook** `review-workflows.updateEntryStage` (`EXT-006`) : `{event, createdAt, model, uid, entity: {id}, workflow: {id, stages: {from: {id, name}, to: {id, name}}}}`.
- **Audit** (`XTR-002`) : `workflow.*`, `review-workflows.updateEntryStage`, `entry.assignee.update`.
- **Permissions** : `admin::review-workflows.read` et `admin::review-workflows.update` (Settings > Review workflows), écrans conformes à `ADM-002`.

## Hors périmètre
- Commentaires ou notifications e-mail aux assignés : amélioration possible, branchable sur l'événement.
- Étapes conditionnelles ou parallèles.
- Workflows sur des content types sans Draft & Publish : refusé à l'association.

## Critères d'acceptation
- [ ] Associer `api::post.post` à un workflow place les posts existants à la première étape, par locale.
- [ ] Un rôle absent des rôles « entrer » d'une étape reçoit 403 en tentant d'y déplacer une entrée, via l'API admin.
- [ ] Avec étape requise « Reviewed », publier depuis « In progress » ou depuis une étape postérieure échoue, depuis « Reviewed » réussit.
- [ ] La réponse REST d'un post publié ou brouillon ne contient aucun champ d'étape ni d'assigné.
- [ ] Publier ne modifie pas l'étape du brouillon ; republier après modification reste soumis à l'étape requise.
- [ ] Supprimer l'étape 2 déplace ses entrées en étape 1 ; supprimer l'unique étape est refusé.
- [ ] Le webhook `review-workflows.updateEntryStage` reçoit le payload du périmètre (récepteur de test).
- [ ] 300 workflows et 300 étapes se créent sans erreur (aucune limite).
- [ ] Le filtre par assigné de la vue liste renvoie les mêmes entrées que l'API.

## Notes techniques
- Tables `grapi_workflows`, `grapi_workflow_stages`, `grapi_workflow_stage_permissions`, et `grapi_review_states` (`uid`, `document_id`, `locale`, `stage_id`, `assignee_id`, `updated_at`), unique sur `(uid, document_id, locale)`. Pas de colonne ajoutée aux tables des content types : aucun recâblage à la publication, et les tables utilisateur restent compatibles Strapi (`DB-011`).
- Filtre et tri par étape via jointure dans le query builder (`DB-005`), exposés seulement à l'API admin.
- Pas de compatibilité avec le stockage d'une instance Strapi Enterprise.

## Référence
- Review Workflows : https://docs.strapi.io/cms/features/review-workflows
- Webhooks : https://docs.strapi.io/cms/backend-customization/webhooks
- Spécification clean-room : aucune source Strapi sous licence Enterprise consultée.

## Dépendances
- `DOC-002`, `DOC-004`, `DOC-005`, `DB-005`, `AUTH-001`, `AUTH-007`, `EXT-006`, `ADM-003`, `ADM-004`, `ADM-008`, `XTR-002`, `SETUP-003`

## Fichiers
- `internal/extra/reviewworkflows/` (`model.go`, `stages.go`, `assign.go`, `publish_guard.go`, `webhook.go`, `handlers.go`)
- `admin/src/pages/settings/review-workflows/`, `admin/src/pages/content-manager/review/`
