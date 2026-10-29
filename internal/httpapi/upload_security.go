package httpapi

import (
	"fmt"
	"mime"
	"path"
	"regexp"
	"strings"
)

// UploadSecurity is the upload plugin's security config: the mime types an
// upload may have. A nil list is not set; Strapi tells it from an empty one.
type UploadSecurity struct {
	AllowedTypes []string
	DeniedTypes  []string
	// Invalid is the configuration error, reported on each upload as Strapi
	// does (allowedTypes or deniedTypes that are not lists of strings).
	Invalid string
}

func (c UploadSecurity) set() bool { return c.AllowedTypes != nil || c.DeniedTypes != nil }

func matchesMimePattern(mime string, patterns []string) bool {
	mime = strings.ToLower(mime)
	for _, p := range patterns {
		p = strings.ToLower(p)
		if strings.Contains(p, "*") {
			// Only * is a wildcard. Strapi does not escape the rest of the
			// pattern; for the usual ones (image/*) it is the same.
			re, err := regexp.Compile("^" + strings.ReplaceAll(regexp.QuoteMeta(p), `\*`, ".*") + "$")
			if err == nil && re.MatchString(mime) {
				return true
			}
			continue
		}
		if p == mime {
			return true
		}
	}
	return false
}

func (c UploadSecurity) allows(mime string) bool {
	if mime == "" {
		return false
	}
	if len(c.DeniedTypes) > 0 && matchesMimePattern(mime, c.DeniedTypes) {
		return false
	}
	if len(c.AllowedTypes) > 0 {
		return matchesMimePattern(mime, c.AllowedTypes)
	}
	return true
}

// baseType is a media type without its parameters, lower case:
// "text/html; charset=utf-8" is text/html.
func baseType(t string) string {
	if mt, _, err := mime.ParseMediaType(t); err == nil {
		return mt
	}
	t, _, _ = strings.Cut(t, ";")
	return strings.ToLower(strings.TrimSpace(t))
}

// claimsBinary is a declared type whose files have a signature: when the
// content shows none, the declaration cannot be true.
func claimsBinary(t string) bool {
	if t == "image/svg+xml" {
		return false
	}
	return strings.HasPrefix(t, "image/") || strings.HasPrefix(t, "video/") || strings.HasPrefix(t, "audio/")
}

// Active content is served according to its stored extension by the local
// provider. A caller-supplied MIME type must not bypass the policy for that
// extension. JavaScript and XML have commonly used MIME aliases.
func (c UploadSecurity) checkExtension(ext string) error {
	if !c.set() {
		return nil
	}
	var types []string
	switch strings.ToLower(ext) {
	case ".html", ".htm", ".shtml":
		types = []string{"text/html"}
	case ".svg", ".svgz":
		types = []string{"image/svg+xml"}
	case ".xhtml", ".xht":
		types = []string{"application/xhtml+xml"}
	case ".js", ".mjs":
		types = []string{"application/javascript", "text/javascript"}
	case ".xml", ".xsl":
		types = []string{"application/xml", "text/xml"}
	}
	allowed := len(c.AllowedTypes) == 0
	for _, t := range types {
		if matchesMimePattern(t, c.DeniedTypes) {
			return validationError(fmt.Sprintf("File type '%s' is not allowed", t), nil)
		}
		allowed = allowed || matchesMimePattern(t, c.AllowedTypes)
	}
	if len(types) > 0 && !allowed {
		return validationError(fmt.Sprintf("File type '%s' is not allowed", types[0]), nil)
	}
	return nil
}

// check is validateFile: the type read from the content comes first, the
// declared one only when the content says nothing. Unlike Strapi, a
// declared image, video or audio type that the content does not show is
// refused as unverifiable.
func (c UploadSecurity) check(f *inputFile) error {
	if !c.set() {
		return nil
	}
	declared := baseType(f.declared)
	if f.detected == "" && claimsBinary(declared) && (len(c.AllowedTypes) > 0 || len(c.DeniedTypes) > 0) {
		return validationError("Cannot verify file type for security reasons", nil)
	}
	if f.detected == "" && (declared == "" || declared == "application/octet-stream") && (len(c.AllowedTypes) > 0 || len(c.DeniedTypes) > 0) {
		return validationError("Cannot verify file type for security reasons", nil)
	}
	t := baseType(f.detected)
	if t == "" {
		t = declared
	}
	if t != "" && !c.allows(t) {
		return validationError(fmt.Sprintf("File type '%s' is not allowed", t), nil)
	}
	return c.checkExtension(path.Ext(f.filename))
}

// enforceUploadSecurity keeps the files the security config allows, with
// their fileInfo; when none is left, the first refusal is the answer.
func (s *Server) enforceUploadSecurity(files []*inputFile, infos []map[string]any) ([]*inputFile, []map[string]any, error) {
	sec := s.cfg.Upload.Security
	if sec.Invalid != "" {
		return nil, nil, appError(sec.Invalid)
	}
	if !sec.set() {
		return files, infos, nil
	}
	var keptFiles []*inputFile
	var keptInfos []map[string]any
	var first error
	for i, f := range files {
		if err := sec.check(f); err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		keptFiles = append(keptFiles, f)
		if i < len(infos) {
			keptInfos = append(keptInfos, infos[i])
		}
	}
	if len(keptFiles) == 0 && first != nil {
		return nil, nil, first
	}
	return keptFiles, keptInfos, nil
}
