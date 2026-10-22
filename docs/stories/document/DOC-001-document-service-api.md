---
status: in-progress
---

# DOC-001 : Document Service : API et documentId

## Contexte
Le Document Service est la couche métier unique : l'admin, l'API REST, GraphQL et les hooks utilisateur passent tous par elle. Il manipule des **documents** (identifiés par `documentId`) et non des lignes : un document regroupe jusqu'à deux lignes par locale (brouillon et publiée, voir `DOC-002`).

## Objectif
Exposer `document.Service(uid)` avec la même surface fonctionnelle que `strapi.documents(uid)`.

## Périmètre
- Méthodes : `FindOne(documentId)`, `FindFirst`, `FindMany`, `Count`, `Create`, `Update`, `Delete`, `Clone`. `Publish`, `Unpublish` et `DiscardDraft` existent seulement si le draft & publish est actif (`DOC-002`).
- Paramètres communs : `fields`, `filters`, `populate`, `sort`, `pagination` (`page`/`pageSize` ou `start`/`limit`), `locale`, `status` (`draft` par défaut, ou `published`), `publicationFilter`, `data` pour les écritures.
- `documentId` : cuid2 de 24 caractères minuscules alphanumériques. Il est stable entre locales et versions. `id` reste l'auto-incrément de la ligne et diffère entre brouillon et publiée.
- `Create` : crée toujours le brouillon. Avec `status: published`, il publie ensuite dans la même transaction.
- `Update` : met à jour le brouillon de la locale demandée. Si le document existe mais pas la locale, la locale est créée et les champs non localisés sont recopiés.
- `Delete` : supprime toutes les versions de la locale demandée (`'*'` = toutes les locales). Avec D&P actif, `status: draft` est refusé.
- `Clone` : nouveau documentId, copie profonde des brouillons, relations comprises.
- Chaque méthode s'exécute dans une transaction (`DB-001`). Un appel imbriqué réutilise la transaction courante.
- Mode strict (`api.documents.strictParams`) : clés racine inconnues, `status`, `locale` ou pagination invalides rejetés en `ValidationError`.

## Hors périmètre
- L'Entity Service v4 (déprécié) n'est pas reproduit.
- La validation des valeurs (`DOC-003`), la résolution des relations (`DOC-006`) et les single types (`DOC-007`).

## Critères d'acceptation
- [ ] `documentId` généré conforme au format cuid2 Strapi (regex `^[a-z0-9]{24}$`) et unique (test sur 1M générations).
- [x] `FindMany` sans `status` renvoie les brouillons, comme Strapi (`post-list-draft`).
- [ ] `Create` puis `FindOne` avec `status: published` renvoie `nil` tant que rien n'est publié (D&P actif).
- [ ] `Update` sur une locale absente crée la ligne de locale avec les champs non localisés recopiés.
- [x] Une erreur au milieu d'un `Create` avec composants annule toute l'écriture (une seule transaction).
- [ ] Banc de compatibilité : findMany, findOne, create, update, delete et clone sur les 21 cas de caractérisation de dev-freelance (admin, REST, documents) donnent le même résultat que Strapi 5.59.

## Avancement
- `internal/document` : `FindMany`, `FindFirst`, `FindOne`, `Count`, `Create`, `Update`, `Delete`, `Publish`, `Unpublish`, `DiscardDraft`, conversion des paramètres (`params.go`, port de `convert-query-params`), résolution documentId vers id dans les données (`relations.go`), composants (`components.go`).
- Banc : opérations `doc*` dans le mode `query` (`components--docs.yaml`, identique à Strapi sur SQLite et Postgres).
- Reste : `clone`, la copie des champs non localisés (`copyNonLocalizedFields`), les événements (`DOC-005`), le validateur d'entrées (`DOC-003`).

## Notes techniques
- La signature Go prend un `context.Context` qui porte la transaction, l'auth et la locale de requête.
- Les paramètres sont une struct typée (`document.Params`) ; l'API REST la construit depuis la query (`API-004`).
- Ne pas exposer de méthode qui écrit sur une ligne précise : tout passe par documentId + locale + status.

## Référence Strapi
- `core/core/src/services/document-service/repository.ts`, `entries.ts`, `params.ts`, `index.ts`
- `core/core/src/services/document-service/transform/` (id-map, id-transform)

## Dépendances
- `SCH-001`, `SCH-005`, `DB-005`, `DB-006`, `DB-007`

## Fichiers
- `document/service.go`, `document/params.go`, `document/cuid2.go`, `document/repository.go`, `document/clone.go`
