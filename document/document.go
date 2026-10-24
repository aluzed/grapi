// Package document exposes the Document Service to Go hooks: the context a
// middleware gets, the actions it can see and the params of a call.
package document

import (
	"context"

	"github.com/aluzed/grapi/schema"
)

// Params are the Document Service params: Strapi's REST-shaped query (sort,
// filters, fields, populate, pagination) plus status and locale.
type Params map[string]any

// Action is a Document Service method, as strapi.documents.use sees it.
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
	Clone        Action = "clone"
)

// Context is a Document Service call going through the middlewares. Params
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

// DeleteResult is what Delete removed: the document and its entries.
type DeleteResult struct {
	DocumentID string
	Entries    []*schema.OrderedMap[any]
}

// CloneResult is what Clone created: the documentId of its first entry,
// and the entries.
type CloneResult struct {
	DocumentID string
	Entries    []*schema.OrderedMap[any]
}
