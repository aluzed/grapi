---
status: done
---

# DOC-007 : Single types

## Contexte
Un single type (`kind: singleType`) n'a qu'un seul document par locale (page d'accueil, réglages globaux, profil).

## Objectif
Gérer les single types dans le Document Service et la core API.

## Périmètre
- Lecture : le document unique de la locale demandée, ou `404 NotFoundError` côté REST s'il n'existe pas.
- `PUT` : crée le document s'il n'existe pas, sinon met à jour (upsert).
- `DELETE` : supprime la locale demandée.
- Draft & publish et i18n identiques aux collection types.

## Hors périmètre
- Les routes elles-mêmes (`API-006`).

## Critères d'acceptation
- [x] `GET /api/<singular>` sur un single type vide renvoie le même 404 que Strapi.
- [x] Deux `PUT` successifs ne créent qu'un document.
- [x] Banc de compatibilité sur le single type `profile` de dev-freelance.

## Avancement
- Banc en mode grapi, SQLite et Postgres : `basic / homepage-missing`, `homepage-put`, `homepage-get`, `homepage-put-again`, `homepage-delete` identiques à Strapi. `profile` est un collection type dans dev-freelance (`kind: collectionType`), couvert par `devfreelance / front-home-profile` ; le cas single type est celui de `homepage`.

## Notes techniques
- Pas de documentId dans l'URL : le service le retrouve par `FindFirst`.

## Référence Strapi
- `core/core/src/core-api/service/single-type.ts`, `core/core/src/core-api/controller/single-type.ts`

## Dépendances
- `DOC-001`

## Fichiers
- `document/single_type.go`
