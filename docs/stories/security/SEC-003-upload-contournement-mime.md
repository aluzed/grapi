---
status: in-progress
---

# SEC-003 : Empêcher le contournement des types interdits à l'upload

Priorité : **Haute**. Source : [revue complémentaire du 29 octobre 2026](../../security-review-20261029.md).

## Contexte

Une politique interdisant HTML, SVG et JavaScript était contournable en déclarant `text/plain`. Le provider local sert ensuite le fichier selon son extension, avec un risque d'exécution de script sur l'origine du CMS.

## Objectif

En tant qu'administrateur, je veux que la politique d'upload couvre les extensions actives afin qu'un type MIME déclaré ne permette pas de la contourner.

## Périmètre

- Vérifier les extensions de contenu actif et leurs alias MIME en complément du contrôle du contenu.
- Vérifier l'extension effectivement conservée lors d'un remplacement.
- Documenter le maintien du comportement permissif en l'absence de politique.

## Hors périmètre

Activation de la politique sur un projet déployé et traitement des fichiers existants (`SEC-007`).

## Critères d'acceptation

- [ ] HTML, SVG et JavaScript interdits sont refusés même déclarés `text/plain`, y compris avec une extension en majuscules.
- [ ] Les alias JavaScript/XML et les extensions actives prévues par le contrôle suivent la même politique.
- [ ] Un remplacement ne peut conserver une extension interdite en présentant un nouveau fichier autorisé.
- [ ] Les fichiers autorisés et le traitement des lots restent conformes au contrat documenté.

## Notes techniques

Patches présents dans `checkExtension` et `uploadOne`. Régression : `TestUploadSecurityChecksActiveFileExtensions`. Le rapport indique des tests locaux réussis ; un contrôle de bout en bout du stockage et du service HTTP reste à consigner.

## Référence Strapi

Politique `plugins.upload.config.security` ; documenter le durcissement sans changer silencieusement les défauts de compatibilité.

## Dépendances

`MEDIA-001`, `MEDIA-003`, `SEC-015`.

## Fichiers

- `internal/httpapi/upload_security.go`
- `internal/httpapi/upload.go`
- `upload_security_test.go`
- `docs/configuration.md`

