# XTR-003 : Content history

> **Clean-room** : fonction sous licence Enterprise chez Strapi. Spécification rédigée uniquement à partir de la documentation publique de Strapi et du comportement observable. Il est **interdit d'ouvrir** les dossiers `ee/`, les paquets `core/review-workflows` et `core/content-releases`, ainsi que `core/content-manager/{admin,server}/src/{history,preview}` (voir `SETUP-003`). Dans grapi, la fonction est sous MIT, sans clé de licence ni limite (nombre de workflows, de releases, durée de rétention...).

## Contexte
Strapi Enterprise conserve l'historique des versions d'un document. Une version est enregistrée à chaque sauvegarde et à chaque publication. Un panneau permet de les parcourir, de voir l'état du document à chaque version (avec les champs depuis supprimés ou modifiés dans le schéma) et de restaurer une version comme nouveau brouillon. Une rétention limite la durée de conservation.

## Objectif
Historiser chaque version des documents, permettre la consultation et la restauration, avec une rétention configurable.

## Périmètre
- **Capture** :
  - une version par création, mise à jour et publication, et par locale ;
  - contenu complet du document (scalaires, composants, zones dynamiques, références des relations et des médias), statut, auteur, date ;
  - schéma du content type au moment de la version.
- **Activation** par content type (option de schéma, active par défaut sur les types avec draft & publish), et désactivable globalement.
- **Consultation** :
  - depuis la vue édition, accès à la liste des versions (date, auteur, statut) ;
  - vue lecture seule d'une version ;
  - indication des champs qui n'existent plus dans le schéma actuel (« champs inconnus ») et des relations ou médias supprimés depuis.
- **Restauration** :
  - remplace le brouillon courant par la version choisie, après confirmation ;
  - les relations vers des documents disparus sont ignorées et signalées ;
  - la restauration crée elle-même une nouvelle version.
- **Rétention** en jours (`contentHistory.retentionDays`, 90 par défaut), purge par cron.
- **Permissions** : lecture de l'historique et restauration, alignées sur les permissions read et update du type et de la locale.

## Hors périmètre
- Diff visuel champ par champ entre deux versions : amélioration ultérieure, à prévoir dans le modèle de données.

## Critères d'acceptation
- [ ] Trois sauvegardes puis une publication produisent quatre versions, dans l'ordre, avec l'auteur.
- [ ] Après suppression d'un champ dans le schéma, l'ancienne version l'affiche dans « champs inconnus ».
- [ ] Restaurer une version remet exactement ses valeurs dans le brouillon (vérifié via l'API REST avec `status=draft`) et ne touche pas à la version publiée.
- [ ] Une relation vers un document supprimé est ignorée à la restauration et signalée.
- [ ] La purge respecte la rétention.
- [ ] Un utilisateur sans permission update ne voit pas le bouton de restauration, et le serveur refuse.
- [ ] Panneau conforme aux conventions UI (`ADM-002`), avec confirmation avant restauration.

## Notes techniques
- Capture via un middleware du Document Service (`DOC-004`), dans la même transaction que l'écriture.
- Stockage en JSON (JSONB sur Postgres) avec index (content_type, related_document_id, locale, created_at).

## Référence Strapi
- Documentation publique Strapi : « Content History ».

## Dépendances
- `DOC-001`, `DOC-002`, `DOC-004`, `EXT-005`, `ADM-004`, `AUTH-001`

## Fichiers
- `internal/plugins/history/{capture,store,restore,purge,handlers}.go`
- `admin/src/features/history/`
