// Package document exposes the Document Service to Go hooks: the context a
// middleware gets, and the actions it can see.
package document

import idoc "github.com/aluzed/grapi/internal/document"

type (
	// Context is a Document Service call going through the middlewares.
	Context = idoc.Context
	// Next runs the rest of the chain.
	Next = idoc.Next
	// Middleware is strapi.documents.use.
	Middleware = idoc.Middleware
	// Params are the call's params (Strapi's REST shape).
	Params = idoc.Params
	// Action is a Document Service method.
	Action = idoc.Action
)

// Actions.
const (
	FindOne      = idoc.FindOne
	FindFirst    = idoc.FindFirst
	FindMany     = idoc.FindMany
	Count        = idoc.Count
	Create       = idoc.Create
	Update       = idoc.Update
	Delete       = idoc.Delete
	Publish      = idoc.Publish
	Unpublish    = idoc.Unpublish
	DiscardDraft = idoc.DiscardDraft
)
