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
- [ ] **Compat config** : la configuration nodemailer et DKIM de dev-freelance (sélecteur `sel2026`, clé lue depuis un fichier monté) se transpose en provider `smtp`. Un e-mail envoyé vers Mailpit sur le homelab est signé et vérifié (`d=dev-freelance.com`, `s=sel2026`).
- [ ] Un template contenant `<% process.exit() %>` est rendu littéralement et n'exécute rien (TU).
- [ ] `<%= USER.email %>` est remplacé, et une variable inconnue est rendue vide sans erreur, comme Strapi.
- [ ] `POST /api/email` sans permission donne 403.
- [ ] Les templates reset et confirmation de `AUTH-002` passent par ce service.

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
