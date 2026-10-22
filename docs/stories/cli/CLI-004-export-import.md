---
status: todo
---

# CLI-004 : Export et import (format d'archive Strapi)

## Contexte
`strapi export` produit une archive de tout le contenu : schémas, entités, liens, assets et configuration. `strapi import` la recharge. Pouvoir lire une archive Strapi réelle, c'est une seconde voie de migration vers grapi, en plus de `DB-011`, qui fonctionne aussi entre dialectes (MySQL vers Postgres, par exemple).

## Objectif
Export et import compatibles avec le format d'archive de Strapi 5, en lecture comme en écriture.

## Périmètre
- **Format** :
  - archive tar, puis gzip, puis chiffrement : `<nom>.tar[.gz][.enc]` ;
  - contenu :
    - `metadata.json` (versions, date) ;
    - `schemas/`, `entities/`, `links/` et `configuration/` en fichiers JSONL découpés (`maxSizeJsonl` lignes par fichier) ;
    - `assets/uploads/<fichier>` et `assets/metadata/<fichier>.json`.
- **Étapes**, dans l'ordre : schemas, entities, links, assets, configuration.
- À l'import, contrôle de compatibilité des schémas avec `--force` pour passer outre. Les données existantes sont remplacées (`strategy: restore`), mais les tables admin (utilisateurs, rôles, tokens) sont conservées, comme Strapi.
- Options :
  - `--file`, `--no-encrypt`, `--no-compress`, `--key` (ou demandée en interactif), `--format tar|dir` ;
  - `--only` et `--exclude` (content, files, config) ;
  - `--include-types` et `--exclude-types`, `--throttle`, `--force`.
- Progression affichée par étape, et code de sortie non nul en cas d'erreur.
- **Chiffrement** :
  - **compatibilité Strapi, obligatoire en lecture et disponible en écriture** : `aes-128-ecb`, avec une clé dérivée par `scrypt(key, salt = "", keylen = 16)` aux paramètres par défaut de Node (N = 16384, r = 8, p = 1) ;
  - **format natif grapi, par défaut à l'écriture** : AES-256-GCM en blocs authentifiés, clé dérivée par scrypt ou argon2id avec un sel aléatoire stocké en en-tête. L'en-tête porte un magic, qui permet de détecter le format à la lecture ;
  - `--strapi-compat` force l'ancien format pour produire une archive qu'un Strapi peut importer.

## Hors périmètre
- Transfert distant (`CLI-005`).

## Critères d'acceptation
- [ ] **Import d'une archive produite par `strapi export` 5.59 chiffrée** : le contenu de la copie dev-freelance est restauré et l'API donne des réponses identiques à Strapi (banc `SETUP-002`).
- [ ] Une archive grapi produite avec `--strapi-compat` est importée par Strapi 5.59 sans erreur (homelab).
- [ ] Une archive au format natif dont un octet est altéré est rejetée (authentification GCM), alors que le format ECB ne le détecte pas (TU qui documente cet écart).
- [ ] Une mauvaise clé donne une erreur claire, sans plantage du parseur.
- [ ] Les comptes admin existants survivent à un import (intégration).

## Notes techniques
- **Pourquoi changer de chiffrement par défaut** :
  - AES-ECB chiffre chaque bloc de 16 octets indépendamment, donc des blocs identiques donnent des chiffrés identiques. La structure des données répétitives (JSONL) fuit, et il n'y a ni intégrité ni authentification ;
  - scrypt sans sel donne la même clé pour un même mot de passe sur toutes les archives.
  On garde donc ce format uniquement pour la compatibilité, avec un avertissement dans les logs quand on l'écrit.
- `archive/tar`, `compress/gzip`, `crypto/aes` (ECB se fait à la main, bloc par bloc, avec padding PKCS#7 comme le `Cipher` de Node) et `golang.org/x/crypto/scrypt`.
- Tout se fait en streaming : pas de chargement de l'archive en mémoire.

## Référence Strapi
- `core/data-transfer/src/file/providers/{source,destination}/index.ts`
- `core/data-transfer/src/utils/encryption/{encrypt,decrypt}.ts`
- `core/data-transfer/src/engine/`
- `core/strapi/src/cli/commands/{export,import}/`

## Dépendances
- `DOC-001`, `DB-006`, `MEDIA-003`, `DB-010`, `CLI-001`

## Fichiers
- `internal/transfer/{engine.go,stages.go}`, `internal/transfer/archive/{reader.go,writer.go,crypto_strapi.go,crypto_native.go}`, `internal/cli/{export.go,import.go}`
