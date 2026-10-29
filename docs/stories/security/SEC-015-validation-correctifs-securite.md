---
status: in-progress
---

# SEC-015 : Valider les correctifs sauvegardés avant livraison

Priorité : **Haute**. Source : [revue complémentaire du 29 octobre 2026](../../security-review-20261029.md).

## Contexte

Le crash a laissé les correctifs de la revue complémentaire dans l'arbre de travail. Le rapport consigne des succès ciblés, mais la suite complète n'est pas verte sous Windows et PostgreSQL, le détecteur de courses et le déploiement réel n'ont pas été validés.

## Objectif

En tant que mainteneur, je veux une validation reproductible des correctifs afin de préparer une livraison dont les limites sont connues.

## Périmètre

- Rejouer les régressions sur un environnement supporté Linux, avec SQLite et une base PostgreSQL dédiée.
- Vérifier les courses, la compilation, l'admin et les parcours manuels sensibles.
- Actualiser les résultats de dépendances avec le compilateur et les lockfiles de livraison.

## Hors périmètre

Déploiement en production et déclaration de correction d'échecs préexistants sans preuve.

## Critères d'acceptation

- [ ] Les régressions `SEC-001` à `SEC-006` sont rejouées et leurs résultats associés à la révision testée.
- [ ] Les suites Go, l'intégration PostgreSQL et le détecteur de courses sont exécutés sur Linux ; tout échec est analysé par rapport à la base.
- [ ] La création concurrente du premier admin et les accès à l'historique sont validés sur PostgreSQL dédié ; les paquets d'intégration sont exécutés en série.
- [ ] L'admin passe typecheck/tests et une recette couvre édition avec relations, sélecteurs, upload et historique sous un rôle restreint.
- [ ] Les contrôles de dépendances, la version du compilateur, les résultats et les limitations restantes sont consignés ; les stories ne sont clôturées qu'après preuve de leurs critères.

## Notes techniques

Commandes existantes : `go test ./...`, `go test -race ./...`, `go test -p 1 -tags integration ./...` avec `GRAPI_TEST_PG_DSN` pointant exclusivement sur une base de test. Les tests d'intégration réinitialisent cette base. Le rapport sauvegardé distingue les échecs Windows et les références typegen préexistantes. Il signale des audits de dépendances sans vulnérabilité utilisée à la date du contrôle ; ce n'est pas un nouveau résultat de scan.

## Référence Strapi

Recette du moteur grapi et banc de compatibilité `SETUP-002` ; ne pas consulter les sources Enterprise Strapi.

## Dépendances

Patches de `SEC-001` à `SEC-006` disponibles. Cette story centralise la preuve de validation nécessaire à leur clôture.

## Fichiers

- `docs/security-review-20261029.md`
- `docs/security-audit-20261029.md`
- `Makefile`
- `admin_pg_integration_test.go`
- `admin_registration_security_test.go`
- `admin_history_security_test.go`

## Avancement

- 29 octobre 2026, Windows 11, Go 1.27.0, révision de travail après `ee637d4`. `go build ./...` et `go vet ./...` réussis, avec l'overlay qui exclut `ctb_fixture_test.go` et `develop_test.go` (`syscall.Kill`).
- `go test ./...` : tout passe sauf les échecs Windows connus. `TestGracefulShutdown` et `TestUnixSocket` (vérifiés identiques sur la base sans les changements), `TestSuperviseRebuildsOnGoChanges`, `TestGenerateGoSkeletons` et `internal/typegen/TestMatchesStrapi` (déjà en échec avant les correctifs).
- Les régressions de `SEC-001` à `SEC-006` passent sur SQLite sous Windows.
- Reste tout le périmètre Linux : suite complète, `-race`, intégration PostgreSQL dédiée en série, typecheck et tests de l'admin, recette manuelle, audits de dépendances. Ni WSL ni Docker ni PostgreSQL ne sont disponibles sur ce poste.
