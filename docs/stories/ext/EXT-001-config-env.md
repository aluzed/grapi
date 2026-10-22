---
status: in-progress
---

# EXT-001 : Configuration déclarative et env()

## Contexte
Chez Strapi, la configuration est faite de fichiers JS/TS (`config/*.ts`) qui exportent un objet ou une fonction `({env}) => ({...})`. Go ne peut pas les exécuter nativement (décision D2). Il faut un format déclaratif qui offre les mêmes possibilités, notamment le helper `env()` et ses conversions.

## Objectif
Charger la configuration du projet, avec la même structure de clés que Strapi, à partir de fichiers déclaratifs.

## Périmètre
- Formats acceptés : `config/<nom>.yaml`, `.yml`, `.json`, `.toml`. Les fichiers `.js` et `.ts` arrivent avec `EXT-003`.
- Interpolation : `${env:CLE}`, `${env:CLE:defaut}`, avec conversions `${env.int:CLE:1337}`, `env.float`, `env.bool` (vrai uniquement si la valeur vaut exactement `true`), `env.json` (erreur explicite si JSON invalide), `env.array` (découpe sur les virgules, retire les `[]` englobants, les espaces et les guillemets), `env.date`, `env.oneOf:CLE:a|b|c:defaut` (le défaut doit faire partie des valeurs).
- Lecture de fichier : `${file:chemin}` injecte le contenu d'un fichier relatif à la racine du projet (clés privées DKIM, certificats). C'est l'équivalent des `fs.readFileSync` qu'on trouve dans les configs Strapi (cas dev-freelance, voir `MIG-001`). Un fichier absent fait échouer le démarrage avec le nom de la clé de config.
- Une valeur entièrement interpolée garde son type (entier, booléen, tableau).
- Chargement de `.env` (ou du fichier pointé par `ENV_PATH`), sans écraser les variables déjà définies.
- Alias des variables Strapi (décision du 2026-10-22) : chaque variable `GRAPI_*` accepte aussi son nom Strapi (`NODE_ENV` pour `GRAPI_ENV`, `STRAPI_*`, etc.), avec une table d'alias unique dans `internal/config/aliases.go`. Si les deux sont définis, `GRAPI_*` l'emporte et un avertissement est journalisé.
- Fusion : valeurs par défaut de grapi, puis `config/*`, puis `config/env/<GRAPI_ENV|NODE_ENV>/*`.
- Fichiers connus : `server`, `database`, `admin`, `api`, `middlewares`, `plugins`, `features`, `cron-tasks`. Tout autre nom devient un espace de config custom. Les noms réservés (`uuid`, `license`, `telemetry`, `strapi`, `internal`, `dirs`, etc.) sont refusés.
- Valeurs par défaut clés : `server.port` 1337, `server.host` 0.0.0.0, `api.rest.prefix` `/api`, `api.rest.defaultLimit` 25.
- Validation du résultat contre un schéma de config typé : erreur claire avec fichier et clé.
- Accès typé en Go (`app.Config().Server.Port`) et générique (`app.Config().Get("plugins.upload.config.sizeLimit")`).

## Hors périmètre
- Les valeurs fonctionnelles (origin CORS calculée, `responses.handlers`) : disponibles seulement en mode framework Go (`EXT-002`).

## Critères d'acceptation
- [ ] La config de dev-freelance (server, database, admin, api, middlewares, plugins) se transcrit en YAML et donne le même objet fusionné que Strapi (dump comparé sur le banc).
- [x] `${env.bool:X}` vaut `false` pour `X=1` (comportement Strapi conservé).
- [x] `${env.oneOf:DB:postgres|sqlite:mysql}` fait échouer le démarrage (défaut hors liste).
- [x] `${file:dkim/private.key}` injecte le contenu du fichier ; un fichier absent fait échouer le démarrage.
- [x] `config/env/production/server.yaml` surcharge `config/server.yaml` en production seulement.

## Avancement
- Fait : `internal/config` (YAML, JSON, directives, `.env`, alias, fusion lodash, noms réservés, contrôle de type des clés lues), branché dans `grapi start` ; doc `docs/configuration.md`.
- Reste : TOML, dump comparé à Strapi sur la config dev-freelance, accès typé complet (`app.Config().Server.Port`), valeurs par défaut des autres fichiers.

## Notes techniques
- Garder une table de correspondance documentée « clé Strapi » vers « clé grapi » : identique sauf exception justifiée.
- `CLI-003` (`configuration:dump`) concerne le core store, pas ce fichier.

## Référence Strapi
- `core/core/src/configuration/index.ts`, `config-loader.ts`, `urls.ts`
- `core/core/src/utils/load-config-file.ts`, `core/utils/src/env-helper.ts`
- `core/types/src/core/config/*.ts` (formes de config)

## Dépendances
- `SETUP-001`

## Fichiers
- `internal/config/load.go`, `internal/config/env.go`, `internal/config/interpolate.go`, `internal/config/schema.go`, `internal/config/defaults.go`
