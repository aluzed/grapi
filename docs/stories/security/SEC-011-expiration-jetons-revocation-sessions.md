---
status: in-progress
---

# SEC-011 : Définir l'expiration des jetons et la révocation après changement de mot de passe

Priorité : **Moyenne**. Source : [revue complémentaire du 29 octobre 2026](../../security-review-20261029.md).

## Contexte

Les jetons de reset et de confirmation des utilisateurs n'ont pas de durée d'expiration ; les JWT legacy restent valides après changement de mot de passe, selon les rapports sauvegardés.

## Objectif

En tant qu'utilisateur, je veux que les liens sensibles expirent et que les sessions soient révoquées selon une politique claire afin de limiter l'usage d'anciens accès.

## Périmètre

- Définir les durées de validité et le comportement attendu après changement ou réinitialisation du mot de passe.
- Évaluer puis intégrer le mode `jwtManagement: refresh` aux clients concernés.
- Ajouter le durcissement explicite nécessaire pour les jetons de reset et confirmation, avec documentation de compatibilité.

## Hors périmètre

Bascule automatique du mode JWT pour tous les projets et révocation de comptes sans décision liée à un incident.

## Critères d'acceptation

- [x] Un jeton expiré ou déjà consommé ne peut pas être réutilisé ; les durées sont testées avec une horloge contrôlée.
- [x] Les sessions et jetons antérieurs au changement de mot de passe suivent la politique documentée.
- [ ] Le front sait renouveler et révoquer ses sessions si le mode refresh est retenu.
- [x] La migration précise le traitement des jetons déjà émis et les écarts à la compatibilité Strapi.

## Notes techniques

Le mode refresh n'implique pas à lui seul l'expiration des jetons de reset ou de confirmation. Tester ces mécanismes séparément.

## Référence Strapi

Comportements legacy conservés pour la compatibilité ; durcissement à rendre explicite dans le contrat applicatif.

## Dépendances

`AUTH-002`, `AUTH-003`.

## Fichiers

- `internal/httpapi/users.go`
- `internal/httpapi/users_email.go`
- `internal/httpapi/users_sessions.go`
- `up_sessions_test.go`
- `docs/configuration.md`

## Avancement

- Options grapi, désactivées par défaut : `plugins.users-permissions.config.resetPassword.expiresIn`, `emailConfirmation.expiresIn` et `jwt.revokeOnPasswordChange`. Une valeur invalide arrête le démarrage.
- L'expiration est portée par le code lui-même (`<hex>.<secondes Unix>`) : pas de nouvelle colonne, le schéma reste celui de Strapi. Un code expiré est refusé puis effacé ; un code sans expiration, émis avant l'option, est refusé quand l'option est active.
- En `legacy-support`, le JWT porte une empreinte HMAC du hash de mot de passe (`pwd`) : tout changement de mot de passe (change-password, reset, admin) le révoque. L'activation déconnecte une fois tous les utilisateurs. En mode `refresh`, le changement et la réinitialisation ferment déjà les sessions, comme Strapi.
- Tests : `TestTokenExpiry` et `TestPasswordStamp` (horloge fixe), `TestUsersTokenPolicy` (bout en bout : code à usage unique, codes expirés ou anciens refusés et effacés, JWT révoqués après reset et change-password, confirmation expirée), `TestUsersTokenPolicyInvalid`.
- Reste : décider du mode retenu par le projet déployé ; si c'est `refresh`, intégrer renouvellement et révocation au front.
