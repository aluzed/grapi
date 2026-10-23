---
status: done
---

# XTR-003 : Content history

> Clean-room (`SETUP-003`, décision D6) : ce ticket se spécifie uniquement à partir de la documentation publique de Strapi et du comportement observable. Aucun fichier source Strapi sous licence Enterprise ne doit être ouvert, y compris pour en lire le nom.

## Contexte
Strapi garde des versions des documents modifiés depuis le Content Manager. La documentation publique décrit : une liste de versions (date, auteur, statut Draft, Modified ou Published), l'affichage d'une version, une section « Unknown fields » quand le schéma a changé, une restauration qui écrase le brouillon et passe le document en Modified, une purge quotidienne selon une rétention. Les écritures via REST, GraphQL, Document Service, lifecycles, cron et import ne créent pas de version. grapi n'impose aucune limite de rétention.

## Objectif
Historique de versions par document et par locale, avec comparaison et restauration, robuste aux évolutions du schéma.

## Périmètre
- **Création d'une version** à chaque enregistrement et à chaque publication depuis l'admin (`ADM-004`), après commit : snapshot du brouillon (ou de la version publiée pour une publication), pour une locale.
- **Origines** : par défaut `admin-panel` uniquement, comme Strapi. Option `admin.history.origins: [admin-panel, admin-token, content-api, document-service]` pour tracer davantage.
- **Contenu d'une version** : `documentId`, `uid`, `locale`, `status` (`draft`, `modified`, `published` au moment de la version), `createdAt`, `createdBy`, `data` (attributs, composants et zones dynamiques intégrés), relations et médias stockés comme références (`documentId` + `locale`, id de fichier), et `schema` : empreinte des attributs du content type à cet instant.
- **Liste** : barre latérale paginée, du plus récent au plus ancien, avec date, auteur, badge de statut, nombre total de versions.
- **Affichage** : formulaire en lecture seule de la version. Les champs supprimés ou renommés depuis apparaissent sous « Champs inconnus » ; les champs ajoutés depuis sont signalés « absent de cette version ».
- **Comparaison** (ajout grapi) : diff champ par champ entre la version choisie et le brouillon actuel, ou entre deux versions ; composants et zones dynamiques comparés par position, relations par ensemble de `documentId`.
- **Restauration** : modale de confirmation, puis la version remplace le brouillon de la locale ; le document passe en Modified (ou reste brouillon s'il n'a jamais été publié) ; aucune publication automatique ; une nouvelle version est créée après la restauration.
  - champs inconnus ignorés ; champs ajoutés depuis remis à leur valeur par défaut ;
  - relation ou média qui n'existe plus : ignoré, et listé dans un avertissement avant confirmation ;
  - valeur devenue invalide (énumération retirée, type changé) : champ ignoré et signalé ; la restauration reste un brouillon, donc les règles strictes ne s'appliquent qu'à la publication (`DOC-003`).
- **i18n** (`PLG-001`) : une liste par locale ; restaurer une version restaure aussi les champs non localisés, donc pour toutes les locales, avec un avertissement explicite.
- **Rétention** : `admin.history.retentionDays` (défaut 90, `0` = illimitée), purge quotidienne à minuit (`EXT-005`), plus `admin.history.maxVersionsPerDocument` optionnel. `admin.history.enabled` (défaut `true`).
- **Permissions** : lire l'historique exige `read` sur le content type et la locale ; restaurer exige `update`. Les champs non lisibles par l'utilisateur sont masqués dans la vue et le diff.
- Single types inclus, même mécanique.
- Suppression d'un document : ses versions sont supprimées. Suppression d'un content type : ses versions aussi.

## Hors périmètre
- Historique des médias, des réglages et des schémas.
- Restauration partielle (champ par champ) : amélioration possible.
- Journal exhaustif des changements : c'est le rôle de `XTR-002`.

## Critères d'acceptation
- [x] Enregistrer deux fois puis publier depuis l'admin produit 3 versions aux statuts attendus ; une mise à jour via REST n'en produit aucune (configuration par défaut).
- [x] Restaurer une version remplace le brouillon, met le document en Modified, ne touche pas la version publiée, et ajoute une version.
- [x] Après suppression d'un champ `subtitle` dans le CTB, une ancienne version l'affiche sous « Champs inconnus » et la restauration l'ignore sans erreur.
- [x] Une relation vers un document supprimé est signalée avant restauration et absente après.
- [x] Avec deux locales, restaurer la version `fr` modifie le champ non localisé `price` dans `en` aussi, après avertissement.
- [x] Le diff signale exactement les champs modifiés (fixtures avec composant répétable et zone dynamique).
- [x] Un utilisateur sans `update` voit l'historique mais pas le bouton « Restaurer » ; l'API renvoie 403.
- [x] `retentionDays: 1` purge les versions de plus de 24 h au passage du cron.
- [x] Écran conforme aux conventions de `ADM-002` : sur mobile, la liste des versions passe au-dessus du contenu, pas à côté.

## Avancement
- Table `grapi_history_versions` (créée au démarrage, hors registre de schéma : le banc confirme que Strapi redémarre sur la base, SQLite et Postgres). Une version par enregistrement, création ou publication depuis l'admin (collection et single types), avec statut draft, modified ou published, auteur et empreinte des champs. Les écritures REST, GraphQL et Document Service n'en créent pas.
- API : liste par document et locale, lecture avec champs inconnus et absents, restauration (brouillon remplacé, version publiée intacte, document en Modified, nouvelle version ; relations par `documentId`, médias par id, composants recréés, champs ajoutés depuis à leur défaut). Versions supprimées avec le document. Rétention `admin.history.retentionDays` (90 jours par défaut), `admin.history.enabled`.
- Interface : bouton « Historique » dans l'édition, liste des versions (au-dessus sur mobile), lecture seule de la version, champs inconnus, restauration confirmée (avertissement sur les locales). Testé (`TestAdminHistory`) et vérifié dans Chrome.
- Diff (`GET .../versions/{id}/diff?against=current|<id>`) : composants et zones dynamiques par position, relations par ensemble de `documentId`, médias par id ; onglet « Différences avec le brouillon » dans l'interface (`TestAdminHistoryDiff`).
- Relations et médias disparus : listés dans `danglingReferences` à la lecture, avertissement avant restauration, ignorés à la restauration (`TestAdminHistoryDanglingRelations`).
- Purge par le planificateur du serveur (au démarrage puis au plus une fois par heure), `admin.history.maxVersionsPerDocument`, versions et entrées de releases supprimées avec leur content type (`TestAdminHistoryRetention`, `TestAdminHistoryRules`, `TestAdminHistoryRestoresSharedFields`).
- Origines configurables `admin.history.origins` (`admin-panel`, `admin-token`, `content-api`, `document-service`) : l'origine est portée par le contexte (`document.WithOrigin`), un middleware du Document Service enregistre les écritures des origines listées (`TestAdminHistoryOrigins`). Champ supprimé du schéma : `TestAdminHistoryAfterFieldRemoval`.

## Notes techniques
- Middleware du Document Service (`DOC-004`) qui lit l'origine dans le contexte et enregistre après commit ; la version publiée est capturée après le recâblage de `DOC-002`.
- Table `grapi_history_versions` (`uid`, `document_id`, `locale`, `status`, `data` JSON, `schema` JSON, `created_at`, `created_by_id`), index `(uid, document_id, locale, created_at DESC)`.
- Diff calculé côté Go (`internal/extra/history/diff.go`), réutilisé par l'API et par les tests.
- API : `GET /admin/api/v1/history/:uid/:documentId?locale=`, `GET .../versions/:id`, `GET .../versions/:id/diff?against=current|<id>`, `POST .../versions/:id/restore`.

## Référence
- Content History : https://docs.strapi.io/cms/features/content-history
- Spécification clean-room : aucune source Strapi sous licence Enterprise consultée.

## Dépendances
- `DOC-001`, `DOC-002`, `DOC-003`, `DOC-004`, `PLG-001`, `EXT-005`, `AUTH-001`, `ADM-004`, `ADM-006`, `SETUP-003`

## Fichiers
- `internal/extra/history/` (`recorder.go`, `snapshot.go`, `diff.go`, `restore.go`, `retention.go`, `handlers.go`)
- `admin/src/pages/content-manager/history/`
