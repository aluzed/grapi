package httpapi

import (
	"fmt"
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

// check is validateFile: the type read from the content comes first, the
// declared one only when the content says nothing.
func (c UploadSecurity) check(f *inputFile) error {
	if !c.set() {
		return nil
	}
	declared := f.declared
	if f.detected == "" && (declared == "" || declared == "application/octet-stream") && (len(c.AllowedTypes) > 0 || len(c.DeniedTypes) > 0) {
		return validationError("Cannot verify file type for security reasons", nil)
	}
	mime := f.detected
	if mime == "" {
		mime = declared
	}
	if mime != "" && !c.allows(mime) {
		return validationError(fmt.Sprintf("File type '%s' is not allowed", mime), nil)
	}
	return nil
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
