package grapi

import (
	"context"

	"github.com/aluzed/grapi/internal/media/provider"
	"github.com/aluzed/grapi/internal/plugins/email"
)

type (
	// UploadProvider stores the files of the upload plugin.
	UploadProvider = provider.Provider
	// UploadFile is what an upload provider gets: set URL, and Metadata
	// for the provider_metadata given back on delete.
	UploadFile = provider.File
	// EmailProvider sends the email plugin's messages.
	EmailProvider = email.Provider
	// EmailMessage is a message to send.
	EmailMessage = email.Message
)

// UseUploadProvider registers an upload provider of the project, used when
// plugins.upload.config.provider is name.
func (a *App) UseUploadProvider(name string, p UploadProvider) {
	if a.uploadProviders == nil {
		a.uploadProviders = map[string]UploadProvider{}
	}
	a.uploadProviders[name] = p
}

// UseEmailProvider registers an email provider of the project, used when
// plugins.email.config.provider is name.
func (a *App) UseEmailProvider(name string, p EmailProvider) {
	if a.emailProviders == nil {
		a.emailProviders = map[string]EmailProvider{}
	}
	a.emailProviders[name] = p
}

// SendEmail is strapi.plugin('email').service('email').send: the message
// goes through the configured email provider, with the settings'
// defaultFrom and defaultReplyTo when it has none. It works once the App
// is set up.
func (a *App) SendEmail(ctx context.Context, m EmailMessage) error {
	return a.email.Send(ctx, m)
}
