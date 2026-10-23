---
status: done
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
- Single types : lecture, création au premier enregistrement, mise à jour, publication, dépublication, suppression (`/content-manager/single-types/{uid}`), même formulaire. Testé (`TestAdminSingleType`) et vérifié dans Chrome.
- Composants (simples et répétables, imbriqués) et zones dynamiques : ajout, retrait, réordonnancement, choix du composant, document chargé en profondeur (`populate` récursif) et renvoyé entier avec les `id` des composants. Erreurs de validation avec leur chemin (`links.0.label`). Testé (`TestAdminComponents`, `TestAdminValidationDetails`) et vérifié dans Chrome sur le fixture `components`.
- Médias : champ simple ou multiple, sélection dans la médiathèque, réordonnancement, retrait (aussi dans les composants).
- Relations dans les composants (`?component=`), génération d'uid depuis le champ cible et vérification de disponibilité (`/content-manager/uid/*`). Testés (`TestAdminComponents`, `TestAdminUID`).
- Duplication (depuis la liste ou l'édition) : formulaire de création pré-rempli, sans les champs uniques ni les uid, et sans les `id` des composants pour que la copie crée les siens. Vérifié dans Chrome sur une page à composants imbriqués : l'original garde ses composants.
- Locales : sélecteur dans l'en-tête, création de la version d'une autre locale avec les champs non localisés repris de la locale par défaut (sinon l'enregistrement écraserait ces champs partagés). Testé (`TestAdminLocales`) et vérifié dans Chrome sur `i18n-dp`.
- Blocks : éditeur par blocs (paragraphe, titres 1 à 6, listes, citation, code avec langage, image de la médiathèque), réordonnables, mise en forme en ligne en Markdown léger (gras, italique, souligné, barré, code, lien) convertie vers et depuis les nœuds Strapi, avec échappement des caractères spéciaux (`admin/src/lib/blocks.ts`, tests aller-retour). Vérifié dans Chrome : le JSON enregistré est au format Strapi. Ce n'est pas un éditeur WYSIWYG.
- Permissions par champ : `/content-manager/models` donne `createFields` et `updateFields` (`null` sans restriction), l'EditView désactive les champs absents de la liste et le serveur ignore leurs valeurs (`TestAdminFieldLevelUpdate`). Vérifié dans Chrome sur le binaire : champs grisés et non modifiables.
- Brouillon incomplet : il s'enregistre, et l'échec de publication porte `details.errors`, que l'EditView affiche sous le champ fautif (`TestAdminIncompleteDraft`, vérifié dans Chrome). Ordre des relations : envoyé en `set`, relu dans le nouvel ordre par l'API REST (`TestAdminRelationReorder`). UID pris : `check-availability` (`TestAdminUID`), appelé par le champ avant l'enregistrement.
- « Remplir depuis une autre locale » : modale avec la locale source, copie des seuls champs localisés et modifiables, rien n'est enregistré avant « Enregistrer » (`admin/src/pages/content-manager/fill.ts`, testé, vérifié dans Chrome). La règle « localisé » est celle du serveur (option, relation ou uid) : un champ sans option est partagé, et le formulaire d'une nouvelle locale l'affiche avec sa valeur au lieu d'un champ vide (le serveur gardait déjà la valeur partagée).
- Aller-retour par type : le formulaire rend chaque type à l'identique (`roundtrip.test.ts`, fuseau Europe/Paris) et l'API admin relit ce qu'elle a écrit (`TestAdminAttributeRoundTrip`). Les dates sont affichées en heure locale et renvoyées au même instant, à la milliseconde (avant : décalage du fuseau à chaque enregistrement et secondes perdues). `timestamp`, lu en millisecondes comme dans Strapi, est éditable. Vérifié dans Chrome : enregistrer sans rien changer garde les valeurs.
- Composants et zones dynamiques : ajout, déplacement et retrait persistent avec les charges de l'EditView (ids conservés) ; `max` refuse l'enregistrement sans draft & publish et la publication avec, un brouillon au-delà s'enregistre, et l'erreur s'affiche sous la zone (`TestAdminComponentEditing`).
- 375 px (Chrome, iframe de 375 px sur le binaire, page avec composants et zone dynamique) : en-tête et actions empilés en pleine largeur, champs en une colonne, outils des éléments en icône avec `title`, aucun scroll horizontal.
- Reste, hors critères : relations morph, statut des cibles, éditeur blocks WYSIWYG et listes imbriquées, mise en page configurée (`ADM-005`).

## Hors périmètre
- Preview (`ADM-009`), content history (`XTR-003`), review workflows (`XTR-004`) : ce sont des panneaux latéraux ajoutés par ces tickets.

## Critères d'acceptation
- [x] Chaque type d'attribut a un test d'aller-retour : saisie, enregistrement, rechargement, valeur identique (y compris biginteger, decimal et blocks).
- [x] Un brouillon incomplet (champ requis vide) s'enregistre. La publication échoue avec l'erreur sur le bon champ.
- [x] Réordonner trois relations envoie des positions correctes, et l'ordre persiste (vérifié via l'API REST).
- [x] Composants répétables et zones dynamiques : l'ajout, le déplacement et la suppression persistent. `min` et `max` bloquent l'enregistrement à la publication.
- [x] Un UID déjà pris est signalé avant l'enregistrement.
- [x] « Remplir depuis une autre locale » copie uniquement les champs localisés.
- [x] Un utilisateur avec la permission `update` limitée aux champs `title` et `content` voit les autres champs en lecture seule, et le serveur rejette toute tentative de les modifier.
- [x] À 375 px, tous les champs, composants et actions sont empilés, avec des boutons en pleine largeur.

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
