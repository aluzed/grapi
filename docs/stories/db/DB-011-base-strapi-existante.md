---
status: in-progress
---

# DB-011 : Ouverture d'une base Strapi 5 existante sans migration

## Contexte
Décision D3 : migrer de Strapi vers grapi, c'est changer de binaire, sans transférer les données. Ce ticket ne crée pas de mécanisme nouveau : il vérifie, de bout en bout, que l'ensemble de la couche données (`DB-001` à `DB-010`) lit et écrit une base Strapi 5.59 sans la modifier ni la casser.

## Objectif
Démarrer grapi sur une base créée et utilisée par Strapi 5.59, sans aucune modification de schéma, et pouvoir revenir à Strapi ensuite (réversibilité).

## Périmètre
- **Garde de démarrage** :
  - détecter une base Strapi (présence de `strapi_database_schema` et `strapi_migrations_internal`) ;
  - lire la version de Strapi d'origine si elle est disponible ;
  - refuser une base v4 (`DB-004`).
- **Mode `--dry-run-sync`** : calcule le diff (`DB-003`) et l'affiche sans rien appliquer. C'est la première commande à lancer sur une base de prod copiée.
- **Contrôles de cohérence**, avec un rapport `grapi db:check` :
  - toutes les tables et colonnes attendues existent, avec le même type ;
  - les `document_id` sont présents et au format cuid2 ;
  - pas de lignes orphelines dans les `_lnk`, `_mph` et `_cmps` ;
  - les hash de mots de passe sont lisibles par bcrypt (`$2a$`, `$2b$`).
- **Écriture compatible** :
  - les entrées créées par grapi sont lues à l'identique par Strapi 5.59 (documentId, ordre des relations, composants, médias, core store) ;
  - les migrations et repairs propres à grapi ne laissent aucune trace qui gênerait Strapi.
- **Tables admin et utilisateurs** (`admin_users`, `up_users`, rôles, permissions, API tokens) : lues sans réécriture au démarrage. La règle de l'utilisateur s'applique : on ne réinitialise jamais la table admin.

## Hors périmètre
- MySQL (phase 3).
- Les bases v4.

## Critères d'acceptation
- [x] Sur une copie de la base de prod de dev-freelance (sur le homelab, jamais sur le serveur de prod), `grapi --dry-run-sync` renvoie `UNCHANGED`.
- [x] `grapi db:check` sur cette copie ne remonte aucune erreur.
- [x] Aller-retour : Strapi 5.59 crée la base, grapi crée, modifie et publie 20 entrées (relations, composants, médias), puis Strapi redémarre sur la même base sans changement de schéma. Le banc donne alors des réponses identiques depuis les deux moteurs.
- [ ] Les comptes admin et utilisateurs finaux existants se connectent avec leur mot de passe sous grapi (dépend de `AUTH-002` et `AUTH-007`, test reporté en phase 2 pour l'admin).
- [x] Le dump SQL de schéma (`pg_dump --schema-only`) est identique avant et après un démarrage de grapi.

## Avancement
- 2026-10-22, copie de prod sur le homelab (supprimée ensuite) : `grapi db:sync --dry-run` donne `UNCHANGED` avec le hash stocké identique, `grapi db:check` donne `OK`, et après un `grapi start` le dump `--schema-only` (hors jeton `\restrict` aléatoire de pg_dump) ainsi que `strapi_database_schema`, `strapi_migrations_internal` et `strapi_core_store_settings` sont inchangés.
- Le hash coïncide parce que grapi reprend du schéma stocké les tables des fonctions qu'il ne reproduit pas (`strapi_release*`, `strapi_workflows*`, `strapi_history_*`, `strapi_ai_*`).
- Aller-retour (2026-10-22) : en mode grapi du banc, Strapi crée la base, grapi joue tout le corpus (créations, mises à jour, publications, relations, composants, médias, locales), puis Strapi redémarre sur cette base sans changement de schéma et relit chaque content type comme grapi le sert : contrôle inverse OK sur les 6 fixtures, SQLite et Postgres (seul l'écart bigCounter documenté reste sous SQLite).
- Reste : la connexion des comptes existants (mots de passe de prod inconnus, à faire avec un compte de test).

## Notes techniques
- Toujours travailler sur une copie : la base de prod est copiée en flux vers le homelab (serveur vers homelab, sans jamais afficher de secret), comme pour les tests de la mise à jour Strapi 5.59.
- Ce ticket est la porte d'entrée de `MIG-002`.

## Référence Strapi
- `core/database/src/schema/storage.ts`, `core/database/src/migrations/internal-migrations/`
- `core/core/src/Strapi.ts` (séquence de démarrage de la base)

## Repris d'autres tickets
- `DB-008` : sur la copie de la base dev-freelance, les couvertures des posts (`cover`, populate) se lisent à l'identique entre Strapi et grapi.

## Dépendances
- `DB-001` à `DB-010`.

## Fichiers
- `internal/db/compat/detect.go`, `internal/db/compat/check.go`, `cmd/grapi/db.go`
