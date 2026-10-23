# Banc de compatibilité

Envoie le même corpus de requêtes à Strapi 5.59 (référence) et à un second moteur, normalise les réponses et produit un diff. Tout tourne sur le homelab (`COMPAT_HOST`, `homelab` par défaut) : rien n'est lancé en local.

```sh
make bench-compat FIXTURE=basic                  # Strapi contre Strapi, SQLite
make bench-compat FIXTURE=basic DB=postgres
make bench-compat FIXTURE=basic MODE=grapi       # Strapi contre grapi (quand grapi sait servir)
```

Le rapport arrive dans `compat/report/<date>-<fixture>-<mode>-<db>.md` (non versionné). En mode `grapi`, un contrôle inverse suit : grapi est arrêté, Strapi redémarre sur la base que grapi vient d'écrire, et un instantané de chaque content type (`compat snapshot` : brouillons et publiés, toutes les locales, `populate=*`, utilisateurs et fichiers) est comparé entre les deux moteurs (`<date>-<fixture>-reverse-<db>.md`). Le contrôle direct le précède : un second grapi ouvre la base que la référence (a) a remplie avec le corpus, et le même instantané y est comparé à Strapi (`<date>-<fixture>-forward-<db>.md`). Le journal de grapi est copié dans `<date>-<fixture>-grapi-<db>.log`. Le mode `toggle` inverse draft & publish sur tous les types `api::` d'une copie de la fixture : grapi migre la base que Strapi a remplie, puis Strapi redémarre dessus avec les mêmes schémas, et un instantané compare les deux (`<date>-<fixture>-toggle-<db>.md`). Strapi ne doit rien rejouer. `MODE=perf` monte les deux moteurs sur les mêmes données puis mesure débit, latences et mémoire sur les cas GET sans variable (`compat perf`, réglable par `PERF_C` et `PERF_D`) : `<date>-<fixture>-perf-<db>.md`. Les sorties normalisées de la référence sont versionnées dans `compat/golden/<fixture>-<db>/` : elles servent de spécification lisible, et un diff git montre ce qui change quand on monte de version de Strapi.

## Contenu

| Chemin | Rôle |
|---|---|
| `reference/` | Image Strapi de référence (versions figées dans `versions.env`). Les fixtures ne contiennent que des `schema.json` : l'entrypoint génère les routes, controllers et services standards. Le bootstrap donne `find`/`findOne` au rôle Public et écrit deux tokens (`full`, `read`) dans `/data/tokens.json`. Un `fixtures/<nom>/compat.json` facultatif ajoute des locales (`{"locales":[{"code":"fr","name":"French (fr)"}]}`) et des permissions Public (`"publicPermissions":["plugin::users-permissions.user.find"]`), ainsi que des options users-permissions (`"upConfig":{"jwtManagement":"refresh","sessions":{"cookie":{"secure":false}}}`), passées à Strapi et, pour `jwtManagement` et `cookie.secure`, à grapi par `remote.sh`, et `"bodyMaxFileSize"` (octets), le `formidable.maxFileSize` de `strapi::body` des deux moteurs (`BODY_MAX_FILE_SIZE` côté grapi) |
| `fixtures/<nom>/src/` | Projets de test, lus tels quels par les deux moteurs. `devfreelance` copie les schémas de dev-freelance (sans JS) ; son corpus rejoue les appels exacts du front Go (`apps/web/internal/*`). `naming` force des identifiants de plus de 55 caractères (tables, colonnes, tables de liens et de composants) |
| `querycases/<nom>.yaml` | Cas du mode `query` : `seed` (lignes insérées par Strapi), puis `cases` (`uid`, `op` parmi `findMany`, `findOne`, `count`, et `params` de `db.query` : `where`, `orderBy` en liste d'objets à une clé, `limit`, `offset`, `select`, `_q`). Le rapport liste les cas à résultat vide, qui prouvent moins |
| `corpus/<nom>.yaml` | Cas exécutés dans l'ordre. Champs : `name`, `method`, `path`, `query` (en clair, les valeurs sont encodées par le runner), `token` (`full` ou `read`), `bearer` (jeton brut, ex. `{{jwt}}` tiré d'un login), `body`, `files` (PNG générés envoyés en multipart, ex. `[{ name: a.png, width: 800, height: 600 }]`), `save` (variables tirées de la réponse, ex. `data.documentId`, ou d'un cookie, `cookie:<nom>`), `headers` (en-têtes envoyés, avec `{{var}}`), `cookies` (compare les `Set-Cookie` : nom, attributs, effacement, sans la valeur), `skip` |
| `normalize/` | `id` et `documentId` remplacés par des alias stables (ordre d'apparition), horodatages par `<ts>`, secrets masqués. Un documentId déjà vu qui réapparaît dans un texte (message d'erreur) est remplacé par son alias. L'ordre des clés est ignoré, celui des tableaux non |
| `cmd/compat/` | `compat run` (un moteur) et `compat diff` (deux sorties) |
| `tools/` | Générateurs lancés dans l'image de référence : tables de slugify, golden de nommage, et `gen-builtin-schemas.js`, qui extrait vers `internal/builtin/schemas` les content types et modèles internes que Strapi embarque (points d'entrée Community Edition uniquement) |
| `fixtures/<nom>/framework/`, `fixtures/<nom>/rules.js` | Une fixture qui reproduit le code d'un projet : `rules.js` est chargé par le bootstrap de la référence (`COMPAT_ROW_RULES=1`), `framework/` est son portage Go (un `main` sur le module grapi), compilé par `bench.sh` en `bin/grapi-<nom>` et servi à la place de `grapi`. En mode `query`, un fichier de cas avec `hooks: true` fait tourner ce code des deux côtés : grapi sert alors les cas `doc*` depuis une `App` complète qui enregistre le package Go de la fixture (`compat/cmd/compat/hooks.go`). devfreelance y porte les règles de ligne et les middlewares de documents de dev-freelance (`devfreelance_rules`, `devfreelance--rules`), plus des routes de banc qui lèvent `PolicyError` et `NotImplementedError`, que Strapi ne lève que depuis le code d'un projet (`devfreelance_errors`). |
| `scripts/typecheck.sh` (`make typecheck-compat`) | Les réponses de Strapi enregistrées pour devfreelance (goldens, ids et dates restaurés) typées avec les types TypeScript que grapi génère, compilées par `tsc` dans l'image de référence sur le homelab. |
| `scripts/` | `bench.sh` (côté Mac : cross-compile, rsync, ssh) et `remote.sh` (côté homelab : instances jetables, nettoyage garanti à la sortie). Modes : `self` (Strapi contre Strapi, écrit les golden), `roundtrip` (la synchro de grapi doit répondre `UNCHANGED` sur une base créée par Strapi, et Strapi démarré sur une base créée par grapi doit répondre au corpus comme sur la sienne : `make bench-compat FIXTURE=basic MODE=roundtrip DB=postgres`), `query` (le fichier `querycases/<fixture>.yaml` insère ses lignes via `db.query(uid).create` de Strapi, puis chaque cas passe par `db.query(uid)` de Strapi et par le moteur de grapi sur la même base ; les résultats doivent être identiques), `grapi` (à venir) |

## Écarts connus de Strapi selon le dialecte

Strapi ne répond pas pareil selon la base (vu sur la fixture `basic`) :

- `biginteger` : SQLite perd la précision au-delà de 2^53 (`9007199254740993` revient en `9007199254740992`), Postgres non.
- `time` : `14:30:00.000` sous SQLite, `14:30:00` sous Postgres.
- `$containsi` (fixture `devfreelance`) : SQLite ne replie pas la casse hors ASCII (`développeur` ne trouve pas `DÉVELOPPEUR`), Postgres si.
- i18n (fixture `i18n-dp`) : quand un `PUT ?locale=xx` crée une nouvelle locale, les champs non localisés recopiés viennent d'une ligne choisie sans `ORDER BY` ; SQLite et Postgres ne prennent pas la même, d'où des valeurs `views`/`code` différentes sur les brouillons.

D'où un golden par dialecte. La position de grapi sur ces écarts (les reproduire ou corriger la perte de précision) se décide dans `DB-001`.
