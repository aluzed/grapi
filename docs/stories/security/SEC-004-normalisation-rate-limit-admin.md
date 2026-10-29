---
status: in-progress
---

# SEC-004 : Partager le compteur de connexion entre les variantes d'un e-mail

Priorité : **Moyenne**. Source : [revue complémentaire du 29 octobre 2026](../../security-review-20261029.md).

## Contexte

L'authentification supprimait les espaces de l'e-mail, mais la clé de limitation ne le faisait pas. Des variantes avec espaces contournaient le quota du même compte.

## Objectif

En tant que propriétaire d'un compte admin, je veux que toutes les variantes normalisées de mon e-mail partagent le même compteur afin de limiter les essais de mot de passe.

## Périmètre

- Utiliser la même normalisation pour l'authentification et la clé du middleware admin.
- Couvrir les autres routes qui utilisent ce middleware et conserver la séparation par route et par IP.

## Hors périmètre

Compteur distribué Redis et limitation globale de l'API.

## Critères d'acceptation

- [ ] Après épuisement du quota, les variantes de casse et d'espaces du même e-mail renvoient 429.
- [ ] Un autre compte ou une autre IP conserve le comportement prévu par la configuration.
- [ ] Le format d'erreur et les en-têtes de limitation existants sont conservés.

## Notes techniques

Le patch appelle `normalizeEmail` au lieu de `strings.ToLower`. `TestAdminLoginRateLimit` a été étendu ; réussite locale signalée dans le rapport sauvegardé.

## Référence Strapi

Middleware admin de rate limiting ; compléter `AUTH-006` sans changer les quotas par défaut.

## Dépendances

`AUTH-006`, `SEC-015`.

## Fichiers

- `internal/adminapi/ratelimit.go`
- `internal/adminapi/auth.go`
- `admin_test.go`

