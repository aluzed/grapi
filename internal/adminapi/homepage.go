package adminapi

import (
	"net/http"
	"sort"
	"strings"

	"github.com/aluzed/grapi/internal/document"
	"github.com/aluzed/grapi/schema"
)

// RecentDocument is a line of the home page widgets.
type RecentDocument struct {
	UID         string `json:"uid"`
	Kind        string `json:"kind"`
	DisplayName string `json:"displayName"`
	DocumentID  string `json:"documentId"`
	Title       string `json:"title"`
	Locale      any    `json:"locale"`
	At          string `json:"at"`
}

func (s *Server) homepageRoutes() {
	s.authed("GET /homepage", s.homepage)
}

// homepage counts the documents of the readable types and lists the last
// edited and the last published ones.
func (s *Server) homepage(w http.ResponseWriter, r *http.Request, u *User) error {
	ctx := r.Context()
	a, err := s.Ability(ctx, u)
	if err != nil {
		return err
	}
	type count struct {
		UID         string `json:"uid"`
		Kind        string `json:"kind"`
		DisplayName string `json:"displayName"`
		Total       int64  `json:"total"`
	}
	counts := []count{}
	var edited, published []RecentDocument
	const n = 5
	for _, uid := range s.ContentManagerTypes() {
		filter, ok := a.Filter(actionRead, uid)
		if !ok || strings.HasPrefix(uid, "plugin::") {
			continue
		}
		ct, _ := s.set.ContentType(schema.UID(uid))
		m, err := s.model(ctx, uid, a)
		if err != nil || m == nil {
			return err
		}
		base := document.Params{}
		if filter != nil {
			base["filters"] = scoped(filter)
		}
		if ct.Localized() {
			base["locale"] = "*"
		}
		total, err := s.docs.Count(ctx, uid, base)
		if err != nil {
			return err
		}
		counts = append(counts, count{UID: uid, Kind: string(ct.Kind), DisplayName: ct.Info.DisplayName, Total: total})
		recent := func(status, sortBy string, into *[]RecentDocument) error {
			p := document.Params{"sort": sortBy + ":desc", "limit": n}
			for k, v := range base {
				p[k] = v
			}
			if ct.DraftAndPublish() {
				p["status"] = status
			} else if status == "published" {
				return nil
			}
			rows, err := s.docs.FindMany(ctx, uid, p)
			if err != nil {
				return err
			}
			for _, row := range rows {
				d := toMap(row)
				title, _ := d[m.MainField].(string)
				id, _ := d[schema.AttrDocumentID].(string)
				at, _ := d[sortBy].(string)
				*into = append(*into, RecentDocument{UID: uid, Kind: string(ct.Kind), DisplayName: ct.Info.DisplayName, DocumentID: id, Title: title, Locale: d["locale"], At: at})
			}
			return nil
		}
		if err := recent("draft", "updatedAt", &edited); err != nil {
			return err
		}
		if err := recent("published", "publishedAt", &published); err != nil {
			return err
		}
	}
	latest := func(list []RecentDocument) []RecentDocument {
		sort.SliceStable(list, func(i, j int) bool { return list[i].At > list[j].At })
		if len(list) > n {
			list = list[:n]
		}
		if list == nil {
			list = []RecentDocument{}
		}
		return list
	}
	sort.Slice(counts, func(i, j int) bool { return counts[i].DisplayName < counts[j].DisplayName })
	return data(w, map[string]any{"counts": counts, "lastEdited": latest(edited), "lastPublished": latest(published)})
}
