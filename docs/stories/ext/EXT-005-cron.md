---
status: todo
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
- [ ] Une tâche à date unique ne s'exécute qu'une fois.
- [ ] `server.cron.enabled: false` désactive tout.

## Notes techniques
- `robfig/cron/v3` avec `cron.WithSeconds()` et un parseur pour les objets de récurrence.
- Sur plusieurs instances, Strapi exécute la tâche partout. Proposer une option `lock: db` (verrou applicatif en base) en amélioration.

## Référence Strapi
- `core/core/src/services/cron.ts`, `core/core/src/providers/cron.ts`

## Dépendances
- `EXT-001`, `EXT-002`

## Fichiers
- `internal/cron/cron.go`, `internal/cron/rules.go`
