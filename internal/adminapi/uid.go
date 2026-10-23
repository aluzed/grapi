package adminapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/internal/slug"
	"github.com/aluzed/grapi/schema"
)

func (s *Server) uidRoutes() {
	s.authed("POST /content-manager/uid/generate", s.uidGenerate)
	s.authed("POST /content-manager/uid/check-availability", s.uidCheck)
}

type uidBody struct {
	ContentTypeUID string         `json:"contentTypeUID"`
	Field          string         `json:"field"`
	Data           map[string]any `json:"data"`
	Value          string         `json:"value"`
	Locale         string         `json:"locale"`
}

// uidField checks that the user may write the content type and returns its
// uid attribute.
func (s *Server) uidField(r *http.Request, u *User, b *uidBody) (*schema.ContentType, *schema.Attribute, error) {
	if err := decode(r, b); err != nil {
		return nil, nil, err
	}
	ct, ok := s.set.ContentType(schema.UID(b.ContentTypeUID))
	if !ok || !contains(s.ContentManagerTypes(), b.ContentTypeUID) {
		return nil, nil, errNotFound
	}
	attr, ok := ct.Attributes.Get(b.Field)
	if !ok || attr.Type != schema.TypeUID {
		return nil, nil, validation([]FieldError{{Path: []string{"field"}, Message: "field must be a uid attribute"}})
	}
	a, err := s.Ability(r.Context(), u)
	if err != nil {
		return nil, nil, err
	}
	if !a.Can(actionCreate, b.ContentTypeUID, "") && !a.Can(actionUpdate, b.ContentTypeUID, "") {
		return nil, nil, errForbidden
	}
	return ct, attr, nil
}

func slugOptions(attr *schema.Attribute) slug.Options {
	o := slug.Defaults()
	if len(attr.Options) == 0 {
		return o
	}
	var raw struct {
		Separator          *string     `json:"separator"`
		Lowercase          *bool       `json:"lowercase"`
		Decamelize         *bool       `json:"decamelize"`
		CustomReplacements [][2]string `json:"customReplacements"`
	}
	if json.Unmarshal(attr.Options, &raw) != nil {
		return o
	}
	if raw.Separator != nil {
		o.Separator = *raw.Separator
	}
	if raw.Lowercase != nil {
		o.Lowercase = *raw.Lowercase
	}
	if raw.Decamelize != nil {
		o.Decamelize = *raw.Decamelize
	}
	o.CustomReplacements = raw.CustomReplacements
	return o
}

// taken lists the values of field starting with base, drafts and
// publications alike, in locale when the type is localized.
func (s *Server) taken(ctx context.Context, ct *schema.ContentType, field, base, locale string) (map[string]bool, error) {
	where := map[string]any{field: map[string]any{"$startsWith": base}}
	if ct.Localized() && locale != "" {
		where["locale"] = locale
	}
	rows, err := s.e.FindMany(ctx, string(ct.UID), query.Params{Where: where, Select: []string{field}})
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, row := range rows {
		out[getString(row, field)] = true
	}
	return out, nil
}

func (s *Server) uniqueUID(ctx context.Context, ct *schema.ContentType, field, base, locale string) (string, error) {
	used, err := s.taken(ctx, ct, field, base, locale)
	if err != nil {
		return "", err
	}
	if !used[base] {
		return base, nil
	}
	for i := 1; ; i++ {
		if c := base + "-" + strconv.Itoa(i); !used[c] {
			return c, nil
		}
	}
}

func (s *Server) uidGenerate(w http.ResponseWriter, r *http.Request, u *User) error {
	var b uidBody
	ct, attr, err := s.uidField(r, u, &b)
	if err != nil {
		return err
	}
	source := ct.Info.SingularName
	if attr.TargetField != "" {
		if v, ok := b.Data[attr.TargetField].(string); ok && v != "" {
			source = v
		}
	}
	base := slug.Make(source, slugOptions(attr))
	uid, err := s.uniqueUID(r.Context(), ct, b.Field, base, b.Locale)
	if err != nil {
		return err
	}
	return data(w, uid)
}

func (s *Server) uidCheck(w http.ResponseWriter, r *http.Request, u *User) error {
	var b uidBody
	ct, _, err := s.uidField(r, u, &b)
	if err != nil {
		return err
	}
	used, err := s.taken(r.Context(), ct, b.Field, b.Value, b.Locale)
	if err != nil {
		return err
	}
	out := map[string]any{"isAvailable": !used[b.Value], "suggestion": nil}
	if used[b.Value] {
		if out["suggestion"], err = s.uniqueUID(r.Context(), ct, b.Field, b.Value, b.Locale); err != nil {
			return err
		}
	}
	return data(w, out)
}
