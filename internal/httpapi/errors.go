package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/internal/document"
)

// APIError is an error answered in Strapi's format:
// {"data": null, "error": {status, name, message, details}}.
type APIError struct {
	Status  int
	Name    string
	Message string
	Details any
}

func (e *APIError) Error() string { return e.Message }

func newError(status int, name, msg string) *APIError {
	return &APIError{Status: status, Name: name, Message: msg}
}

func validationError(msg string, details any) *APIError {
	return &APIError{Status: http.StatusBadRequest, Name: "ValidationError", Message: msg, Details: details}
}

var (
	errNotFound     = newError(http.StatusNotFound, "NotFoundError", "Not Found")
	errForbidden    = newError(http.StatusForbidden, "ForbiddenError", "Forbidden")
	errUnauthorized = newError(http.StatusUnauthorized, "UnauthorizedError", "Missing or invalid credentials")
)

// toAPIError maps the errors of the lower layers to Strapi's error classes.
func toAPIError(err error) *APIError {
	var api *APIError
	if errors.As(err, &api) {
		return api
	}
	var ve *document.ValidationError
	if errors.As(err, &ve) {
		return &APIError{Status: 400, Name: "ValidationError", Message: ve.Msg, Details: ve.Details}
	}
	var pe *document.PaginationError
	if errors.As(err, &pe) {
		return &APIError{Status: 400, Name: "PaginationError", Message: pe.Msg}
	}
	var re *query.RelationError
	if errors.As(err, &re) {
		return &APIError{Status: 400, Name: "ValidationError", Message: re.Msg}
	}
	if errors.Is(err, document.ErrApplication) {
		return &APIError{Status: 400, Name: "ApplicationError", Message: err.Error()}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(body)
}

func writeError(w http.ResponseWriter, err error, log *slog.Logger) {
	api := toAPIError(err)
	if api == nil {
		log.Error("request failed", "error", err)
		api = newError(http.StatusInternalServerError, "InternalServerError", "Internal Server Error")
	} else {
		log.Debug("request rejected", "status", api.Status, "name", api.Name, "message", api.Message)
	}
	body := map[string]any{"status": api.Status, "name": api.Name, "message": api.Message}
	if api.Status < http.StatusInternalServerError {
		body["details"] = api.Details
		if api.Details == nil {
			body["details"] = map[string]any{}
		}
	}
	writeJSON(w, api.Status, map[string]any{"data": nil, "error": body})
}
