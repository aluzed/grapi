// Package logging is grapi's logger: Strapi's levels on log/slog, text in
// development and JSON in production.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// LevelHTTP is Strapi's http level, between debug and info: one line per
// request.
const LevelHTTP = slog.Level(-2)

// DefaultLevel is Strapi's default, http.
const DefaultLevel = LevelHTTP

// ParseLevel reads a Strapi (winston) or slog level name.
func ParseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "error":
		return slog.LevelError, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "info":
		return slog.LevelInfo, nil
	case "http", "":
		return LevelHTTP, nil
	case "verbose", "debug", "silly":
		return slog.LevelDebug, nil
	}
	return 0, fmt.Errorf("unknown log level %q: use error, warn, info, http or debug", s)
}

// Format is the output format: "text" or "json".
type Format string

// Formats.
const (
	Text Format = "text"
	JSON Format = "json"
)

// DefaultFormat is JSON in production, text elsewhere.
func DefaultFormat(environment string) Format {
	if environment == "production" {
		return JSON
	}
	return Text
}

// ParseFormat reads a format name; empty gives def.
func ParseFormat(s string, def Format) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return def, nil
	case "text":
		return Text, nil
	case "json":
		return JSON, nil
	}
	return "", fmt.Errorf("unknown log format %q: use text or json", s)
}

func levelName(l slog.Level) string {
	if l == LevelHTTP {
		return "HTTP"
	}
	return l.String()
}

// New returns a logger writing to w.
func New(w io.Writer, format Format, level slog.Leveler) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level, ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
		if len(groups) == 0 && a.Key == slog.LevelKey {
			if l, ok := a.Value.Any().(slog.Level); ok {
				return slog.String(slog.LevelKey, levelName(l))
			}
		}
		return a
	}}
	if format == JSON {
		return slog.New(slog.NewJSONHandler(w, opts))
	}
	return slog.New(slog.NewTextHandler(w, opts))
}
