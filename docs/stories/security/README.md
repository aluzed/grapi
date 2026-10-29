# Stories de sécurité du 29 octobre 2026

Sources récupérées après le crash : [audit initial](../../security-audit-20261029.md) et [revue complémentaire](../../security-review-20261029.md). Les tickets ci-dessous transforment les constats sauvegardés en travail vérifiable ; leur création n'est pas un nouveau scan.

L'audit initial recense 15 failles déjà corrigées dans l'historique Git. La revue complémentaire décrit 6 problèmes reproduits : 3 de gravité haute et 3 de gravité moyenne. Les patches et les tests correspondants sont encore non commités à la création de ce backlog. Leurs stories restent donc `in-progress`, avec des critères à vérifier avant clôture. Les résultats antérieurs sont attribués au rapport ; ils n'ont pas été rejoués pendant la rédaction des tickets.

Les recommandations de configuration deviennent des stories `todo` propres au projet déployé. Leur priorité reflète le risque si les conditions décrites sont réunies ; elle ne prouve pas une exposition actuelle en production. La revue complémentaire n'a interrogé ni production ni backend privé.

| Ticket | Priorité | Statut initial | Objet |
|---|---|---|---|
| [SEC-001](./SEC-001-premier-admin-atomique.md) | Haute | `in-progress` | Garantir une seule inscription du premier super administrateur |
| [SEC-002](./SEC-002-historique-droits-document-locale.md) | Haute | `in-progress` | Appliquer les droits par document et par langue à l'historique |
| [SEC-003](./SEC-003-upload-contournement-mime.md) | Haute | `in-progress` | Empêcher le contournement des types interdits à l'upload |
| [SEC-004](./SEC-004-normalisation-rate-limit-admin.md) | Moyenne | `in-progress` | Partager le compteur de connexion entre les variantes d'un e-mail |
| [SEC-005](./SEC-005-historique-champs-supprimes.md) | Moyenne | `in-progress` | Protéger les valeurs historiques des champs supprimés |
| [SEC-006](./SEC-006-ssrf-plages-non-publiques.md) | Moyenne | `in-progress` | Bloquer les plages non publiques oubliées dans les connexions sortantes |
| [SEC-007](./SEC-007-politique-upload-production.md) | Haute | `todo` | Configurer une politique d'upload explicite en production |
| [SEC-008](./SEC-008-webhooks-protection-connexion.md) | Haute | `todo` | Activer la protection réseau des webhooks du projet |
| [SEC-009](./SEC-009-cors-origines-explicites.md) | Moyenne | `todo` | Restreindre CORS aux origines utilisées par les applications |
| [SEC-010](./SEC-010-permissions-utilisateurs-sensibles.md) | Haute | `todo` | Restreindre les modifications sensibles des utilisateurs finaux |
| [SEC-011](./SEC-011-expiration-jetons-revocation-sessions.md) | Moyenne | `todo` | Définir l'expiration des jetons et la révocation après changement de mot de passe |
| [SEC-012](./SEC-012-graphql-limites.md) | Moyenne | `todo` | Borner les requêtes GraphQL du projet |
| [SEC-013](./SEC-013-proxy-confiance-ip.md) | Moyenne | `todo` | Fiabiliser l'IP client derrière le proxy de confiance |
| [SEC-014](./SEC-014-budgets-ressources-production.md) | Moyenne | `todo` | Adapter les limites de ressources au déploiement |
| [SEC-015](./SEC-015-validation-correctifs-securite.md) | Haute | `todo` | Valider les correctifs sauvegardés avant livraison |

## Ordre de traitement

1. Terminer la revue des patches `SEC-001` à `SEC-006`, puis consigner leur validation Linux/PostgreSQL dans `SEC-015` avant livraison.
2. Vérifier les configurations et permissions réelles : uploads (`SEC-007`), webhooks (`SEC-008`), utilisateurs (`SEC-010`), CORS (`SEC-009`) et proxy (`SEC-013`).
3. Traiter les politiques de jetons (`SEC-011`), les limites GraphQL si activé (`SEC-012`) et les budgets de ressources (`SEC-014`).

Les contrôles PostgreSQL utilisent une base de test dédiée. Les limites et preuves de la revue restent détaillées dans le rapport source.

