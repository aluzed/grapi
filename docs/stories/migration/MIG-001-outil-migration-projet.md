# MIG-001 : Outil de migration d'un projet Strapi

## Contexte
Un projet Strapi, c'est des schémas JSON (réutilisés tels quels par grapi, `SCH-004`), une configuration en JS ou TS, et du code utilisateur JS ou TS : controllers, services, lifecycles, policies, middlewares et `src/index`. La base est ouverte directement (`DB-011`). Reste à convertir la configuration et à accompagner la réécriture du code (D2).

## Objectif
Une commande `grapi migrate-from-strapi <dir>` qui produit un projet grapi prêt à démarrer. Elle convertit automatiquement tout ce qui est déterministe et produit un rapport précis pour le reste.

## Périmètre
- **Analyse** (étend `grapi doctor`, `SCH-004`) :
  - inventaire des schémas, des plugins activés (`config/plugins`) et des dépendances `@strapi/*` du `package.json`, avec leur version ;
  - inventaire des fichiers de code utilisateur.
- **Configuration** :
  - évaluation des `config/*.{js,ts}` et `config/env/<env>/*` dans un bac à sable (esbuild + goja, `EXT-003`), avec un `env()` instrumenté qui enregistre chaque appel `env('KEY', default)` ;
  - génération des fichiers de configuration déclaratifs grapi (`EXT-001`), qui contiennent des **références** `${env:...}`, pas des valeurs ;
  - si une config dépend de logique non évaluable, elle est signalée et laissée en TODO.
- **Variables d'env** : `.env.example` du projet grapi généré avec toutes les clés relevées. Les valeurs des `.env` ne sont jamais copiées ni affichées.
- **Code utilisateur** :
  - `createCoreRouter`, `createCoreController` et `createCoreService` sans surcharge sont reconnus comme triviaux et supprimés (le défaut de grapi suffit) ;
  - pour le reste, le rapport pointe le fichier et la fonction, et propose un squelette de hook Go (`EXT-002`) généré dans `hooks/` avec la signature exacte ;
  - en phase 3, il propose aussi l'option de garder le fichier en hook JS (`EXT-003`), après vérification que ses imports sont compatibles (pas de module natif, pas d'API Node non polyfillée).
- **Plugins** : correspondance des plugins officiels vers les plugins grapi. Les plugins tiers sont listés comme « non supportés », avec un lien vers `EXT-004`.
- **Rapport** : `MIGRATION.md` dans le projet généré, organisé par catégorie (automatique, à faire, non supporté), chaque point avec son fichier source.

## Hors périmètre
- Traduire automatiquement le JS en Go.
- La migration des données (inutile grâce à `DB-011`).

## Critères d'acceptation
- [ ] Sur une copie de `dev-freelance/apps/backend` :
  - la commande produit un projet qui démarre (schémas et config) ;
  - `MIGRATION.md` liste `src/index.ts` (middlewares de slug, IA et cache), `libs/row-rules.ts`, le controller `comment` et le lifecycle `user`.
- [ ] Les fichiers de config générés ne contiennent aucune valeur de secret, seulement des références d'env (vérifié par un test qui injecte des secrets factices et cherche leur valeur dans la sortie).
- [ ] Sur chaque fixture du banc, le projet migré démarre et le banc passe comme sur la fixture d'origine.
- [ ] Une config qui appelle une fonction Node non disponible (`fs.readFileSync`) est signalée dans le rapport avec le fichier et la ligne, sans faire planter la migration.

## Notes techniques
- La clé DKIM de dev-freelance est lue au chargement de la config (`fs.readFileSync` dans `config/plugins.ts`) : c'est un cas réel de config non évaluable dans goja. Il faut prévoir une directive `${file:path}` dans la config grapi (`EXT-001`).
- Ne jamais lire les fichiers `.env` réels. On ne lit que `.env.example`, s'il existe, et les appels `env()` du code.

## Référence Strapi
- `core/core/src/configuration/` (`config-loader.ts`, `utils/load-config-file.ts`)
- `utils/upgrade/` (outillage de codemods, pour l'approche)
- dev-freelance `apps/backend/config/plugins.ts`, `apps/backend/src/index.ts`

## Dépendances
- `SCH-004`, `EXT-001`, `EXT-002`. `EXT-003` pour l'évaluation de la config et l'option hooks JS.

## Fichiers
- `cmd/grapi/migrate.go`, `internal/migrate/{analyze,config,codegen,report}.go`, `internal/migrate/templates/`
