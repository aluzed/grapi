---
status: in-progress
---

# ADM-016 : Recette de l'admin sur dev-freelance

> Option retenue : B (admin maison), voir `docs/decisions/D1-admin.md`. Conventions UI : voir `ADM-002`.

## Contexte
Le site dev-freelance lit déjà ses données via grapi (déploiement décrit dans le dépôt privé du projet), mais l'édition se fait encore dans l'admin Strapi. Pour basculer aussi l'admin, il faut vérifier qu'on y fait tout ce que l'équipe fait au quotidien, aussi confortablement, et que le JS du projet Strapi (génération IA, invalidation du cache Redis) a son équivalent.

## Objectif
Pouvoir écrire, illustrer et publier les articles dev-freelance dans l'admin grapi sans regretter Strapi.

## Périmètre
- Parcours réels, chronométrés dans les deux admins : écrire un article complet (Markdown avec images), créer et lier une catégorie et un thème, illustrer depuis l'ordinateur, publier, corriger un article publié, dépublier.
- Capture d'écran de chaque écran dans les deux admins, écarts listés et corrigés ou assumés.
- Équivalents Go des comportements du JS Strapi : invalidation du cache du front à la publication, et middleware IA (`EXT-002`, `DOC-004`).

## Critères d'acceptation
- [ ] Chaque parcours se fait dans l'admin grapi, sans étape en plus par rapport à Strapi.
- [ ] Après une publication dans grapi, le site affiche la nouvelle version (cache vidé comme par Strapi).
- [ ] La liste des écarts est vide ou chaque écart restant est validé par l'utilisateur.

## Avancement
- Fait : les équivalents Go du JS du projet (invalidation du cache du front après chaque changement, champs générés par IA), dans le dépôt privé du projet, testés (cache contre un vrai Redis, appel au LLM contre un serveur de test). Les tickets d'admin `ADM-011` à `ADM-015` sont livrés.
- Reste : les parcours chronométrés et les captures dans les deux admins, sur une copie des données ; la liste des écarts à faire valider par l'utilisateur.

## Dépendances
- `ADM-011`, `ADM-012`, `ADM-014`, `ADM-015`, `DOC-004`, `EXT-002`
