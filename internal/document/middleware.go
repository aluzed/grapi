package document

import (
	"context"
	"sync"

	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/schema"
)

// Action is a document service method, as strapi.documents.use sees it.
type Action string

// Actions.
const (
	FindOne      Action = "findOne"
	FindFirst    Action = "findFirst"
	FindMany     Action = "findMany"
	Count        Action = "count"
	Create       Action = "create"
	Update       Action = "update"
	Delete       Action = "delete"
	Publish      Action = "publish"
	Unpublish    Action = "unpublish"
	DiscardDraft Action = "discardDraft"
)

// Context is a document service call going through the middlewares. Params
// may be changed before next; the documentId is Params["documentId"].
type Context struct {
	Ctx         context.Context
	UID         string
	ContentType *schema.ContentType
	Action      Action
	Params      Params
}

// Next runs the rest of the chain and the action.
type Next func() (any, error)

// Middleware is strapi.documents.use: registration order is execution
// order, Koa style.
type Middleware func(c *Context, next Next) (any, error)

type middlewares struct {
	mu   sync.RWMutex
	list []*Middleware
}

// Use adds a middleware and returns the function that removes it.
func (s *Service) Use(m Middleware) func() {
	s.mw.mu.Lock()
	defer s.mw.mu.Unlock()
	p := &m
	s.mw.list = append(s.mw.list, p)
	return func() {
		s.mw.mu.Lock()
		defer s.mw.mu.Unlock()
		for i, e := range s.mw.list {
			if e == p {
				s.mw.list = append(s.mw.list[:i:i], s.mw.list[i+1:]...)
				return
			}
		}
	}
}

func (s *Service) run(ctx context.Context, uid string, action Action, p Params, documentID *string, final func(ctx context.Context, p Params) (any, error)) (any, error) {
	s.mw.mu.RLock()
	chain := make([]*Middleware, len(s.mw.list))
	copy(chain, s.mw.list)
	s.mw.mu.RUnlock()
	if len(chain) == 0 {
		return final(ctx, p)
	}
	c := &Context{Ctx: ctx, UID: uid, Action: action, Params: clone(p)}
	if c.Params == nil {
		c.Params = Params{}
	}
	if documentID != nil {
		c.Params["documentId"] = *documentID
	}
	if ct, ok := s.Set.ContentType(schema.UID(uid)); ok {
		c.ContentType = ct
	}
	var step func(i int) (any, error)
	step = func(i int) (any, error) {
		if i == len(chain) {
			params := clone(c.Params)
			if documentID != nil {
				if d, ok := params["documentId"].(string); ok {
					*documentID = d
				}
				delete(params, "documentId")
			}
			return final(c.Ctx, params)
		}
		return (*chain[i])(c, func() (any, error) { return step(i + 1) })
	}
	return step(0)
}

func asRow(v any, err error) (query.Row, error) {
	r, _ := v.(query.Row)
	return r, err
}

func asRows(v any, err error) ([]query.Row, error) {
	r, _ := v.([]query.Row)
	return r, err
}

// FindMany returns the documents matching p.
func (s *Service) FindMany(ctx context.Context, uid string, p Params) ([]query.Row, error) {
	return asRows(s.run(ctx, uid, FindMany, p, nil, func(ctx context.Context, p Params) (any, error) { return s.findManyCore(ctx, uid, p) }))
}

// FindFirst returns the first document matching p.
func (s *Service) FindFirst(ctx context.Context, uid string, p Params) (query.Row, error) {
	return asRow(s.run(ctx, uid, FindFirst, p, nil, func(ctx context.Context, p Params) (any, error) { return s.findFirstCore(ctx, uid, p) }))
}

// FindOne returns one version of a document.
func (s *Service) FindOne(ctx context.Context, uid, documentID string, p Params) (query.Row, error) {
	return asRow(s.run(ctx, uid, FindOne, p, &documentID, func(ctx context.Context, p Params) (any, error) { return s.findOneCore(ctx, uid, documentID, p) }))
}

// Count counts the documents matching p.
func (s *Service) Count(ctx context.Context, uid string, p Params) (int64, error) {
	v, err := s.run(ctx, uid, Count, p, nil, func(ctx context.Context, p Params) (any, error) { return s.countCore(ctx, uid, p) })
	n, _ := v.(int64)
	return n, err
}

// Create creates a document.
func (s *Service) Create(ctx context.Context, uid string, p Params) (query.Row, error) {
	return asRow(s.run(ctx, uid, Create, p, nil, func(ctx context.Context, p Params) (any, error) { return s.createCore(ctx, uid, p) }))
}

// Update updates a document.
func (s *Service) Update(ctx context.Context, uid, documentID string, p Params) (query.Row, error) {
	return asRow(s.run(ctx, uid, Update, p, &documentID, func(ctx context.Context, p Params) (any, error) { return s.updateCore(ctx, uid, documentID, p) }))
}

// Delete deletes a document.
func (s *Service) Delete(ctx context.Context, uid, documentID string, p Params) (*DeleteResult, error) {
	v, err := s.run(ctx, uid, Delete, p, &documentID, func(ctx context.Context, p Params) (any, error) { return s.deleteCore(ctx, uid, documentID, p) })
	r, _ := v.(*DeleteResult)
	return r, err
}

// Publish publishes a document's drafts.
func (s *Service) Publish(ctx context.Context, uid, documentID string, p Params) ([]query.Row, error) {
	return asRows(s.run(ctx, uid, Publish, p, &documentID, func(ctx context.Context, p Params) (any, error) { return s.publishCore(ctx, uid, documentID, p) }))
}

// Unpublish removes a document's published versions.
func (s *Service) Unpublish(ctx context.Context, uid, documentID string, p Params) ([]query.Row, error) {
	return asRows(s.run(ctx, uid, Unpublish, p, &documentID, func(ctx context.Context, p Params) (any, error) { return s.unpublishCore(ctx, uid, documentID, p) }))
}

// DiscardDraft replaces a document's drafts with its published versions.
func (s *Service) DiscardDraft(ctx context.Context, uid, documentID string, p Params) ([]query.Row, error) {
	return asRows(s.run(ctx, uid, DiscardDraft, p, &documentID, func(ctx context.Context, p Params) (any, error) { return s.discardDraftCore(ctx, uid, documentID, p) }))
}
