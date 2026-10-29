# Revue complémentaire de sécurité du 29 octobre 2026

Base examinée : `a51e206`. Cette revue complète l'[audit précédent](security-audit-20261029.md). Elle porte sur le code de ce dépôt, ses configurations par défaut et ses dépendances ; aucun serveur de production ni backend privé n'a été interrogé. Les correctifs sont dans l'arbre de travail.

## Problèmes reproduits et corrigés

| Priorité | Problème | Conditions et impact | Correctif |
|---|---|---|---|
| Haute | Inscriptions simultanées du premier administrateur | Pendant l'initialisation d'une base vide, plusieurs requêtes passaient le contrôle d'absence d'admin. Huit requêtes concurrentes ont créé huit super administrateurs. | Vérification et création dans une transaction ; verrou de ligne PostgreSQL sur le rôle commun, écriture sérialisée SQLite. |
| Haute | Historique sans restrictions par document et par langue | Un admin limité à ses propres documents ou à une langue pouvait consulter, comparer et restaurer les versions des autres. Les identifiants numériques de versions sont énumérables. | Vérification des conditions sur le document actuel et sa langue pour chaque accès, y compris les deux versions d'une comparaison. La liste omet les versions interdites ; lecture et restauration renvoient 404. |
| Haute | Contournement des types interdits à l'upload | Avec HTML, SVG et JavaScript explicitement interdits, les mêmes fichiers étaient acceptés en déclarant `text/plain`. Le stockage local conserve l'extension et la sert selon celle-ci : risque de script exécuté sur l'origine du CMS. | Les extensions de contenu actif sont aussi soumises à la politique MIME. Contrôle de l'extension effectivement conservée lors d'un remplacement. Sans configuration de sécurité, le comportement reste permissif. |
| Moyenne | Limitation des connexions contournable par des espaces | La connexion normalisait l'e-mail avec suppression des espaces, mais le compteur ne le faisait pas. Après cinq échecs sur un compte, chaque variante avec espaces permettait de nouveaux essais sur ce même compte. | Même normalisation pour le compteur et l'authentification ; cela couvre aussi les autres routes utilisant ce middleware. |
| Moyenne | Anciennes valeurs exposées par `unknownFields` | Après suppression d'un champ du schéma, l'historique renvoyait sa valeur à un lecteur sans droit sur ce champ, hors du nettoyage de `data`. | Les valeurs sans schéma sont réservées aux super admins. Les noms dans `missingFields` respectent aussi les droits de lecture. |
| Moyenne | Plages locales oubliées dans le filtrage SSRF | Malgré `blockPrivateNetworks`, les livraisons de webhooks acceptaient le CGNAT. Les téléchargements par URL et la validation des webhooks acceptaient une partie de `0.0.0.0/8`. L'impact dépend du réseau accessible depuis le serveur. | Blocage de ces plages, avec tests des formes IPv4 et IPv4 encapsulé en IPv6, au moment de la connexion. |

Les plages `100.64.0.0/10` et `0.0.0.0/8` ne sont pas globalement accessibles selon le [registre IANA](https://www.iana.org/assignments/iana-ipv4-special-registry/iana-ipv4-special-registry.xhtml).

Tests de régression : `TestAdminFirstRegistrationIsAtomic`, `TestAdminHistoryEnforcesDocumentAndLocalePermissions`, `TestAdminHistoryAfterFieldRemoval`, `TestAdminLoginRateLimit`, `TestUploadSecurityChecksActiveFileExtensions`, `TestPublicOnlyAddresses`, `TestPublicOnly` et `TestWebhookURLInProduction`. Les inscriptions concurrentes, les accès à l'historique, les variantes d'e-mail et les uploads interdits ont été reproduits par des tests en échec avant correction.

## Points restant à traiter avant une mise en production

Ce dépôt est un moteur de CMS : la configuration effective du projet et les permissions en base ne sont pas disponibles dans cette revue. Les réglages ci-dessous doivent être appliqués au projet déployé selon ses besoins.

1. **Upload : configurer une liste de types autorisés et limiter les comptes pouvant uploader.** Sans `plugins.upload.config.security`, tout type reste accepté. Le correctif empêche le contournement d'une politique existante, mais n'instaure pas une interdiction globale. Préférer, par exemple, `allowedTypes: [image/jpeg, image/png, image/webp, application/pdf]` à une liste d'interdictions incomplète. Pour des contenus actifs nécessaires, utiliser une origine distincte sans cookies d'administration.
2. **Webhooks : activer `server.webhooks.blockPrivateNetworks: true`.** Par défaut, cette protection de connexion reste désactivée. La validation de l'URL à l'enregistrement ne suffit pas contre un changement DNS ou une redirection ultérieure. Les téléchargements par URL ont déjà une protection distincte, activée par défaut.
3. **CORS : remplacer l'origine `*` par les origines précises des applications.** Le défaut reflète toute origine et autorise les credentials. C'est particulièrement risqué avec des cookies `SameSite=None` ou un sous-domaine non fiable du même site. Les sessions admin utilisent un cookie `SameSite=Lax` par défaut, ce qui réduit les scénarios intersites mais ne remplace pas une politique CORS explicite.
4. **Users-permissions : ne pas accorder `user.update` à un rôle ordinaire sans contrôle adapté.** Cette route accepte des modifications sensibles, dont le rôle, sur l'utilisateur ciblé. La présence d'une permission d'action ne constitue pas une restriction au propre compte de l'appelant. Les jetons de reset/confirmation des utilisateurs n'ont pas de durée d'expiration ; les JWT legacy restent valides après changement de mot de passe. Prévoir le mode `jwtManagement: refresh` et un durcissement des jetons selon le contrat applicatif.
5. **GraphQL : définir `depthLimit` et `maxLimit` si le plugin est activé.** Ils sont illimités par défaut. Par exemple, commencer avec une profondeur de 8 et 100 éléments, puis ajuster aux requêtes du projet. Une limite de débit et de complexité à l'entrée reste utile contre les requêtes avec beaucoup d'alias.
6. **Proxy et ressources : n'exposer que le proxy de confiance.** Avec un seul proxy, `server.proxy.koa: true` et `maxIpsCount: 1` évitent de prendre une valeur client arbitraire dans `X-Forwarded-For`, à condition de fermer l'accès direct au port interne. Définir aussi les délais de lecture des requêtes et des limites de ressources. La limite d'image par défaut autorise environ 268 millions de pixels, soit environ 1 Go pour une seule image RGBA décodée : à réduire pour un petit conteneur.

Ces comportements sont pour partie conservés pour la compatibilité Strapi. Cette compatibilité ne dispense pas de traiter leur impact dans le déploiement.

## Vérifications et limites

- Outils installés temporairement hors du dépôt : Go `1.26.10`, Bun `1.4.4`, `govulncheck`.
- `go build ./...`, `go vet ./internal/...` et `go mod verify` : réussis.
- Tests d'administration et d'upload, tests des paquets `internal/adminapi` et `internal/webhooks` : réussis après correction.
- Admin : `bun run typecheck` et 74 tests réussis. Installation avec lockfile figé et scripts d'installation désactivés.
- `bun audit` : aucune vulnérabilité signalée sur 247 paquets du lockfile.
- `govulncheck -show verbose ./...`, puis analyse pour la cible `linux/amd64` : aucune vulnérabilité dans les paquets Go importés ou les symboles utilisés. Le module `golang.org/x/crypto` contient l'[avis GO-2026-5932 sur OpenPGP](https://pkg.go.dev/vuln/GO-2026-5932), mais ce paquet n'est pas importé par le projet. Le résultat sur la bibliothèque standard correspond au compilateur utilisé pour ce contrôle ; maintenir aussi le compilateur de production à jour.
- Recherche ciblée de clés privées, clés AWS et jetons GitHub dans l'arbre courant hors dépendances : aucune correspondance. Ce contrôle ne couvre pas tout l'historique Git ni tous les formats de secrets.
- La suite complète n'est pas verte dans cet environnement Windows. Deux fichiers utilisent `syscall.Kill` et empêchent sa compilation ; un overlay Go temporaire les exclut sans modifier le dépôt. Après cette exclusion, les tests de signaux, socket Unix, supervision de compilation et génération de squelettes échouent sous Windows. Les références de `internal/typegen/TestMatchesStrapi` échouaient déjà avant les correctifs. Ces échecs n'ont pas été masqués dans les résultats de la suite complète.
- PostgreSQL, le détecteur de courses, les services S3 et le déploiement réel n'ont pas été testés ici. Le test concurrent du premier admin et ceux de l'historique peuvent être rejoués avec `-tags integration` et une base PostgreSQL de test dédiée via le mécanisme existant du dépôt.

Cette revue ne constitue pas une preuve d'absence de vulnérabilités. Les correctifs ne retirent pas rétroactivement des fichiers dangereux déjà stockés et ne révoquent pas des comptes ou sessions éventuellement compromis.
