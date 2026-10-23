package document

import "context"

type originKey struct{}

// WithOrigin tags the calls made with ctx: admin-panel, admin-token (the
// content API with an API token), content-api, or document-service when
// untagged.
func WithOrigin(ctx context.Context, origin string) context.Context {
	return context.WithValue(ctx, originKey{}, origin)
}

// OriginOf is the origin of ctx.
func OriginOf(ctx context.Context) string {
	if o, ok := ctx.Value(originKey{}).(string); ok {
		return o
	}
	return "document-service"
}
