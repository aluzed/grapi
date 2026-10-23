---
status: done
---

# ADM-006 : Content-Type Builder (écriture des schémas, rechargement à chaud, migrations de renommage)

> Option retenue : B (admin maison), voir `docs/decisions/D1-admin.md`. Conventions UI : voir `ADM-002`.

## Contexte
Le Content-Type Builder (CTB) de Strapi crée, modifie et supprime des content types, des composants, des catégories de composants et des champs. Côté serveur, il écrit les fichiers `schema.json` dans `src/` et génère des migrations de renommage. Il redémarre ensuite le process, et il n'est actif qu'en mode develop. Strapi 5 enregistre par lot (`POST /update-schema`) et l'admin interroge ensuite `update-schema-status`. Un binaire Go ne redémarre pas comme un process Node : grapi recharge le schéma dans le process.

## Objectif
Modéliser le contenu depuis l'admin, avec écriture des fichiers de schéma au format Strapi, synchro de la base et rechargement à chaud, sans redémarrer le binaire.

## Périmètre
- **Écrans** :
  - liste des content types (collection et single), des composants par catégorie ;
  - création d'un type (nom d'affichage, singularName et pluralName dérivés et modifiables, kind, draft & publish, i18n) ;
  - ajout et édition de champs par type d'attribut, avec réglages de base (nom, options du type) et avancés (required, unique, private, default, min, max, minLength, maxLength, regex, `allowedTypes` pour les médias, `targetField` pour les UID, enum pour les énumérations) ;
  - relations avec le schéma visuel des 10 types (oneWay, oneToOne, oneToMany, manyToOne, manyToMany, manyWay, et morph en lecture seule) et le choix du champ inverse ;
  - composants (création, catégorie, icône) ;
  - zones dynamiques (liste des composants autorisés) ;
  - custom fields enregistrés (`SCH-006`).
- **Enregistrement par lot** :
  - toutes les modifications en attente partent ensemble ;
  - le serveur valide (`SCH-003`, noms réservés), écrit les fichiers, puis recharge les registres, synchronise la base (`DB-003`) et reconstruit les routes ;
  - pendant ce temps, l'admin affiche l'état via un endpoint de statut.
- **Renommage de champ ou de type** : génération d'une migration qui renomme les colonnes et tables au lieu de supprimer puis recréer (pas de perte de données), et mise à jour de la configuration de vue (`ADM-005`).
- **Suppression** d'un type, d'un champ ou d'un composant :
  - modale de confirmation qui liste les impacts (relations inverses, usages du composant) ;
  - suppression effective des tables seulement si `forceMigration` l'autorise, sinon conservation des données orphelines (même règle que Strapi).
- **Disponibilité** : actif uniquement en mode develop (`grapi develop`). En production, les endpoints d'écriture renvoient 403, et le menu est masqué ou passe en lecture seule.
- **Endpoint des noms réservés** pour la validation côté client.

## Hors périmètre
- Édition des schémas des plugins hors extensions (`src/extensions`).
- Le rechargement de code Go utilisateur : en mode framework, un changement de code Go demande toujours une recompilation.

## Critères d'acceptation
- [x] Créer un content type avec 1 champ de chaque type, des relations et un composant :
  - les `schema.json` écrits sont valides au format Strapi, et Strapi 5.59 les charge sans erreur (vérifié par le banc `SETUP-002`) ;
  - grapi expose les routes REST sans redémarrage.
- [x] Renommer un champ rempli conserve toutes les valeurs (vérifié sur Postgres et SQLite).
- [x] Un enregistrement par lot invalide (relation vers un type inexistant, nom réservé) est rejeté entièrement, sans fichier écrit ni changement en base.
- [x] Pendant un rechargement, les requêtes en cours se terminent, et les nouvelles attendent le nouveau schéma ou reçoivent 503 avec `Retry-After`. Aucune ne voit un état mixte.
- [x] En mode production, `POST` d'écriture du CTB renvoie 403.
- [x] Toute suppression passe par la modale de confirmation qui liste les impacts.

## Avancement
- Content API en lecture seule du plugin (`internal/httpapi/ctb.go`) : `GET /api/content-type-builder/content-types[/:uid]` (`kind`, types invisibles en 404) et `GET /api/content-type-builder/components[/:uid]`, au format de `formatContentType` et `formatComponent`. Corpus `components_ctb`, SQLite et Postgres : identique à Strapi. Les deux listes complètes sont en skip au banc : celle des content types contient les types Enterprise (clean-room), celle des composants suit chez Strapi l'ordre de lecture des fichiers.
- API admin (`docs/admin-api.md`, Content-Type Builder) : lecture des schémas depuis les fichiers, lot `update-schema` validé sur une copie de `src/` puis écrit atomiquement, relations bidirectionnelles écrites des deux côtés, nettoyage des références à la suppression, statut par hash du schéma. Écriture réservée à `grapi develop`, qui recharge l'application dans le process par bascule atomique (pas de redémarrage du binaire). Testé (`TestAdminContentTypeBuilder`, `TestAdminContentTypeBuilderProduction`).
- Interface (`admin/src/pages/ctb/`) : liste des types de collection, types uniques et composants, détail des champs, création et édition d'un type (nom affiché, ID d'API dérivé, draft & publish, i18n) ou d'un composant (catégorie), ajout et édition de champ (21 types, réglages de base et avancés, relations avec champ inverse, composants, zones dynamiques), suppressions confirmées, brouillon local enregistré par lot puis attente du rechargement par le hash du schéma. Lecture seule hors `grapi develop`. Vérifié dans Chrome sous `grapi develop` : type créé avec une relation bidirectionnelle, rechargé et servi sans redémarrer.
- Vérifié le 2026-10-23 : l'image Strapi 5.59 du banc démarre sur un `src/` modifié par le CTB de grapi (type Recette avec relation manyToMany et champ inverse sur Tag) et sert `/api/recettes`. Vérification manuelle, pas encore un cas automatique du banc.
- Renommage d'un champ scalaire (`renamedFrom`) : migration SQL `ALTER TABLE … RENAME COLUMN` écrite dans `database/migrations`, jouée au rechargement avant la synchronisation, configuration de vue déplacée (métadonnées, colonnes, mise en page, champ principal, tri), uid qui ciblaient l'ancien nom mis à jour. Relations, médias et composants ne se renomment pas (tables de jointure) : refus explicite. Testé (`TestAdminContentTypeBuilderRename` : la valeur survit, SQLite et Postgres via `make ci`).
- Les noms réservés du builder ne s'appliquent qu'aux nouveaux attributs : un type existant qui en porte déjà un (`meta`) reste modifiable.
- Suppressions : la modale liste ce qui change ailleurs (relations vers un type supprimé, champs composant et zones dynamiques d'un composant supprimé, autre côté d'une relation bidirectionnelle), calculé comme le serveur l'applique (`admin/src/pages/ctb/model.ts`, testé). Retirer un côté d'une relation bidirectionnelle retire l'autre, comme le builder de Strapi ; avant, le lot était refusé (`inverse-missing`) (`TestAdminContentTypeBuilderDropsInverse`). Vérifié dans Chrome sous `grapi develop` jusqu'à l'enregistrement et au rechargement.
- Rechargement de `grapi develop` : une barrière (`reload_gate.go`) laisse finir les requêtes en cours avant la synchronisation du schéma, fait attendre les nouvelles (10 s au plus, puis 503 avec `Retry-After`) et ne ferme l'ancienne version qu'une fois vide. Sous charge (8 clients en continu, ajout d'un champ, `ALTER TABLE`), 0 erreur sur ~11 000 réponses ; sans la barrière, des 500 à chaque essai (`TestDevelopReloadUnderLoad`, `TestReloadGate`, avec `-race`).
- Banc automatique : `TestContentTypeBuilderFixture` crée sous `grapi develop`, par le builder, un type avec un champ de chaque type (21, dont deux médias), une relation simple et une bidirectionnelle, un composant répétable et une zone dynamique ; les routes REST répondent après chaque lot sans redémarrage, et les fichiers écrits forment la fixture `ctb`. Strapi 5.59 la charge et répond au corpus `ctb` (6 cas, création et relecture de toutes les valeurs) ; grapi lui est identique sur SQLite et Postgres, contrôles direct et inverse compris.
- Un nouveau type reçoit le controller, la route et le service par défaut que génère le builder de Strapi (JS, ou TS avec un `tsconfig.json`), sans écraser un fichier existant : un projet passé par le builder de grapi est servi par Strapi tel quel (`TestAdminContentTypeBuilderCoreFilesTS`).
- Reste, hors critères : les custom fields, le schéma visuel des relations, les icônes de composant.

## Notes techniques
- Le rechargement à chaud est la vraie difficulté. Il faut :
  - construire un nouveau « monde » (registres, métadonnées DB, routeur) à côté de l'ancien ;
  - synchroniser la base dans une transaction quand le dialecte le permet (Postgres : DDL transactionnel ; SQLite : oui ; MySQL : non) ;
  - basculer un pointeur atomique.
- Les fichiers sont écrits de façon atomique (fichier temporaire puis rename). L'ordre des clés JSON est stable, pour des diffs git propres.
- **Si l'option A est retenue** : reproduire `/content-type-builder/update-schema`, `/update-schema-status`, `/schema`, `/reserved-names` et les CRUD content types, components et categories selon `content-type-builder/shared/contracts/`.

## Référence Strapi
- `core/content-type-builder/server/src/routes/admin.ts`
- `core/content-type-builder/server/src/services/schema-builder/`, `core/content-type-builder/server/src/services/migration-builder/`, `core/content-type-builder/server/src/services/schema.ts`
- `core/content-type-builder/server/src/middlewares/` (is-development-mode)
- `core/content-type-builder/admin/src/`

## Dépendances
- `ADM-002`, `ADM-005`, `SCH-001` à `SCH-006`, `DB-003`, `DB-004`, `API-002`, `CLI-001` (mode develop)

## Fichiers
- `admin/src/pages/content-type-builder/`
- `internal/plugins/ctb/{handlers,writer,validate,rename}.go`
- `internal/app/reload.go`
