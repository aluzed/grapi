# SCH-005 : Registres namespacés

## Contexte
Strapi range chaque élément sous un uid namespacé : `api::article.article`, `plugin::upload.file`, `admin::user`, `global::is-owner`, `strapi::cors`. Toute la résolution (routes vers controllers, policies, middlewares) passe par ces uids. Le mode framework Go (`EXT-002`) enregistre ses éléments dans ces mêmes registres.

## Objectif
Fournir des registres typés, thread-safe après le démarrage, avec la même syntaxe d'uid et les mêmes règles de résolution que Strapi.

## Périmètre
- **Registres** : `contentTypes`, `components`, `services`, `controllers`, `policies`, `middlewares`, `hooks` (`EXT-008`), `models` (tables internes `strapi::*`), `plugins`, `customFields` (`SCH-006`), `apis`, `sanitizers`, `validators`.
- **Format d'uid** :
  - `<namespace>::<nom>[.<sous-nom>]`, avec les namespaces `api`, `plugin`, `admin`, `global`, `strapi` ;
  - les composants utilisent `<catégorie>.<nom>`, sans namespace ;
  - parseur et formateur dans `schema/uid.go`, avec validation.
- **Opérations** : `Add(uid, v)` (erreur si l'uid existe déjà), `Get`, `Has`, `GetAll(namespacePrefix)`, `Extend(uid, func(old) new)` (enveloppe un service ou un controller existant).
- **Fabriques paresseuses** : services et controllers sont enregistrés comme `func(*App) T` et instanciés au premier `Get`. Cela permet les dépendances circulaires entre services, comme chez Strapi.
- **Résolution des noms courts**, pour les policies et les middlewares :
  1. l'uid complet tel quel ;
  2. sinon `plugin::<plugin courant>.<nom>` ;
  3. sinon `api::<api courante>.<nom>` ;
  4. sinon `global::<nom>`.
  Une erreur explicite liste les candidats essayés.
- **Gel** : après `bootstrap`, toute écriture dans un registre (sauf le rechargement à chaud contrôlé) panique en dev et renvoie une erreur en prod.
- **Introspection**, pour `CLI-003` (`*:list`) : liste triée des uids par registre.

## Hors périmètre
- Le contenu de chaque registre (traité par les tickets concernés).

## Critères d'acceptation
- [ ] `ParseUID` accepte tous les uids des fixtures et rejette les formes invalides (`api:foo`, `plugin::`, majuscules).
- [ ] Ajouter deux fois le même uid renvoie une erreur.
- [ ] `Extend` sur un controller conserve l'accès à l'implémentation d'origine (équivalent de `super`).
- [ ] La résolution de policy `is-owner` depuis une route de l'API `post` trouve `api::post.is-owner` avant `global::is-owner` (test en table sur les 4 niveaux).
- [ ] Les tests de concurrence (`-race`) passent : lectures parallèles après le gel.
- [ ] `grapi content-types:list` sur une fixture donne la même liste triée que `strapi content-types:list`.

## Notes techniques
- Les génériques Go (`Registry[T]`) évitent un registre par type.
- Le gel évite des verrous sur le chemin chaud : `sync.Map` ou une map en lecture seule après le gel, sans mutex.
- Le rechargement à chaud (CTB) reconstruit un nouveau jeu de registres, puis bascule atomiquement (`atomic.Pointer`). On ne mute jamais un registre en place.

## Référence Strapi
- `core/core/src/registries/` (un fichier par registre)
- `core/core/src/providers/registries.ts`
- `core/utils/src/policy.ts` (résolution des noms de policies)

## Dépendances
- `SCH-001`.

## Fichiers
- `schema/uid.go`, `internal/registry/registry.go`, `internal/registry/{contenttypes,services,controllers,policies,middlewares}.go`, `internal/registry/registry_test.go`
