# SETUP-003 : Règles clean-room et licences des dépendances

## Contexte
grapi doit rester intégralement MIT (D6). Le dépôt Strapi est MIT, sauf les dossiers `ee/` et quelques dossiers sous licence EE qui ont leur propre `LICENSE` (par exemple preview et history dans le Content Manager). Les fonctionnalités ne sont pas protégeables, le code oui.

## Objectif
Écrire et outiller les règles qui garantissent que grapi ne contient aucun code dérivé de parties non MIT, et aucune dépendance de licence incompatible.

## Périmètre
- **`docs/clean-room.md`** :
  - Le code MIT de Strapi peut être lu comme spécification. On reproduit les comportements, pas la structure : pas de traduction ligne à ligne, ni de copie de commentaires ou de noms internes non exposés.
  - Interdiction absolue de lire les dossiers `ee/`, et tout dossier contenant un `LICENSE` qui n'est pas MIT. Les fonctions concernées (`XTR-*`) se spécifient uniquement à partir de la documentation publique de Strapi. On n'utilise pas d'instance sous licence d'essai, dont les conditions s'appliqueraient à nous.
  - Les identifiants exposés (noms de tables, de routes, de champs JSON, codes d'erreur) font partie du contrat de compatibilité et peuvent être repris tels quels.
  - Les données publiques (liste des locales ISO, par exemple) se prennent de leur source d'origine (CLDR, ISO), pas du fichier JSON de Strapi.
- **Script `scripts/list-forbidden-paths.sh`** : liste les chemins non MIT du clone Strapi de référence, en cherchant les `LICENSE` autres que MIT et les dossiers `ee/`. Il est utilisé pour configurer les exclusions de recherche des outils et des agents.
- **Contrôle des licences des dépendances Go** : `google/go-licenses` en CI. Licences autorisées : MIT, BSD-2, BSD-3, Apache-2.0, ISC, MPL-2.0 (au niveau du fichier uniquement). GPL, AGPL, LGPL, SSPL et BUSL sont refusées.
- **Contrôle des dépendances npm** de l'admin (`ADM-*`) : `license-checker` en mode liste blanche, avec les mêmes règles.
- **`NOTICE`** : liste les dépendances embarquées dans le binaire (admin compris) et leurs licences, générée automatiquement au build de release.
- **Modèle de PR** : case à cocher « aucune source sous `ee/` ou sous licence non MIT consultée ».

## Hors périmètre
- L'avis juridique formel : à faire valider par un juriste si le projet prend de l'ampleur.

## Critères d'acceptation
- [ ] `docs/clean-room.md` est en place et référencé depuis `CONTRIBUTING.md`.
- [ ] La CI échoue si une dépendance Go GPL est ajoutée (testé avec une branche jetable).
- [ ] `scripts/list-forbidden-paths.sh ~/Documents/dev/strapi` liste au minimum `packages/core/admin/ee` et les dossiers EE du Content Manager.
- [ ] `NOTICE` est généré par `make release` et inclus dans l'image Docker.
- [ ] Le modèle de PR contient la case clean-room.

## Notes techniques
- Cas de la réutilisation de la SPA Strapi (si D1 = A) : le bundle doit exclure physiquement les dossiers EE. Il faut vérifier à chaque mise à jour que le build n'importe aucun fichier sous licence EE (analyse du graphe d'import de Vite). C'est un argument de plus pour D1 = B.
- Pour les fonctions Enterprise, la spécification observable vient de la documentation publique de Strapi et de son guide utilisateur. Il faut garder une trace de la source de chaque règle métier dans le ticket.

## Référence Strapi
- `LICENSE` à la racine du clone (MIT avec l'exception pour `ee/`).
- Chemins sous licence Enterprise dans la 5.59, vérifiés via leurs fichiers `LICENSE`, à **ne pas ouvrir** (seulement à lister) :
  - `packages/core/admin/ee/`, `packages/core/core/src/ee/` ;
  - `packages/core/review-workflows/` et `packages/core/content-releases/` (paquets entiers) ;
  - `packages/core/content-manager/{admin,server}/src/history/` et `packages/core/content-manager/{admin,server}/src/preview/`.
- Le script `list-forbidden-paths.sh` doit trouver ces chemins en cherchant les fichiers `LICENSE` qui contiennent « Enterprise License », pas en se fiant au nom `ee/`.

## Dépendances
- `SETUP-001`.

## Fichiers
- `docs/clean-room.md`, `scripts/list-forbidden-paths.sh`, `.github/workflows/licenses.yml`, `.github/pull_request_template.md`, `NOTICE` (généré), `Makefile` (cible `licenses`)
