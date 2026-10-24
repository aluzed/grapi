package grapi

import (
	"context"

	"github.com/aluzed/grapi/internal/adminapi"
)

// PreviewRequest is the document a preview URL is built for, as the edit
// view shows it, with a token that reads it from the content API (Bearer
// or ?previewToken=, drafts included, for an hour).
type PreviewRequest struct {
	UID, DocumentID, Locale, Status string
	Document                        map[string]any
	Token                           string
}

// PreviewHandler builds a document's preview URL; "" hides the preview.
type PreviewHandler func(ctx context.Context, req PreviewRequest) (string, error)

// PreviewHandler sets uid's preview URL builder, ahead of the URL template
// of admin.preview.handlers. admin.preview.enabled must be true and the
// URL's origin listed in admin.preview.allowedOrigins.
func (a *App) PreviewHandler(uid string, fn PreviewHandler) {
	if a.preview == nil {
		a.preview = map[string]PreviewHandler{}
	}
	a.preview[uid] = fn
}

func (a *App) previewFuncs() map[string]adminapi.PreviewFunc {
	out := map[string]adminapi.PreviewFunc{}
	for uid, fn := range a.preview {
		out[uid] = func(ctx context.Context, req adminapi.PreviewRequest) (string, error) {
			return fn(ctx, PreviewRequest(req))
		}
	}
	return out
}
