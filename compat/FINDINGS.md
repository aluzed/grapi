# Comportements de Strapi 5.59 observés sur le banc

Constats tirés des fixtures et des golden (`compat/golden/`). Ils font partie de la spécification : grapi les reproduit, sauf décision contraire notée dans `docs/divergences.md`. Chaque ticket concerné doit s'y référer.

## Écriture et validation

- Un `id` ou un `documentId` dans le body d'un update est **refusé** (400 `Invalid key id`), pas ignoré.
- `maxLength` est inclusif.
- Les brouillons ne vérifient pas `required`, même à l'intérieur des composants. La vérification a lieu à la publication.
- `[]` passe le `min` d'un composant répétable non requis, mais `null` renvoie 400.
- Le contrôle de `__component` porte sur **tous** les composants : un composant non autorisé dans une zone dynamique est accepté (201), puis supprimé sans rien dire.
- À la création, un `id` de composant appartenant à une autre entrée est ignoré et un nouveau composant est créé. À l'update, le même cas renvoie 400.
- Un update de composant sans son `id` le remplace, et les données imbriquées non envoyées sont perdues.
- Les composants d'un tableau répétable imbriqué sont créés en parallèle : leurs ids ne suivent pas l'ordre du tableau.

## Draft & publish

- La création REST publie par défaut.
- Un PUT sans `status` publie tout le brouillon, y compris les modifications antérieures restées en brouillon. `status=archived` est lu comme `draft`.
- Chaque publication crée une nouvelle ligne, donc un nouvel `id`, pour la version publiée.
- Il est impossible de dépublier via l'API REST de contenu :
  - `DELETE ?status=draft` renvoie 500 ;
  - `publishedAt: null` est ignoré.
- Un document publié qui pointe vers une cible jamais publiée affiche `null`. Publier la cible rétablit le lien.
- `hasPublishedVersion=false` renvoie une liste vide.

## Suppression

- Supprimer deux fois le même document renvoie 204 les deux fois.
- `DELETE ?locale=*` sur un document déjà supprimé renvoie 204.

## i18n

- La locale n'est quasiment pas validée : `POST ?locale=it`, et même `?locale=not a locale!`, créent des entrées dans cette « locale ».
- Une `locale` placée dans le body est ignorée.
- `GET` d'un élément ou d'un single type avec `locale=*` renvoie 404.
- Synchronisation partielle des champs non localisés :
  - elle n'atteint que les entrées de même statut ;
  - quand un PUT crée une nouvelle locale, les valeurs copiées viennent d'une ligne prise sans ordre, d'où des valeurs différentes entre SQLite et Postgres.
- Connecter une cible qui n'a pas de version dans la locale renvoie 400 `Document with id …, locale fr not found`.
- `populate=localizations` sur un type non localisé renvoie 400.

## Relations

- Une ancre de position invalide produit un message d'erreur qui expose des ids internes et `"locale":"en"`, même sans i18n.
- Un documentId inconnu renvoie 400 avec `locale "null"`.
- Un filtre sur une relation many-to-many renvoie des lignes distinctes.
- Relations morph :
  - morphToMany fonctionne en REST (`{documentId, __type}`, populate avec `on` et `count`) ;
  - filtrer sur une relation morph renvoie 400 ;
  - morphToOne ne peut pas s'écrire en REST : un objet donne 400 `Invalid key`, un tableau donne 500 ;
  - écrire depuis le côté inverse morphMany donne 500, et le populate inverse vaut toujours `[]`.

## Lecture

- `populate[dz]=*` renvoie 400 : il faut des fragments `on` ou `[populate]=*`.
- Un fragment `on` retire les entrées de zone dynamique des composants qu'il ne liste pas.
- `fields` sur un champ privé renvoie 400 `Invalid key`.
- `$containsi` ne gère pas la casse des caractères non ASCII sous SQLite.
- La réponse d'upload contient `isUrlSigned`, mais pas les médias peuplés.
- Une route POST inconnue renvoie 405.

## Différences entre dialectes

- `biginteger` perd sa précision au-delà de 2^53 sous SQLite (`9007199254740993` revient en `9007199254740992`).
- `time` vaut `14:30:00.000` sous SQLite et `14:30:00` sous Postgres.
- `decimal` devient `float` dans le DDL SQLite écrit par knex.
- Trier sur un champ de composant place les NULL en premier sous SQLite et en dernier sous Postgres.
- Postgres consomme des valeurs de séquence sur les créations en échec, donc les ids divergent entre les deux dialectes.
- `unique: true` ne crée aucune contrainte en base : l'unicité est vérifiée par l'application.
- Le DDL SQLite écrit par knex duplique les contraintes de clé étrangère.

## Sécurité (à vérifier sur les projets réels)

- Si le rôle Public a `users-permissions user.find`, peupler une relation vers un utilisateur expose son e-mail dans l'API publique.

## Limites connues du banc

- Les ids sont remplacés par des alias selon leur valeur brute, toutes tables confondues : writer 1, profile 1 et seo 1 deviennent tous `#id1`. En mode grapi, cela peut masquer ou créer des DIFF. À affiner avant la comparaison avec grapi : alias par table, déduite du contexte JSON.
