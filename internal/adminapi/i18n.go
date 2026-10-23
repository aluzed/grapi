package adminapi

import (
	"context"
	"net/http"
	"strconv"

	"github.com/aluzed/grapi/internal/auth/permissions"
	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/internal/db/store"
	"github.com/aluzed/grapi/internal/document"
	"github.com/aluzed/grapi/internal/isolocales"
	"github.com/aluzed/grapi/schema"
)

const localeUID = "plugin::i18n.locale"

var i18nScope = store.Scope{Type: "plugin", Name: "i18n"}

func (s *Server) i18nRoutes() {
	if _, ok := s.set.ContentType(localeUID); !ok {
		return
	}
	s.allowed("GET /i18n/iso-locales", "plugin::i18n.locale.read", s.isoLocales)
	s.allowed("POST /i18n/locales", "plugin::i18n.locale.create", s.createLocale)
	s.allowed("PUT /i18n/locales/{id}", "plugin::i18n.locale.update", s.updateLocale)
	s.allowed("DELETE /i18n/locales/{id}", "plugin::i18n.locale.delete", s.deleteLocale)
}

func (s *Server) isoLocales(w http.ResponseWriter, _ *http.Request, _ *User, _ *permissions.Ability) error {
	return data(w, isolocales.All())
}

func (s *Server) setDefaultLocale(ctx context.Context, code string) error {
	return s.store.Set(ctx, i18nScope, "default_locale", code)
}

func (s *Server) createLocale(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	var b struct {
		Code      string `json:"code"`
		Name      string `json:"name"`
		IsDefault bool   `json:"isDefault"`
	}
	if err := decode(r, &b); err != nil {
		return err
	}
	iso, ok := isolocales.Name(b.Code)
	if !ok {
		return validation([]FieldError{{Path: []string{"code"}, Message: "code must be one of the ISO locales"}})
	}
	ctx := r.Context()
	if n, err := s.e.Count(ctx, localeUID, query.Params{Where: map[string]any{"code": b.Code}}); err != nil {
		return err
	} else if n > 0 {
		return appError("This locale already exists")
	}
	name := b.Name
	if name == "" {
		name = iso
	}
	row, err := s.e.Create(ctx, localeUID, query.Data{"code": b.Code, "name": name}, query.Params{})
	if err != nil {
		return err
	}
	if b.IsDefault {
		if err := s.setDefaultLocale(ctx, b.Code); err != nil {
			return err
		}
	}
	out := toMap(row)
	out["isDefault"] = b.IsDefault
	writeJSON(w, http.StatusCreated, map[string]any{"data": out})
	return nil
}

func (s *Server) localeByPath(r *http.Request) (query.Row, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return nil, errNotFound
	}
	row, err := s.e.FindOne(r.Context(), localeUID, query.Params{Where: map[string]any{"id": id}})
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, &Error{Status: http.StatusNotFound, Name: "NotFoundError", Message: "Locale not found"}
	}
	return row, nil
}

func (s *Server) updateLocale(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	row, err := s.localeByPath(r)
	if err != nil {
		return err
	}
	var b struct {
		Name      *string `json:"name"`
		IsDefault bool    `json:"isDefault"`
	}
	if err := decode(r, &b); err != nil {
		return err
	}
	ctx := r.Context()
	if b.Name != nil && *b.Name != "" {
		if row, err = s.e.Update(ctx, localeUID, map[string]any{"id": get(row, "id")}, query.Data{"name": *b.Name}, query.Params{}); err != nil {
			return err
		}
	}
	code := getString(row, "code")
	if b.IsDefault {
		if err := s.setDefaultLocale(ctx, code); err != nil {
			return err
		}
	}
	out := toMap(row)
	out["isDefault"] = s.docs.DefaultLocale(ctx) == code
	return data(w, out)
}

// deleteLocale removes a locale and, as Strapi does, every entry written in
// it. The default locale cannot be deleted.
func (s *Server) deleteLocale(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	row, err := s.localeByPath(r)
	if err != nil {
		return err
	}
	ctx := r.Context()
	code := getString(row, "code")
	if s.docs.DefaultLocale(ctx) == code {
		return appError("Cannot delete the default locale")
	}
	for uid, ct := range s.set.ContentTypes.All() {
		if !ct.Localized() {
			continue
		}
		rows, err := s.docs.FindMany(ctx, uid, document.Params{"locale": code, "fields": []any{schema.AttrDocumentID}})
		if err != nil {
			return err
		}
		for _, d := range rows {
			id, _ := toMap(d)[schema.AttrDocumentID].(string)
			if _, err := s.docs.Delete(ctx, uid, id, document.Params{"locale": code}); err != nil {
				return asValidation(err)
			}
		}
	}
	if _, err := s.e.Delete(ctx, localeUID, map[string]any{"id": get(row, "id")}, query.Params{}); err != nil {
		return err
	}
	return data(w, toMap(row))
}
