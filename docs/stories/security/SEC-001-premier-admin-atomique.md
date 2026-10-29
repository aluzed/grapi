---
status: in-progress
---

# SEC-001 : Garantir une seule inscription du premier super administrateur

Priorité : **Haute**. Source : [revue complémentaire du 29 octobre 2026](../../security-review-20261029.md).

## Contexte

Sur une base vide, le contrôle d'absence d'administrateur précédait la création sans verrou commun. Le rapport complémentaire décrit huit super administrateurs créés par huit requêtes concurrentes.

## Objectif

En tant qu'exploitant, je veux que l'initialisation crée un seul super administrateur afin d'empêcher une prise de contrôle pendant le bootstrap.

## Périmètre

- Sérialiser le contrôle et la création dans une transaction SQLite et PostgreSQL.
- Conserver les réponses normales de l'inscription initiale et refuser les autres inscriptions.

## Hors périmètre

Invitations et création volontaire d'autres administrateurs après initialisation.

## Critères d'acceptation

- [ ] Huit inscriptions simultanées produisent exactement un succès et un seul administrateur persistant.
- [ ] Les autres requêtes échouent proprement et aucune relation de rôle orpheline n'est créée.
- [ ] La régression est validée sur SQLite et PostgreSQL, y compris avec plusieurs instances partageant la base.

## Notes techniques

Le correctif sauvegardé utilise une transaction et un verrou PostgreSQL sur la ligne du rôle super admin. `TestAdminFirstRegistrationIsAtomic` existe ; le rapport indique une validation locale, mais PostgreSQL et le détecteur de courses restent à vérifier. Ne pas clôturer sur la seule présence du patch.

## Référence Strapi

Parcours d'inscription initiale de l'admin ; aucune modification du contrat de la Content API.

## Dépendances

`AUTH-007`, `DB-001`, validation finale `SEC-015`.

## Fichiers

- `internal/adminapi/auth.go`
- `admin_registration_security_test.go`
- `admin_pg_integration_test.go`

