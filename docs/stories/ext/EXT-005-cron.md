---
status: in-progress
---

# EXT-005 : Cron

## Contexte
Strapi exécute des tâches planifiées déclarées dans `server.cron.tasks` (souvent importées depuis `config/cron-tasks`), activées par `server.cron.enabled`.

## Objectif
Le même planificateur, avec les mêmes formats de règle.

## Périmètre
- Formats : expression cron à 6 champs (secondes incluses) ou 5 champs, `Date` ou timestamp pour une exécution unique, objet de récurrence, et `{rule, tz, start, end}`.
- Déclaration : `{clé: {task, options}}` ou clé = règle.
- API : `app.Cron().Add(...)`, `Remove(nom)`, `Jobs()`.
- Tâches en Go (`EXT-002`) ou en JS (`EXT-003`).
- Une exécution ne démarre pas si la précédente de la même tâche tourne encore (option `allowOverlap` pour la forcer).
- Journalisation du début, de la fin, de la durée et des erreurs.

## Hors périmètre
- Exécution distribuée (verrou entre plusieurs instances) : voir les notes techniques.

## Critères d'acceptation
- [ ] `*/5 * * * * *` s'exécute toutes les 5 s ; avec `tz: Europe/Paris`, une règle horaire respecte le changement d'heure.
- [x] Une tâche à date unique ne s'exécute qu'une fois.
- [x] `server.cron.enabled: false` désactive tout.

## Avancement
- Fait : `internal/cron` (robfig/cron v3, secondes optionnelles, descripteurs `@daily`, fuseau par tâche, bornes start et end, pas de chevauchement sauf `AllowOverlap`, journalisation début, fin, durée et erreurs, paniques isolées), `grapi.App.Cron` et `CronAt`, actifs seulement avec `server.cron.enabled: true` comme chez Strapi. Tests : `internal/cron/cron_test.go`.
- Changement d'heure mesuré et figé (`TestDaylightSavingTime`, Europe/Paris) : une heure qui n'existe pas est sautée (02:30 le 29 mars 2026), une heure qui existe deux fois déclenche deux exécutions (02:30 le 25 octobre 2026). Documenté dans `docs/framework.md`.
- Reste : comparaison avec node-schedule sur les mêmes dates, objets de récurrence de node-schedule, `config/cron-tasks` (JS, `EXT-003`).

## Notes techniques
- `robfig/cron/v3` avec `cron.WithSeconds()` et un parseur pour les objets de récurrence.
- Sur plusieurs instances, Strapi exécute la tâche partout. Proposer une option `lock: db` (verrou applicatif en base) en amélioration.

## Référence Strapi
- `core/core/src/services/cron.ts`, `core/core/src/providers/cron.ts`

## Dépendances
- `EXT-001`, `EXT-002`

## Fichiers
- `internal/cron/cron.go`, `internal/cron/rules.go`
