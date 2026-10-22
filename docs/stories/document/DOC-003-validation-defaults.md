---
status: in-progress
---

# DOC-003 : Validation des entrées et valeurs par défaut

## Contexte
Strapi valide les données selon le schéma avant écriture. Les règles diffèrent entre brouillon et publication : un brouillon peut être incomplet.

## Objectif
Valider chaque écriture du Document Service avec les mêmes règles et les mêmes messages d'erreur structurés que Strapi.

## Périmètre
- Règles par type : string/text (minLength, maxLength, regex), email, enumeration (valeur dans `enum`), integer/biginteger/float/decimal (min, max, type numérique), date/time/datetime/timestamp (format), boolean, json, blocks (validateur structurel), uid (regex par défaut `^[A-Za-z0-9-_.~]*$` ou `regex` du schéma), password, media (`allowedTypes`), relation (cible existante), component (min/max si repeatable), dynamiczone (composants autorisés, min/max).
- `required`, `min`/`max`, longueurs, `regex` et `unique` sont **levés en brouillon** et appliqués à la publication.
- `unique` : vérifié parmi les lignes publiées, par locale, en excluant le même document. Vérifié aussi dans les composants répétables et zones dynamiques.
- Valeurs par défaut (`default`, valeur ou fonction côté hook Go) appliquées à la création.
- Attributs non `writable` ignorés en entrée.
- Erreur : `ValidationError` avec `details.errors[]` contenant `path`, `message`, `name` (format `API-007`).

## Hors périmètre
- Le sanitize lié aux permissions (`API-005`).

## Critères d'acceptation
- [ ] Un brouillon sans champ requis est accepté ; sa publication échoue avec `ValidationError` et le `path` du champ.
- [ ] Deux documents publiés avec la même valeur `unique` dans la même locale : la seconde publication échoue ; dans deux locales différentes, elle réussit.
- [ ] Les messages et `details.errors` sont identiques à Strapi pour un jeu de 30 cas (banc de compatibilité).
- [ ] `default` appliqué à la création seulement, jamais à l'update.

## Notes techniques
- Validateur maison piloté par le schéma (pas de lib de validation par tags : le schéma est dynamique).
- Reproduire les messages yup de Strapi tels quels : les fronts existants les affichent.

## Référence Strapi
- `core/core/src/services/entity-validator/`
- `core/utils/src/validate/`

## Dépendances
- `SCH-001`, `SCH-002`, `DOC-001`

## Fichiers
- `document/validate/validate.go`, `document/validate/rules.go`, `document/validate/unique.go`, `document/defaults.go`
