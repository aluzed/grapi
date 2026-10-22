---
status: in-progress
---

# DOC-002 : Draft & Publish

## Contexte
Avec `options.draftAndPublish: true`, un document a, par locale, une ligne brouillon (`published_at IS NULL`) et au plus une ligne publiée (`published_at` renseigné), partageant le même `document_id`. Sans D&P, il n'y a qu'une ligne dont `published_at` est toujours renseigné (le champ existe même quand D&P est désactivé).

## Objectif
Reproduire exactement le cycle de vie brouillon / publié de Strapi 5, y compris les effets de bord visibles par les hooks.

## Périmètre
- `Publish(documentId, locale)` : supprime l'ancienne ligne publiée, puis **insère une copie du brouillon** comme nouvelle ligne publiée (nouvel `id`). Conséquence observée sur dev-freelance : le `beforeCreate` de la base se déclenche à chaque publication, et les règles de ligne (slug recalculé) s'appliquent à la version publiée.
- Recâblage des relations à la publication : les relations unidirectionnelles, bidirectionnelles et auto-référentes qui pointaient vers l'ancienne ligne publiée sont reportées sur la nouvelle.
- `Unpublish` : supprime les lignes publiées. `DiscardDraft` : remplace le brouillon par une copie de la version publiée (inverse de publish).
- `locale: '*'` accepté par publish, unpublish et discardDraft.
- Paramètre `status` (`draft` par défaut, `published`) pour la lecture.
- `publicationFilter` : `never-published`, `has-published-version`, `modified`, `unmodified`, `never-published-document`, `has-published-version-document`, `published-without-draft`, `published-with-draft`. `hasPublishedVersion` est accepté mais déprécié.
- Les validations strictes (required, min/max, longueurs, regex, unique) s'appliquent à la publication et non au brouillon (`DOC-003`).
- Événements `entry.publish`, `entry.unpublish`, `entry.draft-discard` (`DOC-005`).

## Hors périmètre
- `firstPublishedAt` (expérimental chez Strapi, derrière un flag) : ticket ultérieur si besoin.
- Content history (`XTR-003`), releases (`XTR-005`).

## Critères d'acceptation
- [ ] Après publish, la ligne publiée a un nouvel `id` et le même `document_id`, et les relations entrantes pointent vers elle.
- [ ] Un hook `beforeCreate` de ligne reçoit l'événement à la publication (cas dev-freelance : slug publié recalculé depuis le titre).
- [ ] Une catégorie liée à un post publié reste liée après republication du post (relation bidirectionnelle) et après republication de la catégorie.
- [ ] Les 8 modes de `publicationFilter` renvoient les mêmes documents que Strapi sur un jeu de fixtures couvrant chaque état.
- [ ] Sans D&P, `publish` n'existe pas (erreur explicite) et `published_at` est toujours renseigné.
- [ ] Banc de compatibilité : séquence create, publish, update, publish, unpublish, discardDraft identique à Strapi (contenu des deux versions, relations, `publishedAt`).

## Avancement
- Publish, unpublish et discardDraft copient les versions avec leurs composants et leurs relations sortantes (résolues vers la version cible, `allowMissingId`) : séquence identique à Strapi sur `components--docs.yaml`.
- Reste : la resynchronisation des relations entrantes (`unidirectional-relations`, `bidirectional-relations`, `self-referential-relations`), `publicationFilter`.

## Notes techniques
- Le recâblage est le point le plus délicat de toute la couche données : écrire d'abord les tests de caractérisation contre Strapi.
- Tout se fait dans une seule transaction ; en cas d'échec, l'ancienne version publiée doit rester intacte.

## Référence Strapi
- `core/core/src/services/document-service/draft-and-publish.ts`, `repository.ts`
- `core/core/src/services/document-service/utils/*-relations.ts` (unidirectional, bidirectional)
- `core/utils/src/publication-filter.ts`, `core/core/src/services/document-service/publication-filter.ts`

## Dépendances
- `DOC-001`, `DB-006`, `DB-009`

## Fichiers
- `document/publish.go`, `document/relations_rewire.go`, `document/publication_filter.go`
