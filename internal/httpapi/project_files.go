package httpapi

import (
	"context"
	"image"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/aluzed/grapi/internal/media"
	"github.com/aluzed/grapi/internal/media/provider"
)

// StoreProjectFile stores an admin logo with the upload provider, as the
// project-settings service does: the file info (name, hash, url, width,
// height, ext, size in KB, provider), without a media library entry.
func (s *Server) StoreProjectFile(ctx context.Context, src, filename, mimeType string) (map[string]any, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	hash := media.GenerateFileName(nodeBasename(filename, ext))
	url, meta, err := s.store(ctx, src, hash, ext, mimeType, "")
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(src)
	if err != nil {
		return nil, err
	}
	_, name := s.provider()
	out := map[string]any{"name": filename, "hash": hash, "url": url, "ext": ext, "size": math.Round(float64(st.Size())/1000*100) / 100, "provider": name, "width": nil, "height": nil}
	if meta != nil {
		out["provider_metadata"] = meta
	}
	if f, err := os.Open(src); err == nil { //nolint:gosec // a temporary upload file
		if cfg, _, err := image.DecodeConfig(f); err == nil {
			out["width"], out["height"] = cfg.Width, cfg.Height
		}
		_ = f.Close()
	}
	return out, nil
}

// DeleteProjectFile removes a logo stored by StoreProjectFile, when the
// current provider stored it.
func (s *Server) DeleteProjectFile(ctx context.Context, file map[string]any) error {
	p, name := s.provider()
	if prov, _ := file["provider"].(string); prov != name {
		return nil
	}
	hash, _ := file["hash"].(string)
	ext, _ := file["ext"].(string)
	meta, _ := file["provider_metadata"].(map[string]any)
	return p.Delete(ctx, &provider.File{Hash: hash, Ext: ext, Metadata: meta})
}
