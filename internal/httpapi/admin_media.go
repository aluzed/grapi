package httpapi

import (
	"context"
	"net/http"

	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/schema"
)

type (
	uploaderKey     struct{}
	uploadFolderKey struct{}
)

// rootFolder stands for "no folder" in the upload target.
var rootFolder = &schema.OrderedMap[any]{}

// WithUploader makes the files uploaded with ctx created by the admin
// userID, into folder (a plugin::upload.folder row, nil for the root).
func WithUploader(ctx context.Context, userID any, folder query.Row) context.Context {
	if folder == nil {
		folder = rootFolder
	}
	return context.WithValue(context.WithValue(ctx, uploaderKey{}, userID), uploadFolderKey{}, folder)
}

func uploadTarget(ctx context.Context) query.Row {
	f, _ := ctx.Value(uploadFolderKey{}).(query.Row)
	return f
}

// AdminUpload is the upload route for the admin (see WithUploader): files,
// file info update with ?id=, or replacement with ?id= and a file.
func (s *Server) AdminUpload(w http.ResponseWriter, r *http.Request) {
	if err := s.upload(w, r, &requestContext{params: map[string]string{}}); err != nil {
		writeError(w, err, s.log)
	}
}

// AdminDeleteFile removes a file and its stored formats, nil when there is
// no such file.
func (s *Server) AdminDeleteFile(ctx context.Context, id any) (query.Row, error) {
	row, err := s.docs.E.FindOne(ctx, fileUID, query.Params{Where: map[string]any{"id": id}})
	if err != nil || row == nil {
		return nil, err
	}
	s.removeFromProvider(ctx, row)
	if _, err := s.docs.E.Delete(ctx, fileUID, map[string]any{"id": id}, query.Params{}); err != nil {
		return nil, err
	}
	s.emitMedia("media.delete", row)
	return row, nil
}

// SignFile replaces the URLs of a private provider's file by signed ones.
func (s *Server) SignFile(ctx context.Context, row query.Row) { s.signFile(ctx, row) }
