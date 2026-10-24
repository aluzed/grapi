package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/aluzed/grapi/internal/auth/users"
)

// The admin's preview hands the front a token that reads one document,
// drafts included, for an hour.
const previewTokenType = "grapi-preview"

// SignPreviewToken signs a preview token for documentID of uid.
func SignPreviewToken(secret, uid, documentID string, now time.Time) (string, error) {
	return users.Sign([]users.KV{{Key: "typ", Value: previewTokenType}, {Key: "uid", Value: uid}, {Key: "documentId", Value: documentID}}, secret, "HS256", "1h", now)
}

type previewGrant struct{ uid, documentID string }

// previewToken reads a preview token: ok is false for any other token.
// An expired or forged preview token is a 401.
func (a *authenticator) previewToken(token string) (grant *previewGrant, ok bool, err error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || a.previewSecret == "" {
		return nil, false, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, false, nil
	}
	var claims struct {
		Typ string `json:"typ"`
	}
	if json.Unmarshal(raw, &claims) != nil || claims.Typ != previewTokenType {
		return nil, false, nil
	}
	payload, err := users.Verify(token, a.previewSecret, "HS256", time.Now())
	if err != nil {
		return nil, true, errUnauthorized
	}
	uid, _ := payload["uid"].(string)
	id, _ := payload["documentId"].(string)
	if uid == "" || id == "" {
		return nil, true, errUnauthorized
	}
	return &previewGrant{uid: uid, documentID: id}, true, nil
}

func previewTokenOf(r *http.Request) string {
	if t := bearer(r); t != "" {
		return t
	}
	return r.URL.Query().Get("previewToken")
}

// restrictToPreview keeps a preview token to its document: findOne of that
// document, or find filtered on it.
func restrictToPreview(rt *route, rc *requestContext) error {
	g := rc.auth.preview
	if rt.uid != g.uid {
		return errForbidden
	}
	switch rt.action {
	case "findOne":
		if rc.params["id"] != g.documentID {
			return errForbidden
		}
	case "find":
		only := map[string]any{"documentId": map[string]any{"$eq": g.documentID}}
		if f, ok := rc.query["filters"]; ok {
			rc.query["filters"] = map[string]any{"$and": []any{f, only}}
		} else {
			rc.query["filters"] = only
		}
	default:
		return errForbidden
	}
	return nil
}
