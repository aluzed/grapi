package schema

import "encoding/json"

// Set is every content type and component of an application, once prepared.
type Set struct {
	ContentTypes OrderedMap[*ContentType]
	Components   OrderedMap[*Component]
}

// AddContentType registers a prepared content type under its UID.
func (s *Set) AddContentType(ct *ContentType) { s.ContentTypes.Set(string(ct.UID), ct) }

// AddComponent registers a prepared component under its UID.
func (s *Set) AddComponent(c *Component) { s.Components.Set(string(c.UID), c) }

// ContentType returns the content type registered under uid.
func (s *Set) ContentType(uid UID) (*ContentType, bool) { return s.ContentTypes.Get(string(uid)) }

// Component returns the component registered under uid.
func (s *Set) Component(uid UID) (*Component, bool) { return s.Components.Get(string(uid)) }

// attributesOf returns the attributes of a content type or component uid.
func (s *Set) attributesOf(uid string) (*OrderedMap[*Attribute], bool) {
	if ct, ok := s.ContentTypes.Get(uid); ok {
		return &ct.Attributes, true
	}
	if c, ok := s.Components.Get(uid); ok {
		return &c.Attributes, true
	}
	return nil, false
}

// MarkPrivateAttributes is Strapi's getStoredPrivateAttributes: the names of
// api.responses.privateAttributes and of a model's options.privateAttributes
// are private too, without changing what the schema files say.
func (s *Set) MarkPrivateAttributes(global []string) {
	mark := func(attrs *OrderedMap[*Attribute], opts *OrderedMap[json.RawMessage]) {
		names := append([]string(nil), global...)
		if opts != nil {
			if raw, ok := opts.Get("privateAttributes"); ok {
				var own []string
				if json.Unmarshal(raw, &own) == nil {
					names = append(names, own...)
				}
			}
		}
		for _, n := range names {
			if a, ok := attrs.Get(n); ok {
				a.storedPrivate = true
			}
		}
	}
	for _, ct := range s.ContentTypes.All() {
		mark(&ct.Attributes, ct.Options)
	}
	for _, c := range s.Components.All() {
		mark(&c.Attributes, c.Options)
	}
}
