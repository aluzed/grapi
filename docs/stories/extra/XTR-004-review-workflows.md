# XTR-004 : Review workflows

> **Clean-room** : fonction sous licence Enterprise chez Strapi. Spécification rédigée uniquement à partir de la documentation publique de Strapi et du comportement observable. Il est **interdit d'ouvrir** les dossiers `ee/`, les paquets `core/review-workflows` et `core/content-releases`, ainsi que `core/content-manager/{admin,server}/src/{history,preview}` (voir `SETUP-003`). Dans grapi, la fonction est sous MIT, sans clé de licence ni limite (nombre de workflows, de releases, durée de rétention...).

## Contexte
Dans Strapi Enterprise, les review workflows ajoutent un cycle de relecture aux documents : des workflows nommés, chacun composé d'étapes (stages) ordonnées et colorées, et associés à un ou plusieurs content types. Chaque document porte une étape courante et un assigné. Les permissions précisent quels rôles peuvent faire passer un document vers chaque étape. Une étape peut être requise pour publier. Les changements d'étape déclenchent des webhooks.

## Objectif
Offrir des workflows de relecture configurables, avec étapes, assignés, permissions par étape et webhooks, sans limite de nombre.

## Périmètre
- **Workflows** : CRUD (nom, content types associés, étapes ordonnées avec nom et couleur, étape requise pour publier facultative). Un content type appartient à au plus un workflow.
- **Champs ajoutés** aux content types concernés : étape courante (`strapi_stage`) et assigné (`strapi_assignee`, utilisateur admin). Ils sont exposés dans l'admin et filtrables dans la liste. Ils ne sont pas exposés dans l'API de contenu publique par défaut.
- **Étape initiale** : à la création d'un document, il est placé sur la première étape.
- **Changement d'étape** depuis la vue édition et en masse depuis la liste, autorisé seulement si un rôle de l'utilisateur a la permission de transition vers l'étape cible.
- **Assignation** à un utilisateur admin, avec un filtre « Assigné à moi » dans la liste.
- **Publication** bloquée tant que le document n'a pas atteint l'étape requise (si elle est configurée).
- **Webhooks et événements** : `review-workflows.updateEntryStage` (avec l'étape source et l'étape cible), ainsi que les événements d'assignation. Ils sont proposés dans l'écran des webhooks.
- **Suppression d'une étape** : les documents qui y sont sont déplacés vers une étape choisie (modale de confirmation). La suppression d'un workflow retire les champs d'étape après confirmation.
- **Widget de page d'accueil** « Mes assignations ».

## Hors périmètre
- Les notifications par e-mail sur assignation (amélioration possible via webhook ou hook).

## Critères d'acceptation
- [ ] Un workflow à 4 étapes sur `api::post.post` : un nouveau post démarre à l'étape 1.
- [ ] Un rôle sans permission vers l'étape 3 reçoit 403 en tentant cette transition, et ne voit pas l'étape dans le sélecteur.
- [ ] Avec l'étape requise « Validé », la publication échoue avant et réussit après.
- [ ] Le webhook reçoit l'étape source et l'étape cible à chaque changement.
- [ ] Supprimer une étape non vide déplace ses documents vers l'étape choisie.
- [ ] Le filtre « Assigné à moi » renvoie exactement les documents assignés à l'utilisateur courant.
- [ ] Écrans conformes aux conventions UI (`ADM-002`).

## Notes techniques
- L'étape et l'assigné sont stockés en relation sur la ligne brouillon. Il faut décider (et documenter) s'ils suivent la publication.
- Les permissions de transition s'intègrent au moteur RBAC (`AUTH-001`) comme actions sur le sujet « stage ».

## Référence Strapi
- Documentation publique Strapi : « Review Workflows ».

## Dépendances
- `DOC-002`, `DOC-004`, `DOC-005`, `AUTH-001`, `AUTH-007`, `EXT-006`, `ADM-003`, `ADM-004`, `ADM-008`

## Fichiers
- `internal/plugins/reviewworkflows/{workflows,stages,transitions,assignee,events,handlers}.go`
- `admin/src/pages/settings/review-workflows/`, `admin/src/features/review-workflows/`
