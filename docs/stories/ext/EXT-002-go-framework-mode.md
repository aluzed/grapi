---
status: in-progress
---

# EXT-002 : Mode framework Go (grapi comme bibliothèque)

## Contexte
Chez Strapi, la logique métier d'un projet vit dans `src/index.ts` (register, bootstrap), `lifecycles.ts`, des controllers, services, policies et middlewares en JS/TS. En Go, la voie performante et typée consiste à importer grapi comme une bibliothèque et à enregistrer ce code dans un `main.go` compilé avec lui, comme PocketBase (décision D2, niveau 2).

## Objectif
Une API Go publique et stable qui couvre tous les points d'extension serveur de Strapi.

## Périmètre
- `grapi.New(opts)` puis `app.Start()`. Le binaire officiel `cmd/grapi` est lui-même un `main.go` sans extension.
- Phases : `OnRegister` (avant la synchro de la base : ajout de content types, custom fields, middlewares), `OnBootstrap` (base prête), `OnDestroy`.
- Lifecycles de ligne (`DB-009`) par modèle : `BeforeCreate`, `AfterCreate`, `BeforeUpdate`, ..., avec `event.Params.Data` modifiable.
- Middlewares du Document Service (`DOC-004`).
- Surcharge et ajout d'actions de controller, surcharge de service, routes custom (`API-002`, `API-006`).
- Policies et middlewares globaux nommés (`global::nom`).
- Tâches cron en Go (`EXT-005`).
- Accès aux services : `app.Documents(uid)`, `app.DB()`, `app.Config()`, `app.Log()`, `app.Events()`, `app.Plugin("email")`.
- Les deux modes cohabitent : un projet peut ne rien compiler (binaire officiel + déclaratif) ou tout compiler.

Esquisse, sur les besoins réels de dev-freelance :

```go
func main() {
	app := grapi.New(grapi.Options{Dir: "."})

	// Règles appliquées à chaque ligne écrite, donc aussi à la publication (DOC-002).
	app.OnRowCreate("api::post.post", "api::category.category", "api::medium.medium").
		Bind(func(e *hook.RowEvent) error {
			field := "title"
			if e.UID == "api::category.category" {
				field = "label"
			}
			e.Data["slug"] = slug.Make(e.Data.String(field))
			return nil
		})
	app.OnRowWrite("api::subscriber.subscriber").Bind(func(e *hook.RowEvent) error {
		e.Data["hash"] = uuid.NewString()
		return nil
	})

	// Génération IA quand un toggle est à true, puis toggle remis à false.
	app.Documents().Use(func(c *document.Context, next document.Next) (any, error) {
		if c.UID == "api::post.post" && (c.Action == document.Create || c.Action == document.Update) {
			if err := generateAIFields(c); err != nil {
				return nil, err
			}
		}
		return next(c)
	})

	// Invalidation du cache Redis par content type, après l'écriture.
	app.Documents().Use(func(c *document.Context, next document.Next) (any, error) {
		res, err := next(c)
		if err == nil && c.Action.Mutates() {
			cache.ClearPatterns(invalidationMap[c.UID]...)
		}
		return res, err
	})

	app.Controller("api::post.post").Override("find", func(ctx *httpapi.Ctx, base httpapi.Action) error {
		return base(ctx)
	})
	app.Route(httpapi.Route{Method: "GET", Path: "/posts/popular", Handler: popularPosts})

	log.Fatal(app.Start())
}
```

## Hors périmètre
- Le chargement de plugins tiers non compilés (`EXT-004`), les hooks JS (`EXT-003`).

## Critères d'acceptation
- [ ] Le projet d'exemple `examples/dev-freelance` reproduit les règles de slug, de hash, le middleware IA (avec un faux LLM) et l'invalidation du cache, et passe les 21 cas de caractérisation x 3 chemins (admin, REST, documents) avec le même résultat que la prod Strapi actuelle.
- [ ] Les API publiques sont documentées (godoc) et couvertes par un test d'exemple (`Example*`).
- [ ] Aucun type de `internal/` n'apparaît dans les signatures publiques.

## Avancement
- Fait : paquet racine `grapi` (`New`, `Setup` qui renvoie le `http.Handler`, `Start`, `UseDocuments`, `OnRow*` ... `Bind`), paquets publics `document` et `hook`. Le binaire `cmd/grapi` n'est plus qu'un `main` qui appelle `grapi.New(...).Start()`. Test de bout en bout : `hooks_test.go` (base neuve, bootstrap, inscription par HTTP, hook de ligne et middleware appliqués).
- Fait aussi : routes custom (`Route`, `PublicRoute`, `Request`, `Error`) et `Documents()`.
- Reste : phases `OnRegister`/`OnBootstrap`/`OnDestroy`, surcharge de contrôleurs et de services, policies, cron, accès aux autres services. Les alias de `document` exposent encore des types de `internal/` (critère « aucun type interne » non tenu).

## Notes techniques
- Les paquets publics (`grapi`, `schema`, `document`, `hook`, `errors`) suivent SemVer dès la v1. Avant la v1, les changements cassants sont signalés dans le CHANGELOG.
- Les hooks Go reçoivent `context.Context` pour la transaction et l'annulation.
- `Data` est un `map[string]any` avec quelques helpers typés (`String`, `Int`, `Bool`) : le schéma est dynamique.

## Référence Strapi
- `core/core/src/Strapi.ts` (register, bootstrap, destroy), `core/core/src/loaders/src-index.ts`
- `core/core/src/factories.ts`, `core/database/src/lifecycles/`

## Dépendances
- `SCH-005`, `DB-009`, `DOC-004`, `API-002`, `API-006`, `API-009`

## Fichiers
- `grapi.go`, `app.go`, `hook/*.go`, `examples/dev-freelance/main.go`
