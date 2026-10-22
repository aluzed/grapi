# Banc de compatibilité

Envoie le même corpus de requêtes à Strapi 5.59 (référence) et à un second moteur, normalise les réponses et produit un diff. Tout tourne sur le homelab (`COMPAT_HOST`, `homelab` par défaut) : rien n'est lancé en local.

```sh
make bench-compat FIXTURE=basic                  # Strapi contre Strapi, SQLite
make bench-compat FIXTURE=basic DB=postgres
make bench-compat FIXTURE=basic MODE=grapi       # Strapi contre grapi (quand grapi sait servir)
```

Le rapport arrive dans `compat/report/<date>-<fixture>-<mode>-<db>.md` (non versionné). Les sorties normalisées de la référence sont versionnées dans `compat/golden/<fixture>-<db>/` : elles servent de spécification lisible, et un diff git montre ce qui change quand on monte de version de Strapi.

## Contenu

| Chemin | Rôle |
|---|---|
| `reference/` | Image Strapi de référence (versions figées dans `versions.env`). Les fixtures ne contiennent que des `schema.json` : l'entrypoint génère les routes, controllers et services standards. Le bootstrap donne `find`/`findOne` au rôle Public et écrit deux tokens (`full`, `read`) dans `/data/tokens.json` |
| `fixtures/<nom>/src/` | Projets de test, lus tels quels par les deux moteurs |
| `corpus/<nom>.yaml` | Cas exécutés dans l'ordre. Champs : `name`, `method`, `path`, `query` (en clair, les valeurs sont encodées par le runner), `token` (`full` ou `read`), `body`, `save` (variables tirées de la réponse, ex. `data.documentId`), `skip` |
| `normalize/` | `id` et `documentId` remplacés par des alias stables (ordre d'apparition), horodatages par `<ts>`, secrets masqués. L'ordre des clés est ignoré, celui des tableaux non |
| `cmd/compat/` | `compat run` (un moteur) et `compat diff` (deux sorties) |
| `scripts/` | `bench.sh` (côté Mac : cross-compile, rsync, ssh) et `remote.sh` (côté homelab : instances jetables, nettoyage garanti à la sortie) |

## Écarts connus de Strapi selon le dialecte

Strapi ne répond pas pareil selon la base (vu sur la fixture `basic`) :

- `biginteger` : SQLite perd la précision au-delà de 2^53 (`9007199254740993` revient en `9007199254740992`), Postgres non.
- `time` : `14:30:00.000` sous SQLite, `14:30:00` sous Postgres.

D'où un golden par dialecte. La position de grapi sur ces écarts (les reproduire ou corriger la perte de précision) se décide dans `DB-001`.
