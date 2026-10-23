---
status: in-progress
---

# ADM-002 : Shell admin : connexion, navigation, profil, page d'accueil

> Option retenue : B (admin maison), voir `docs/decisions/D1-admin.md`.

## Contexte
Tous les écrans de l'admin partagent un shell : écran de création du premier administrateur, connexion, mot de passe oublié, menu latéral, profil et page d'accueil. Strapi propose les mêmes fonctions (`/admin/init`, `register-admin`, login avec refresh en cookie, page Profil avec sessions, page d'accueil avec widgets).

## Objectif
Livrer le socle de la SPA admin (`admin/`), embarqué dans le binaire, avec l'authentification admin et la navigation, et fixer une fois pour toutes les conventions UI que les autres tickets ADM référencent.

## Périmètre
- Projet `admin/` : Vite + React 18+ + TypeScript strict + Tailwind, React-Query, React-Hook-Form + Zod, react-router.
  - État global via Context/Provider uniquement (`AuthProvider`, `PermissionsProvider`, `SchemaProvider`).
  - Toasts pour le retour utilisateur.
  - Composants de base : Button, Table, Dialog, Tabs, EntityActionsMenu, ConfirmProvider (`useConfirm`).
- Build embarqué via `go:embed` et servi sous `/admin` (fallback SPA sur `index.html`). URL de l'admin configurable (`admin.path`).
- Écrans :
  - **premier lancement** : si aucun admin n'existe, formulaire de création du premier super admin ;
  - **connexion** : e-mail, mot de passe, « Se souvenir de moi » ;
  - **mot de passe oublié** et **réinitialisation** ;
  - **inscription par invitation** : jeton d'enregistrement.
- Session :
  - access token en mémoire, refresh en cookie httpOnly ;
  - renouvellement transparent sur 401 ;
  - déconnexion.
- Navigation :
  - menu latéral avec Content Manager, Content-Type Builder (en mode develop seulement), Media Library, Settings, et entrées ajoutées par les plugins (`ADM-010`) ;
  - masquage selon les permissions de l'utilisateur.
- Profil :
  - prénom, nom, e-mail, langue de l'interface, changement de mot de passe ;
  - liste des sessions actives, avec révocation d'une session ou de toutes les autres.
- Page d'accueil : widgets nombre de documents par content type, derniers documents modifiés, derniers documents publiés.
- Traductions de l'interface : fr et en au minimum, fichiers JSON par module.

## Conventions UI (référence pour tous les tickets ADM et XTR)
- **Mobile-first** : sur mobile, empiler verticalement, jamais 2 éléments côte à côte sur une ligne serrée. Conteneur type : `flex flex-col gap-X md:flex-row md:items-center md:justify-between`.
- **Boutons sur mobile** : chaque bouton d'action prend 100 % de la largeur (`w-full`), empilé en colonne. Les CTA d'en-tête de page sont en `w-full md:w-auto`.
- **Formulaires** : les grilles s'empilent (`grid grid-cols-1 gap-3 sm:grid-cols-N`). Les onglets prennent 100 % de la largeur sur mobile.
- **Bouton Retour** : toujours seul sur sa ligne, en haut, et le titre en dessous.
- **Boutons d'action** :
  - une action est un vrai bouton : icône + texte + attribut `title` ;
  - jamais d'icône seule, sauf la croix de fermeture d'un dialog et le bouton engrenage `EntityActionsMenu` ;
  - jamais de `variant="ghost"` ni de style lien pour une action.
- **Couleurs des boutons** :
  - jamais de bouton cliquable gris : le gris signifie désactivé ;
  - action neutre en `variant="outline"` ;
  - suppression en `outline` avec `border-destructive/40 text-destructive`, ou en `destructive`.
- **Exception des lignes de liste compactes** : icône seule sur mobile et texte sur desktop, `title` toujours présent.
- **Libellés** complets, sans abréviation.
- **Toute suppression** passe par une modale de confirmation (`useConfirm`).
- **Tables desktop** : actions de ligne regroupées dans le menu engrenage `EntityActionsMenu`, avec zébrures et survol.
- **Modales** : scrollables si le contenu dépasse l'écran (`max-h` + `overflow-y-auto`).

## Hors périmètre
- Les écrans métier (`ADM-003` à `ADM-010`).
- Le SSO (`XTR-001`).

## Critères d'acceptation
- [x] Sur une base vide, `/admin` affiche la création du premier admin. Une fois créé, cet écran n'est plus accessible.
- [ ] Connexion, déconnexion, mot de passe oublié et invitation fonctionnent de bout en bout (e-mail capturé par Mailpit en test).
- [ ] Un access token expiré est renouvelé sans perte de l'action en cours.
- [x] Sans « Se souvenir de moi », la session meurt avec le navigateur.
- [x] Les sessions listées dans le profil sont révocables, et une session révoquée ne peut plus renouveler son token.
- [x] Les entrées de menu sans permission ne s'affichent pas. Accéder à leur URL directement mène à une page 403.
- [x] À 375 px de large, aucun écran du shell n'a de scroll horizontal et tous les boutons d'action sont en pleine largeur.
- [x] Le binaire `grapi` sert l'admin sans aucun fichier externe.
- [x] Interface disponible en fr et en en, avec bascule depuis le profil.

## Avancement
- Backend du shell : API `/admin/api/v1` pour le premier admin, la connexion, le refresh, la déconnexion, le mot de passe oublié, l'invitation, le profil et les sessions (`docs/admin-api.md`).
- `admin/` est monté sous `/admin` avec fallback SPA (`admin.FS`, embarqué avec le tag `adminui`), page d'attente sans build.
- SPA `admin/` (Bun, Vite, React 19, TypeScript strict, Tailwind 4, React Query, React Hook Form + Zod, react-router, sonner) : création du premier admin, connexion avec « Se souvenir de moi », mot de passe oublié, réinitialisation, inscription par invitation, menu latéral (tiroir sur mobile) filtré par permissions avec page 403, profil (infos, langue, mot de passe, sessions révocables), accueil avec raccourcis. Composants de base Button, champs, Card, Dialog, Table, Tabs, EntityActionsMenu, `useConfirm`. Interface fr et en.
- Vérifié dans Chrome sur le binaire (`make build`) : premier admin, reprise de session au rechargement, deux onglets qui renouvellent en même temps, 375 px sans scroll horizontal avec boutons pleine largeur.
- Vérifié en navigateur avec un Editor invité : menu sans Content-Type Builder, Paramètres réduits à la vue d'ensemble, page 403 sur `/settings/users`.
- Écrit mais pas encore vérifié en navigateur : le renouvellement sur 401 avec rejeu de la requête (il faut attendre l'expiration de l'access token, 30 minutes par défaut).
- Reste : widgets de l'accueil (nombre de documents, derniers modifiés et publiés, qui attendent l'API du Content Manager `ADM-003`), e-mails de bout en bout sur Mailpit, client TS généré depuis l'OpenAPI admin.

## Notes techniques
- Endpoints admin grapi versionnés sous `/admin/api/v1`. Schéma OpenAPI généré (voir `PLG-005`) et client TS généré pour la SPA.
- Le cookie de refresh suit la config `admin.auth.cookie` (`secure`, `sameSite`, `domain`).
- **Si l'option A est retenue** : ce ticket se réduit au service des fichiers buildés de `@strapi/admin`, aux endpoints `/admin/init`, `/admin/project-type`, `/admin/information`, `/admin/telemetry-properties`, `/admin/guided-tour-meta` et `/admin/homepage/*`, et au flux `/admin/login` + `/admin/access-token`. Les conventions UI ne s'appliquent alors qu'aux écrans propres à grapi.

## Référence Strapi
- `core/admin/server/src/routes/authentication.ts`, `core/admin/server/src/controllers/authentication.ts`
- `core/core/src/services/session-manager.ts`
- `core/admin/admin/src/pages/Auth/`, `core/admin/admin/src/pages/ProfilePage.tsx`, `core/admin/admin/src/pages/Home/`

## Dépendances
- `ADM-001`, `AUTH-001`, `AUTH-003`, `AUTH-007`, `PLG-002` (e-mails de reset et d'invitation)

## Fichiers
- `admin/package.json`, `admin/vite.config.ts`, `admin/src/main.tsx`, `admin/src/app/`, `admin/src/components/ui/`, `admin/src/providers/`, `admin/src/pages/auth/`, `admin/src/pages/profile/`, `admin/src/pages/home/`, `admin/src/i18n/`
- `admin/embed.go`
- `internal/httpapi/admin/` (handlers shell)
