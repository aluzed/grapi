# Règles clean-room

grapi reproduit les fonctionnalités de Strapi 5, pas son code. Ces règles valent pour les humains comme pour les agents IA.

## Ce qui est autorisé

- Lire le code **MIT** de Strapi comme spécification de comportement : règles métier, formats d'entrée et de sortie, cas limites.
- Reprendre tel quel tout ce qui fait partie du contrat de compatibilité : noms de tables et de colonnes, routes, champs JSON, paramètres de requête, codes et noms d'erreur, formats d'archive.
- Exécuter Strapi Community (MIT) pour observer son comportement : c'est le rôle du banc de compatibilité.

## Ce qui est interdit

- Traduire le code ligne à ligne, ou reprendre ses commentaires, sa structure interne ou ses noms internes non exposés.
- **Lire du code sous licence Enterprise**, sous quelque forme que ce soit (fichiers, noms de fichiers, diff, GitHub).
  - Cette licence ne s'arrête pas aux dossiers `ee/` : elle couvre tout dossier dont le `LICENSE` est la licence Enterprise.
  - La liste à jour se calcule avec `scripts/list-forbidden-paths.sh <clone-strapi>`. En 5.59 :
    - `packages/core/admin/ee`, `packages/core/core/src/ee` ;
    - `packages/core/review-workflows`, `packages/core/content-releases` ;
    - `packages/core/content-manager/{admin,server}/src/{history,preview}` ;
    - les dossiers `ee` sous `docs/` et `tests/`.
- Utiliser une instance Strapi sous licence Enterprise, même en essai : ses conditions s'appliquent à qui l'utilise.

## Fonctions Enterprise de Strapi

SSO, audit logs, content history, review workflows, releases et preview (tickets `XTR-*` et `ADM-009`) se spécifient **uniquement** à partir de la documentation publique (docs.strapi.io) et de connaissances générales du domaine. Chaque ticket cite les pages consultées.

## Données publiques

Les référentiels publics se prennent à leur source (ISO 639, CLDR pour les locales, IANA pour les MIME types), jamais depuis les fichiers JSON de Strapi.

## Agents IA

Tout agent qui travaille sur grapi reçoit la liste des chemins interdits et l'instruction de ne jamais les ouvrir. Les tâches `XTR-*` sont confiées à un agent sans contexte préalable sur le code de Strapi.

## Dépendances

- Go : `go-licenses` en CI, avec cette liste blanche : MIT, BSD-2-Clause, BSD-3-Clause, Apache-2.0, ISC, MPL-2.0. Tout le reste est refusé (GPL, LGPL, AGPL, SSPL, BUSL, etc.).
- npm (admin) : même liste blanche, vérifiée par `license-checker` quand l'admin arrive.
- `make notice` génère `NOTICE`, la liste des dépendances embarquées dans le binaire avec leurs licences.

Ce document ne remplace pas un avis juridique.
