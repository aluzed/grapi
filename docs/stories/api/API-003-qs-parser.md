---
status: in-progress
---

# API-003 : Parseur de query string compatible qs

## Contexte
Les clients Strapi construisent leurs requêtes avec la bibliothèque `qs` : `filters[title][$containsi]=go&populate[cover][fields][0]=url&sort[0]=publishedAt:desc`. Le parseur standard de Go ne comprend pas cette syntaxe imbriquée.

## Objectif
Un parseur Go qui produit le même arbre que `qs.parse` avec les options de Strapi.

## Périmètre
- Options par défaut de Strapi : `strictNullHandling: true` (`?a` donne `null`, `?a=` donne `""`), `arrayLimit: 100` (au-delà, l'index devient une clé d'objet), `depth: 20` (au-delà, le reste est gardé tel quel comme clé).
- Notations : crochets imbriqués, tableaux indexés (`a[0]=x`), tableaux sans index (`a[]=x`), répétition de clé (`a=x&a=y` donne un tableau), notation pointée non activée (comme Strapi).
- Décodage URL (`%5B`, `+`), clés et valeurs.
- Index creux : `a[1]=x&a[3]=y` compacté en tableau, comme qs.
- Mélange tableau et objet sur une même clé : même résultat que qs.
- Options surchargeables via la config du middleware `strapi::query`.
- Sérialiseur inverse (utilisé par les liens de pagination et les tests).

## Hors périmètre
- L'interprétation sémantique des paramètres (`API-004`).

## Critères d'acceptation
- [ ] Table de 100 cas générés en exécutant le vrai `qs.parse` (fixtures JSON produites sur le homelab) : résultat identique à 100 %.
- [ ] Fuzzing (`go test -fuzz`) sans panique ni explosion mémoire (limite de paramètres, défaut 1000 comme qs `parameterLimit`).
- [ ] Les paramètres déjà peuplés en mémoire sont mis en cache par requête.

## Notes techniques
- Représentation : `map[string]any`, `[]any`, `string`, `nil`. Garder l'ordre des clés n'est pas nécessaire pour qs mais utile pour les messages d'erreur : utiliser une map ordonnée si besoin.
- Paquet autonome sans dépendance au reste de grapi : publiable séparément.

## Référence Strapi
- `core/core/src/middlewares/query.ts`
- Comportement de référence : bibliothèque npm `qs` (MIT), à exécuter pour générer les fixtures, pas à traduire.

## Dépendances
- `SETUP-001`

## Fichiers
- `internal/httpapi/qs/parse.go`, `internal/httpapi/qs/stringify.go`, `internal/httpapi/qs/testdata/`
