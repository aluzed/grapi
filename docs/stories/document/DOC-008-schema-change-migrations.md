---
status: todo
---

# DOC-008 : Migrations au changement de schéma (D&P, i18n) et schéma stocké

## Contexte
Au démarrage, Strapi compare le schéma de son dernier démarrage (core store `strapi_content_types_schema`, tout `strapi.contentTypes` sérialisé, types EE compris) au schéma courant, et ses hooks `beforeSync` et `afterSync` migrent les données : activer draft & publish crée les brouillons des lignes publiées, le désactiver supprime les brouillons, désactiver i18n supprime les locales autres que la locale par défaut, `repairPermissionsForNewlyLocalizedTypes` complète les permissions. Puis Strapi réécrit le schéma stocké.

grapi ne fait aucune de ces migrations et n'écrit pas ce schéma.

## Risques
- Activer D&P sous grapi laisse des lignes publiées sans brouillon : l'admin et `status=draft` ne les voient plus.
- Désactiver D&P sous grapi laisse les brouillons : `published_at` n'est plus « toujours renseigné ».
- Au retour sur Strapi, celui-ci rejoue la migration à partir de l'ancien schéma stocké. C'est correct tant que grapi n'a rien migré ; si grapi migre sans réécrire le schéma, Strapi rejoue la migration (brouillons en double).
- Écrire le schéma stocké approximativement (types EE absents, attributs ajoutés par Strapi, `__schema__`) peut déclencher chez Strapi une migration destructive.

## Objectif
Faire les mêmes migrations que Strapi au changement de schéma, et laisser le schéma stocké dans un état où Strapi ne rejoue rien.

## Périmètre
- Spike : comparer les migrations `draft-publish`, `i18n` et `first-published-at` de `@strapi/core` (MIT) et le contenu exact du schéma stocké, puis décider entre écrire ce schéma (format exact) ou le laisser intact et ne migrer que ce que Strapi ne rejouera pas.
- Migrations D&P (activation, désactivation) et i18n (désactivation).

## Critères d'acceptation
- [ ] Activer D&P sous grapi sur une base existante : chaque ligne publiée a son brouillon, comme Strapi.
- [ ] Désactiver D&P : les brouillons sont supprimés, comme Strapi.
- [ ] Désactiver i18n : seules les lignes de la locale par défaut restent, comme Strapi.
- [ ] Banc : schéma changé sous grapi, puis Strapi redémarré sur la base : aucune donnée en double ni perdue.

## Référence Strapi
- `core/core/src/Strapi.ts` (`bootstrap`), `core/core/src/migrations/draft-publish.ts`, `i18n.ts`, `first-published-at.ts`
- `plugins/i18n/server/src/register.ts` (`afterSync`)

## Dépendances
- `DOC-002`, `PLG-001`, `EXT-008`, `DB-010`
