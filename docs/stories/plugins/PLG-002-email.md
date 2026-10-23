---
status: in-progress
---

# PLG-002 : E-mail : service, templates, providers

## Contexte
Le plugin email de Strapi expose un service `send` et `sendTemplatedEmail`, utilisé par users-permissions, par l'admin (reset, invitation) et par le code utilisateur. Il délègue l'envoi à un provider (sendmail par défaut, nodemailer, SES, Mailgun, SendGrid). dev-freelance utilise nodemailer avec SMTP OVH et une signature DKIM.

## Objectif
Un service e-mail avec une interface provider, cinq providers et des templates sûrs.

## Périmètre
- Configuration `config/plugins` → `email` : `provider`, `providerOptions`, `settings {defaultFrom, defaultReplyTo}`.
- Interface Go : `Send(ctx, Message) error`, et en option `Verify(ctx) error`. `Message` contient from, to, cc, bcc, replyTo, subject, text, html, attachments et headers.
- Providers :
  - `sendmail` (binaire local) ;
  - `smtp`, équivalent de nodemailer : host, port (587 par défaut), secure, auth, pool, **DKIM** (domainName, keySelector, privateKey lue depuis un fichier) ;
  - `amazon-ses` ;
  - `mailgun` ;
  - `sendgrid`.
- `SendTemplated(ctx, msg, tpl {subject, text, html}, data)` : interpolation `<%= var %>` comme le template lodash de Strapi, **restreinte aux variables fournies**, sans évaluation de code. Les syntaxes `<% %>` et `${}` ne sont pas interprétées.
- Routes :
  - Content API : `POST /api/email`, soumis aux permissions (aucune par défaut) et rate limité (`AUTH-006`) ;
  - admin : `POST /email/test`, `GET /email/settings`.

## Hors périmètre
- Files d'attente et retries d'envoi (Strapi n'en a pas). C'est une amélioration possible plus tard.

## Critères d'acceptation
- [x] **Compat config** : la configuration nodemailer et DKIM de dev-freelance (sélecteur `sel2026`, clé lue depuis un fichier monté) se transpose en provider `smtp`. Un e-mail envoyé vers Mailpit sur le homelab est signé et vérifié (`d=dev-freelance.com`, `s=sel2026`).
- [x] Un template contenant `<% process.exit() %>` est rendu littéralement et n'exécute rien (TU).
- [x] `<%= USER.email %>` est remplacé, et une variable inconnue est rendue vide sans erreur, comme Strapi.
- [x] `POST /api/email` sans permission donne 403.
- [x] Les templates reset et confirmation de `AUTH-002` passent par ce service.

## Avancement
- Fait : `internal/plugins/email` (SMTP façon nodemailer avec STARTTLS et DKIM via go-msgauth, sendmail, `Template` de users-permissions), config `plugins.email.config` (`nodemailer` ou `sendmail`), `POST /api/email`.
- 2026-10-22, homelab : forgot-password servi par grapi vers Mailpit, signature DKIM vérifiée (`d=dev-freelance.com`, `s=sel2026`, clé lue par `${file:}`).
- Correction du critère 3 : chez Strapi, une variable inconnue n'est pas rendue vide, elle reste telle quelle (`template()` de users-permissions désactive `evaluate` et `escape`, seule l'interpolation stricte s'applique). grapi fait de même.
- Providers `sendgrid` (`apiKey`, API v3 `mail/send`), `mailgun` (`key`, `username`, `url`, `domain`, formulaire `messages` en auth basique) et `amazon-ses` (`key`, `secret`, `amazon` : action `SendEmail` de l'API query, signée SigV4, région lue dans l'endpoint), avec les `providerOptions` des providers Strapi (`internal/plugins/email/http.go`, tests sur serveurs `httptest`). Une réponse en erreur de l'API remonte avec son statut.
- Providers du projet en mode framework : `app.UseEmailProvider(nom, p)`, test `TestCustomEmailProvider` (mail de `forgot-password` reçu par le provider).
- Routes admin `GET /email/settings` et `POST /email/test` faites (`internal/adminapi/emailsettings.go`) ; l'e-mail de test passe par un vrai SMTP jusqu'à Mailpit (`make e2e-invite`).
- Reste : envoi réel vers chaque service hébergé (SendGrid, Mailgun, Amazon SES), qui demande des comptes de test.

## Notes techniques
- `wneessen/go-mail` (SMTP, sendmail) et `emersion/go-msgauth/dkim` pour signer.
- SDK `aws-sdk-go-v2/service/sesv2`, `mailgun/mailgun-go/v4`, `sendgrid/sendgrid-go`.
- Le parseur de template est maison (une regex sur `<%=\s*([\w.]+)\s*%>`) : surtout pas `text/template` avec des données utilisateur arbitraires.
- La clé privée DKIM ne doit jamais apparaître dans les logs ni dans les réponses de settings.

## Référence Strapi
- `core/email/server/src/{config.ts,register.ts,bootstrap.ts}`
- `core/email/server/src/services/email.ts` (`sendTemplatedEmail`)
- `core/email/server/src/routes/{content-api,admin}.ts`
- `providers/email-{nodemailer,sendmail,amazon-ses,mailgun,sendgrid}/src/index.ts`

## Dépendances
- `EXT-001`, `API-006`, `AUTH-006`

## Fichiers
- `internal/plugins/email/{service.go,template.go,routes.go}`, `internal/plugins/email/providers/{smtp,sendmail,ses,mailgun,sendgrid}/*.go`
