package grapi

import (
	"github.com/aluzed/grapi/internal/graphql"
	"github.com/aluzed/grapi/schema"
)

type (
	// GraphQLExtension adds type definitions, resolvers and resolversConfig.
	GraphQLExtension = graphql.Extension
	// GraphQLResolverConfig is a resolversConfig entry: auth, scope, policies.
	GraphQLResolverConfig = graphql.ResolverConfig
	// GraphQLResolveFunc resolves an extension field.
	GraphQLResolveFunc = graphql.ResolveFunc
	// GraphQLPolicyFunc is a resolver policy.
	GraphQLPolicyFunc = graphql.PolicyFunc
	// GraphQLFieldContext is what resolvers and policies get.
	GraphQLFieldContext = graphql.FieldContext
)

// GraphQL customizes the schema of the graphql plugin. Call it before Start.
type GraphQL struct{ app *App }

// GraphQL returns the graphql plugin's extension service.
func (a *App) GraphQL() *GraphQL { return &GraphQL{app: a} }

// Use adds type definitions, resolvers and resolversConfig.
func (g *GraphQL) Use(ext GraphQLExtension) {
	g.app.graphql.Extensions = append(g.app.graphql.Extensions, ext)
}

// ShadowCRUD is the shadow CRUD of one content type or component.
type ShadowCRUD struct{ c *graphql.CRUD }

// ShadowCRUD returns the shadow CRUD of uid.
func (g *GraphQL) ShadowCRUD(uid string) *ShadowCRUD {
	o := &g.app.graphql
	if o.ShadowCRUD == nil {
		o.ShadowCRUD = map[schema.UID]*graphql.CRUD{}
	}
	c := o.ShadowCRUD[schema.UID(uid)]
	if c == nil {
		c = &graphql.CRUD{DisabledActions: map[string]bool{}, Fields: map[string]*graphql.FieldCRUD{}}
		o.ShadowCRUD[schema.UID(uid)] = c
	}
	return &ShadowCRUD{c: c}
}

// Disable removes the type from the schema.
func (s *ShadowCRUD) Disable() *ShadowCRUD { s.c.Disabled = true; return s }

// DisableQueries removes its queries.
func (s *ShadowCRUD) DisableQueries() *ShadowCRUD { s.c.QueriesDisabled = true; return s }

// DisableMutations removes its mutations.
func (s *ShadowCRUD) DisableMutations() *ShadowCRUD { s.c.MutationsDisabled = true; return s }

// DisableAction removes one of find, findOne, create, update, delete.
func (s *ShadowCRUD) DisableAction(action string) *ShadowCRUD {
	s.c.DisabledActions[action] = true
	return s
}

// DisableActions removes several actions.
func (s *ShadowCRUD) DisableActions(actions ...string) *ShadowCRUD {
	for _, a := range actions {
		s.DisableAction(a)
	}
	return s
}

// ShadowField is the shadow CRUD of one field.
type ShadowField struct{ f *graphql.FieldCRUD }

// Field returns the shadow CRUD of a field.
func (s *ShadowCRUD) Field(name string) *ShadowField {
	f := s.c.Fields[name]
	if f == nil {
		f = &graphql.FieldCRUD{}
		s.c.Fields[name] = f
	}
	return &ShadowField{f: f}
}

// Disable removes the field from the output, input and filters types.
func (f *ShadowField) Disable() *ShadowField {
	f.f.Output, f.f.Input, f.f.Filters = true, true, true
	return f
}

// DisableOutput removes the field from the output type.
func (f *ShadowField) DisableOutput() *ShadowField { f.f.Output = true; return f }

// DisableInput removes the field from the input type.
func (f *ShadowField) DisableInput() *ShadowField { f.f.Input = true; return f }

// DisableFilters removes the field from the filters.
func (f *ShadowField) DisableFilters() *ShadowField { f.f.Filters = true; return f }
