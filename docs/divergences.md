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

## Base de données : synchronisation du schéma (`internal/db/schemasync`)

- **Tables des fonctions que grapi ne reproduit pas** (`strapi_release*`, `strapi_workflows*`, `strapi_history_*`, `strapi_ai_*`) : elles ne sont jamais supprimées, même avec `forceMigration` (actif par défaut chez Strapi). Sans cela, ouvrir une base Strapi avec grapi détruirait les releases, les workflows et l'historique. Strapi les supprimerait si leur fonction disparaissait du projet.
- **Hash du schéma stocké** : grapi ajoute à son schéma les tables de ces fonctions telles que le schéma stocké en base les décrit (jamais depuis le code de Strapi). Le hash coïncide alors avec celui de Strapi : ouvrir une base Strapi ne réécrit rien, et Strapi retrouve son schéma au redémarrage.
- **Index modifié sans `forceMigration`** : Strapi ne supprime pas l'ancien index mais tente de recréer le nouveau, et la synchro échoue (« already exists »). grapi garde l'ancien index et le signale dans les logs.
- **Changement de type d'une colonne sous SQLite** : comme knex, grapi reconstruit la table (nouvelle table, copie, échange). knex régénère tout le `CREATE TABLE` ; grapi ne réécrit que les colonnes modifiées et garde le reste du texte tel quel. La structure obtenue est la même.
- **Suppression d'une colonne sous SQLite** : `ALTER TABLE ... DROP COLUMN` natif, là où knex reconstruit la table.

## Base de données : lecture (`internal/db/query`)

- **`biginteger` au-delà de 2^53 sous SQLite** : Strapi relit la valeur via un nombre JavaScript et la tronque (`9007199254740993` revient en `9007199254740992`). grapi relit la valeur exacte stockée en base.
- **Filtre sur une colonne brute** : Strapi accepte en interne un nom de colonne qui n'est pas un attribut. grapi n'accepte que les colonnes qui existent dans la table du modèle ; tout autre nom est refusé.

## Base de données : écritures (`internal/db/query/em.go`)

- **Transaction** : une écriture et ses relations sont dans une seule transaction. Strapi insère la ligne, puis rattache les relations dans une seconde transaction et supprime la ligne à la main si cela échoue.

## Document Service : composants (`internal/document/components.go`)

- **Ordre des ids des composants frères** : Strapi crée les entrées d'un composant répétable en parallèle (`p-map`). Sous SQLite, l'ordre des insertions suit alors la profondeur d'imbrication de chaque entrée : une colonne qui contient plus de composants imbriqués est insérée après sa sœur et reçoit un id plus grand. grapi les crée dans l'ordre de la liste. Les ids diffèrent, pas l'ordre des entrées dans la réponse (banc : `components / 024`, `025` en mode grapi).

## Bootstrap (`internal/bootstrap`)

- **Permissions des actions inconnues** : Strapi supprime au démarrage les permissions dont l'action ne correspond à aucun contrôleur. grapi ne supprime rien : il ne voit pas encore les contrôleurs JS custom, et la purge détruirait leurs permissions sur une base Strapi.
- **Réglage `grant`** : Strapi le réécrit à chaque démarrage dès qu'il diffère des valeurs par défaut ; grapi seulement quand la fusion change la valeur stockée.

## Médias (`internal/media`, `internal/httpapi/upload.go`)

- **Octets des images** : sharp (libvips) et l'encodeur Go ne produisent pas les mêmes fichiers. Les dimensions de l'original et des formats sont identiques (même calcul `fit: inside` que sharp), les tailles (`size`, `sizeInBytes`) ne le sont pas. Le banc masque ces deux champs sur les objets image.
- **Optimisation PNG** : avec `sizeOptimization`, sharp ré-encode un PNG en palette (libimagequant, qualité 80). grapi ré-encode en compression maximale sans quantification et garde l'original si le résultat est plus gros, ce qui est le cas le plus fréquent.
- **Build Go pur** (en attendant libvips, décision D4) : pas de formats pour les WebP (pas d'encodeur WebP en Go), pas d'orientation EXIF (`autoOrientation`), AVIF et SVG sont reconnus mais ni redimensionnés ni optimisés, comme chez Strapi pour le SVG.
- **Dossier `public/uploads` absent** : Strapi refuse de démarrer ; grapi le crée au premier upload.

## API et serveur

- Aucun en-tête `X-Powered-By`, et `/_health` n'envoie pas l'en-tête `strapi` : le serveur ne se signale pas (`API-008`, `API-001`).
- Webhooks : nouvelles tentatives par défaut ; Strapi n'en fait aucune (`EXT-006`).
- users-permissions en `jwtManagement: refresh` : un access token cesse d'être accepté dès que sa session est révoquée (déconnexion, révocation, changement de mot de passe). Strapi ne vérifie que la signature et l'accepte jusqu'à son expiration (10 minutes par défaut) (`AUTH-003`).
- Sessions (admin et users-permissions) : rejouer un refresh token déjà tourné rend la même suite pendant 1 minute (onglets concurrents), puis révoque l'appareil ; Strapi rend la suite sans limite de temps (`AUTH-003`).
- users-permissions en `jwtManagement: refresh` : la confirmation d'e-mail qui renvoie l'utilisateur répond un vrai access token ; Strapi y sérialise une promesse (`"jwt": {}`) (`AUTH-003`).
- Archives d'export natives en AES-256-GCM ; Strapi chiffre en AES-128-ECB, format que grapi sait lire et, sur demande, écrire (`CLI-004`).

## GraphQL (`internal/graphql`, `internal/httpapi/graphql.go`)

- Pas de types `ReviewWorkflows*` : review workflows est une fonction Enterprise que grapi conçoit à part (clean-room).
- Un champ morphToMany (`mentions: [GenericMorph]`) répond chez Strapi 5.59 `null` avec une `INTERNAL_SERVER_ERROR` ; grapi renvoie les entrées liées avec leur `__typename` (corpus `relations_graphql`, cas en skip).
- Pas de page d'accueil Apollo (sandbox) sur `GET /graphql` : grapi répond comme une requête GET sans en-tête, bloquée par la protection CSRF.

## OpenAPI (`internal/openapi`)

- Pas de `x-powered-by` ni de `x-strapi-version` dans le document : le serveur ne se signale pas.
- Les routes de plugins que grapi ne sert pas encore (content API du Content-Type Builder, rôles, sessions, OAuth) ne sont pas listées.

## Content-Type Builder (`internal/httpapi/ctb.go`)

- `GET /api/content-type-builder/content-types` ne liste pas les types Enterprise (content-releases, review-workflows).
- `GET /api/content-type-builder/components` liste les composants dans l'ordre du chargement de grapi (catégories et fichiers triés) ; Strapi suit l'ordre du système de fichiers.

## À trancher

- `biginteger` sous SQLite : Strapi perd la précision au-delà de 2^53 (voir `compat/README.md`). Le reproduire ou non se décide dans `DB-001`.
