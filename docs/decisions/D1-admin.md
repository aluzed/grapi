# D1 : admin de grapi

**Décision (2026-10-23) : option B, admin maison.** Prise par le mainteneur sur la base de `feasibility.md` (D1), sans faire les spikes prévus par `ADM-001`.

## Option retenue
- SPA Vite + React + TypeScript + Tailwind, sous `admin/`.
- Le build produit des fichiers statiques embarqués dans le binaire via `go:embed` et servis sous `/admin`. Il faut un outil JS au build de l'admin (Bun), mais rien au runtime : grapi reste un binaire unique.
- L'admin parle à une API grapi documentée et versionnée, `/admin/api/v1/*`. Elle n'imite pas les routes internes de Strapi (`/admin/*`, `/content-manager/*`, ...).
- Tout le code est MIT et écrit pour grapi. Rien n'est repris de `@strapi/admin`.
- Les conventions UI du projet s'appliquent (mobile-first, boutons pleine largeur sur mobile, confirmations des suppressions, etc.).

## Option écartée
Option A : servir la SPA `@strapi/admin` et reproduire fil à fil ses routes internes. Ces routes ne sont pas versionnées et changent à chaque mineure (entre 5.38 et 5.59 : sessions avec refresh, admin tokens, MCP, endpoints IA). L'option A impose aussi Node au build, une version de `@strapi/admin` figée par release, et un contrôle permanent qu'aucun code Enterprise n'entre dans le bundle.

## Conséquences
- `ADM-002` à `ADM-010` restent des tickets UI sur l'API `/admin/api/v1/*`.
- Le banc de compatibilité ne compare pas l'API admin à Strapi (`SETUP-002`). L'API de contenu publique (REST, GraphQL, OpenAPI) reste compatible fil à fil.
- L'API admin est protégée par le RBAC admin (`AUTH-001`) et les sessions admin (`AUTH-007`).
- Le premier ticket à ouvrir est `ADM-002` (shell : premier administrateur, connexion, navigation, profil).
