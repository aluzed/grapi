# Divergences assumées avec Strapi 5.59

grapi reproduit les comportements observables de Strapi. Ce document liste les endroits où il s'en écarte volontairement, et pourquoi.

## Validation des schémas (`schema.Validate`)

Tout ce que Strapi refuse au démarrage est une **erreur** dans grapi, avec le même message autant que possible. Les contrôles ajoutés par grapi sont des **avertissements**, jamais des erreurs : Strapi accepte ces schémas, et les refuser empêcherait d'ouvrir un projet existant (décision D3).

| Code | Contrôle ajouté | Ce que fait Strapi |
|---|---|---|
| `inverse-missing` (via `mappedBy`) | l'attribut désigné par `mappedBy` existe sur la cible | ne vérifie que `inversedBy` |
| `inverse-mismatch` | l'inverse pointe en retour, avec le type de relation attendu | aucun contrôle |
| `unidirectional-inverse` | `oneWay` et `manyWay` sans `inversedBy` ni `mappedBy` | aucun contrôle |
| `component-bidirectional-relation` | relation unidirectionnelle dans un composant | aucun contrôle au chargement |
| `uid-target` | `targetField` existe et est `string` ou `text` | aucun contrôle au chargement |
| `bounds` | `min <= max`, `minLength <= maxLength` | aucun contrôle |
| `repeatable-bounds` | `min` et `max` seulement sur un composant répétable | ignorés |
| `enum-default` | le défaut d'une énumération fait partie des valeurs | aucun contrôle |
| `media-allowed-types` | `allowedTypes` parmi `images`, `videos`, `files`, `audios` | aucun contrôle |
| `localized-attribute` | pas d'attribut localisé dans un type non localisé | ignoré |
| `identifier-too-long` | nom de plus de 55 caractères, qui sera tronqué en base | tronque sans prévenir |

Deux erreurs sont plus tôt que chez Strapi, sans changer le verdict : un cycle de composants (`component-cycle`) et une zone dynamique dans un composant sont refusés au chargement, alors que Strapi échoue plus tard.

## API et serveur

- Aucun en-tête `X-Powered-By`, et `/_health` n'envoie pas l'en-tête `strapi` : le serveur ne se signale pas (`API-008`, `API-001`).
- Webhooks : nouvelles tentatives par défaut ; Strapi n'en fait aucune (`EXT-006`).
- Archives d'export natives en AES-256-GCM ; Strapi chiffre en AES-128-ECB, format que grapi sait lire et, sur demande, écrire (`CLI-004`).

## À trancher

- `biginteger` sous SQLite : Strapi perd la précision au-delà de 2^53 (voir `compat/README.md`). Le reproduire ou non se décide dans `DB-001`.
