---
status: in-progress
---

# ADM-004 : Content Manager : vue édition (champs, composants, zones dynamiques, relations, D&P, i18n)

> Option retenue : B (admin maison), voir `docs/decisions/D1-admin.md`. Conventions UI : voir `ADM-002`.

## Contexte
La vue édition est l'écran le plus riche de Strapi. Elle propose un champ de saisie par type d'attribut, des composants répétables, des zones dynamiques, des relations avec recherche et réordonnancement, les actions de draft & publish, le changement de locale, « remplir depuis une autre locale », la génération des champs UID et la duplication.

## Objectif
Pouvoir créer et modifier n'importe quel document (collection type ou single type) depuis un formulaire généré à partir du schéma et de la configuration de vue.

## Périmètre
- **Un champ de saisie par type d'attribut** :
  - texte : string, text, email, password (masqué, jamais pré-rempli) ;
  - richtext (Markdown avec aperçu) et blocks (éditeur de blocs au format JSON Strapi : paragraphes, titres, listes, citations, code, images, liens) ;
  - nombres : integer, biginteger (traité comme une chaîne), float, decimal ;
  - dates : date, time, datetime ;
  - boolean (interrupteur), enumeration (select), json (éditeur avec validation) ;
  - uid (bouton « Générer » depuis `targetField`, vérification de disponibilité en direct) ;
  - media : sélecteur ouvrant la Media Library (`ADM-007`), simple ou multiple, avec filtre `allowedTypes` ;
  - custom fields : composant fourni par le plugin (`ADM-010`).
- **Composants** : simples ou répétables, avec ajout, suppression, réordonnancement par glisser-déposer, repli et dépli, et respect de `min` et `max`.
- **Zones dynamiques** : choix du composant à ajouter, ordre, suppression, `min` et `max`.
- **Relations** :
  - recherche paginée sur le type cible ;
  - affichage du `mainField` et du statut ;
  - ajout, retrait et réordonnancement, envoyés en `connect` / `disconnect` / `position` ;
  - ouverture de l'entrée liée ;
  - pour les relations to-many, chargement paginé des relations existantes.
- **Validation côté client** : schéma Zod dérivé du schéma du content type. Les règles sont souples en brouillon (comme côté serveur, `DOC-003`) et strictes à la publication. Les erreurs serveur sont mappées sur les champs.
- **Actions** :
  - Enregistrer (brouillon), Publier, Dépublier, Annuler les modifications (discardDraft), Dupliquer, Supprimer ;
  - modale de confirmation pour supprimer, dépublier et annuler ;
  - avertissement à la publication si des relations sont encore en brouillon.
- **Statut** : badge de statut, dates de création et de modification, auteur et dernier éditeur (`createdBy` / `updatedBy`).
- **i18n** : sélecteur de locale, création d'une locale manquante, « Remplir depuis une autre locale » (copie des champs localisés), champs non localisés signalés et synchronisés.
- **Garde-fous** : avertissement à la sortie si des modifications ne sont pas enregistrées.
- **Single types** : même formulaire, sans liste.
- **Permissions** : champs non lisibles masqués, champs non modifiables en lecture seule, actions masquées selon `create`, `update`, `publish` et `delete`, permissions par locale.

## Avancement
- API : lecture d'un document, création (`createdBy`, `updatedBy`) et mise à jour, champs filtrés par les permissions (`docs/admin-api.md`).
- Formulaire générique dans l'ordre du schéma pour les champs simples (texte, richtext et JSON en zone de texte, nombres, booléen, énumération, dates, uid sans génération, mot de passe jamais pré-rempli), valeurs par défaut, erreurs de validation du serveur par champ, enregistrer, publier, dépublier, supprimer avec confirmation, badge de statut.
- Relations (hors morph) : recherche paginée des cibles libellées par leur `mainField` (`GET /content-manager/relations/{uid}/{field}`), ajout, retrait, réordonnancement, envoi en `set` dans l'ordre affiché. Testé de bout en bout (`TestAdminRelations`) et dans Chrome sur le fixture `relations`.
- Reste : relations morph, statut des cibles, médias, composants, zones dynamiques, blocks, génération d'uid, locales, single types, mise en page configurée (`ADM-005`).

## Hors périmètre
- Preview (`ADM-009`), content history (`XTR-003`), review workflows (`XTR-004`) : ce sont des panneaux latéraux ajoutés par ces tickets.

## Critères d'acceptation
- [ ] Chaque type d'attribut a un test d'aller-retour : saisie, enregistrement, rechargement, valeur identique (y compris biginteger, decimal et blocks).
- [ ] Un brouillon incomplet (champ requis vide) s'enregistre. La publication échoue avec l'erreur sur le bon champ.
- [ ] Réordonner trois relations envoie des positions correctes, et l'ordre persiste (vérifié via l'API REST).
- [ ] Composants répétables et zones dynamiques : l'ajout, le déplacement et la suppression persistent. `min` et `max` bloquent l'enregistrement à la publication.
- [ ] Un UID déjà pris est signalé avant l'enregistrement.
- [ ] « Remplir depuis une autre locale » copie uniquement les champs localisés.
- [ ] Un utilisateur avec la permission `update` limitée aux champs `title` et `content` voit les autres champs en lecture seule, et le serveur rejette toute tentative de les modifier.
- [ ] À 375 px, tous les champs, composants et actions sont empilés, avec des boutons en pleine largeur.

## Notes techniques
- Endpoints admin dédiés : get/create/update/publish/unpublish/discard/clone, recherche de relations, génération et disponibilité des UID. Ils réutilisent le Document Service, la validation (`DOC-003`) et le RBAC (`AUTH-001`).
- Les métadonnées de formulaire (libellé, description, placeholder, visible, editable, taille et ordre des champs) viennent de `ADM-005`.
- **Si l'option A est retenue** : contrats `content-manager/shared/contracts/{collection-types,single-types,relations,uid}.ts` à reproduire fil à fil.

## Référence Strapi
- `core/content-manager/server/src/controllers/{collection-types,single-types,relations,uid}.ts`
- `core/content-manager/admin/src/pages/EditView/`
- `core/content-manager/admin/src/components/` (champs, relations, zones dynamiques)
- `plugins/i18n/server/src/controllers/` (fill-from-locale, non-localized fields)

## Dépendances
- `ADM-002`, `ADM-005`, `ADM-007`, `DOC-001` à `DOC-007`, `DB-006`, `DB-007`, `AUTH-001`, `PLG-001`, `SCH-006`

## Fichiers
- `admin/src/pages/content-manager/edit/`, `admin/src/features/fields/`, `admin/src/features/relations/`, `admin/src/features/components/`, `admin/src/features/blocks-editor/`
- `internal/httpapi/admin/contentmanager/{documents,relations,uid}.go`
