# XTR-005 : Content releases (dont planifiées)

> **Clean-room** : fonction sous licence Enterprise chez Strapi. Spécification rédigée uniquement à partir de la documentation publique de Strapi et du comportement observable. Il est **interdit d'ouvrir** les dossiers `ee/`, les paquets `core/review-workflows` et `core/content-releases`, ainsi que `core/content-manager/{admin,server}/src/{history,preview}` (voir `SETUP-003`). Dans grapi, la fonction est sous MIT, sans clé de licence ni limite (nombre de workflows, de releases, durée de rétention...).

## Contexte
Dans Strapi Enterprise, une release regroupe des actions de publication et de dépublication sur des documents de plusieurs content types et de plusieurs locales. Elle est publiée d'un coup, manuellement ou à une date planifiée avec un fuseau horaire. Avant publication, la release indique les documents invalides. Une fois publiée, elle est verrouillée.

## Objectif
Grouper et planifier des publications, avec exécution atomique et contrôle de validité, sans limite de nombre de releases.

## Périmètre
- **Releases** : CRUD (nom, date et heure de publication facultative, fuseau horaire IANA), statuts : vide, prête, bloquée (au moins un document invalide), publiée, échouée.
- **Actions** :
  - ajouter un document à une release, avec l'action publier ou dépublier, depuis la vue édition, depuis la liste en masse, ou depuis la release ;
  - changer l'action, retirer un document ;
  - un même document (même locale) n'apparaît qu'une fois par release.
- **Vue d'une release** : documents groupés par content type, par locale ou par action, état de validité de chacun (avec la raison) et lien vers l'édition.
- **Publication** :
  - toutes les actions sont exécutées dans une seule transaction (tout ou rien) ;
  - en cas d'échec, la release passe en échouée avec le détail ;
  - une release publiée n'est plus modifiable.
- **Planification** :
  - job planifié à la date dans le fuseau donné (`EXT-005`) ;
  - replanification à la modification, annulation à la suppression ;
  - au démarrage, les jobs des releases futures sont recréés, et les releases en retard (instance arrêtée à l'heure prévue) sont publiées immédiatement et marquées comme telles.
- **Événements et webhooks** : `releases.publish` (succès et échec), proposés dans l'écran des webhooks.
- **Nettoyage** : quand un document est supprimé, ses actions sont retirées des releases non publiées.
- **Permissions** : lire, créer, modifier, supprimer, publier une release ; ajouter ou retirer des documents. La publication respecte en plus les permissions publish du type et de la locale de chaque document.
- **Widget de page d'accueil** « Prochaines releases ».

## Hors périmètre
- La publication planifiée d'un document seul sans release : on la couvre avec une release d'un seul document.

## Critères d'acceptation
- [ ] Une release qui contient 10 publications et 2 dépublications sur 3 types et 2 locales s'exécute en une transaction. Une erreur sur l'un annule tout.
- [ ] Une release planifiée à 09:00 Europe/Paris se publie à 09:00 heure de Paris, y compris le jour d'un changement d'heure.
- [ ] Instance arrêtée à l'heure prévue : la release est publiée au démarrage suivant et marquée « publiée en retard ».
- [ ] Un document invalide (champ requis vide) met la release en « bloquée » et empêche la publication manuelle.
- [ ] Supprimer un document le retire des releases en attente.
- [ ] Un utilisateur sans permission publish sur un des types ne peut pas publier la release (403 avec la liste des documents en cause).
- [ ] Écrans conformes aux conventions UI (`ADM-002`), confirmation avant publication et suppression.

## Notes techniques
- Les releases réutilisent `publish` et `unpublish` du Document Service (`DOC-002`), en passant la transaction dans le `context`.
- Le planificateur se base sur une table des releases planifiées et non sur l'état mémoire du cron, pour survivre aux redémarrages et fonctionner avec plusieurs instances (verrou en base pour qu'une seule instance publie).

## Référence Strapi
- Documentation publique Strapi : « Releases », « Scheduled publishing ».

## Dépendances
- `DOC-002`, `DOC-003`, `DOC-005`, `EXT-005`, `EXT-006`, `AUTH-001`, `ADM-003`, `ADM-004`, `ADM-008`

## Fichiers
- `internal/plugins/releases/{releases,actions,validate,publish,scheduler,handlers}.go`
- `admin/src/pages/releases/`, `admin/src/features/releases/`
