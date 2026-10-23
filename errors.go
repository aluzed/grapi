package grapi

import (
	"net/http"

	"github.com/aluzed/grapi/internal/httpapi"
)

// APIError is an error answered in Strapi's format, {"data": null, "error":
// {status, name, message, details}}: a route handler, a policy or a hook
// that returns one, wrapped or not, gets that answer. errors.As finds it.
// Error builds one from an HTTP status, as ctx.badRequest and the like.
type APIError = httpapi.APIError

func strapiError(status int, name, def, message string, details []any) *APIError {
	if message == "" {
		message = def
	}
	e := &APIError{Status: status, Name: name, Message: message, Details: map[string]any{}}
	if len(details) > 0 {
		e.Details = details[0]
	}
	return e
}

// The error classes of @strapi/utils; an empty message is Strapi's default.

// ApplicationError is a 400 "An application error occurred".
func ApplicationError(message string, details ...any) *APIError {
	return strapiError(http.StatusBadRequest, "ApplicationError", "An application error occurred", message, details)
}

// ValidationError is a 400; details usually holds {errors: [{path, message, name}]}.
func ValidationError(message string, details ...any) *APIError {
	return strapiError(http.StatusBadRequest, "ValidationError", "An application error occurred", message, details)
}

// PaginationError is a 400 "Invalid pagination".
func PaginationError(message string, details ...any) *APIError {
	return strapiError(http.StatusBadRequest, "PaginationError", "Invalid pagination", message, details)
}

// NotFoundError is a 404 "Entity not found".
func NotFoundError(message string, details ...any) *APIError {
	return strapiError(http.StatusNotFound, "NotFoundError", "Entity not found", message, details)
}

// ForbiddenError is a 403 "Forbidden access".
func ForbiddenError(message string, details ...any) *APIError {
	return strapiError(http.StatusForbidden, "ForbiddenError", "Forbidden access", message, details)
}

// PolicyError is a 403 "Policy Failed".
func PolicyError(message string, details ...any) *APIError {
	return strapiError(http.StatusForbidden, "PolicyError", "Policy Failed", message, details)
}

// UnauthorizedError is a 401 "Unauthorized".
func UnauthorizedError(message string, details ...any) *APIError {
	return strapiError(http.StatusUnauthorized, "UnauthorizedError", "Unauthorized", message, details)
}

// PayloadTooLargeError is a 413 "Entity too large".
func PayloadTooLargeError(message string, details ...any) *APIError {
	return strapiError(http.StatusRequestEntityTooLarge, "PayloadTooLargeError", "Entity too large", message, details)
}

// RateLimitError is a 429 "Too many requests, please try again later.".
func RateLimitError(message string, details ...any) *APIError {
	return strapiError(http.StatusTooManyRequests, "RateLimitError", "Too many requests, please try again later.", message, details)
}

// NotImplementedError is a 501 "This feature is not implemented yet".
func NotImplementedError(message string, details ...any) *APIError {
	return strapiError(http.StatusNotImplemented, "NotImplementedError", "This feature is not implemented yet", message, details)
}
