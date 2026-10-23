package grapi

import (
	"context"

	"github.com/aluzed/grapi/document"
	idoc "github.com/aluzed/grapi/internal/document"
	"github.com/aluzed/grapi/schema"
)

// Entry is a document entry: its fields in schema order.
type Entry = *schema.OrderedMap[any]

// DocumentService is strapi.documents: the Document Service for Go code.
type DocumentService struct{ s *idoc.Service }

// FindMany is documents(uid).findMany.
func (d *DocumentService) FindMany(ctx context.Context, uid string, p document.Params) ([]Entry, error) {
	return d.s.FindMany(ctx, uid, p)
}

// FindFirst is documents(uid).findFirst; nil when nothing matches.
func (d *DocumentService) FindFirst(ctx context.Context, uid string, p document.Params) (Entry, error) {
	return d.s.FindFirst(ctx, uid, p)
}

// FindOne is documents(uid).findOne; nil when not found.
func (d *DocumentService) FindOne(ctx context.Context, uid, documentID string, p document.Params) (Entry, error) {
	return d.s.FindOne(ctx, uid, documentID, p)
}

// Count is documents(uid).count.
func (d *DocumentService) Count(ctx context.Context, uid string, p document.Params) (int64, error) {
	return d.s.Count(ctx, uid, p)
}

// Create is documents(uid).create.
func (d *DocumentService) Create(ctx context.Context, uid string, p document.Params) (Entry, error) {
	return d.s.Create(ctx, uid, p)
}

// Update is documents(uid).update.
func (d *DocumentService) Update(ctx context.Context, uid, documentID string, p document.Params) (Entry, error) {
	return d.s.Update(ctx, uid, documentID, p)
}

// Delete is documents(uid).delete.
func (d *DocumentService) Delete(ctx context.Context, uid, documentID string, p document.Params) (*document.DeleteResult, error) {
	return d.s.Delete(ctx, uid, documentID, p)
}

// Publish is documents(uid).publish.
func (d *DocumentService) Publish(ctx context.Context, uid, documentID string, p document.Params) ([]Entry, error) {
	return d.s.Publish(ctx, uid, documentID, p)
}

// Unpublish is documents(uid).unpublish.
func (d *DocumentService) Unpublish(ctx context.Context, uid, documentID string, p document.Params) ([]Entry, error) {
	return d.s.Unpublish(ctx, uid, documentID, p)
}

// DiscardDraft is documents(uid).discardDraft.
func (d *DocumentService) DiscardDraft(ctx context.Context, uid, documentID string, p document.Params) ([]Entry, error) {
	return d.s.DiscardDraft(ctx, uid, documentID, p)
}
