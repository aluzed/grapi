package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/aluzed/grapi/internal/db/query"
	"github.com/aluzed/grapi/internal/media"
)

const (
	fileUID   = "plugin::upload.file"
	folderUID = "plugin::upload.folder"
)

// UploadConfig is the upload plugin config and where the local provider
// writes.
type UploadConfig struct {
	// PublicDir is server.dirs.public, resolved; files go to <PublicDir>/uploads.
	PublicDir string
	// SizeLimit in bytes (1 GB by default).
	SizeLimit int64
	// Breakpoints are the responsive formats, largest first.
	Breakpoints []Breakpoint
}

// Breakpoint is one responsive format.
type Breakpoint struct {
	Name string
	Size int
}

// DefaultBreakpoints are the upload plugin's.
var DefaultBreakpoints = []Breakpoint{{"large", 1000}, {"medium", 750}, {"small", 500}}

type uploadSettings struct {
	SizeOptimization     bool `json:"sizeOptimization"`
	ResponsiveDimensions bool `json:"responsiveDimensions"`
	AutoOrientation      bool `json:"autoOrientation"`
}

func (s *Server) addUploadRoutes() {
	if _, ok := s.set.ContentType(fileUID); !ok {
		return
	}
	base := strings.TrimPrefix(s.cfg.Prefix, "/")
	uid := "plugin::upload.content-api"
	s.routes = append(s.routes,
		route{"POST", []string{base, "upload"}, uid, "upload", s.upload},
		route{"GET", []string{base, "upload", "files"}, uid, "find", s.uploadFind},
		route{"GET", []string{base, "upload", "files", ":id"}, uid, "findOne", s.uploadFindOne},
		route{"DELETE", []string{base, "upload", "files", ":id"}, uid, "destroy", s.uploadDestroy},
	)
}

// inputFile is a multipart file saved in the request's working directory.
type inputFile struct {
	filename string
	declared string
	detected string
	size     int64
	path     string
}

type multipartRequest struct {
	files  []*inputFile
	fields map[string][]string
}

func readMultipart(r *http.Request, dir string) (*multipartRequest, error) {
	out := &multipartRequest{fields: map[string][]string{}}
	mr, err := r.MultipartReader()
	if err != nil {
		return out, nil //nolint:nilerr // a request without multipart has no files
	}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, validationError("Invalid multipart body", nil)
		}
		name := part.FormName()
		if part.FileName() == "" {
			b, err := io.ReadAll(io.LimitReader(part, 1<<20))
			if err != nil {
				return nil, err
			}
			out.fields[name] = append(out.fields[name], string(b))
			continue
		}
		if name != "files" {
			_, _ = io.Copy(io.Discard, part)
			continue
		}
		f, err := os.CreateTemp(dir, "upload-")
		if err != nil {
			return nil, err
		}
		n, err := io.Copy(f, part)
		_ = f.Close()
		if err != nil {
			return nil, err
		}
		out.files = append(out.files, &inputFile{filename: part.FileName(), declared: part.Header.Get("Content-Type"), size: n, path: f.Name()})
	}
}

// detectMime stands for file-type: only binary signatures count.
func detectMime(p string) string {
	f, err := os.Open(p) //nolint:gosec // a temporary upload file
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, 4100)
	n, _ := io.ReadFull(f, head)
	t := http.DetectContentType(head[:n])
	t, _, _ = strings.Cut(t, ";")
	if t == "application/octet-stream" || strings.HasPrefix(t, "text/") {
		return ""
	}
	return t
}

var (
	reservedFilename = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1F]`)
	windowsReserved  = regexp.MustCompile(`(?i)^(con|prn|aux|nul|com\d|lpt\d)$`)
)

func isValidFilename(s string) bool {
	return s != "" && len(s) <= 255 && !reservedFilename.MatchString(s) && !windowsReserved.MatchString(s) && s != "." && s != ".."
}

var mimeExtensions = map[string]string{"image/jpeg": "jpeg", "image/png": "png", "image/gif": "gif", "image/webp": "webp", "image/svg+xml": "svg", "application/pdf": "pdf"}

// extension is mime-types' extension(type).
func extension(t string) string {
	if e, ok := mimeExtensions[t]; ok {
		return e
	}
	if exts, _ := mime.ExtensionsByType(t); len(exts) > 0 {
		return strings.TrimPrefix(exts[0], ".")
	}
	return "false"
}

// nodeBasename is path.basename(p, ext): the suffix goes only when it
// matches and leaves something.
func nodeBasename(p, ext string) string {
	b := path.Base(strings.ReplaceAll(p, "\\", "/"))
	if ext != "" && strings.HasSuffix(b, ext) && b != ext {
		return strings.TrimSuffix(b, ext)
	}
	return b
}

func bytesToHumanReadable(n int64) string {
	sizes := []string{"Bytes", "KB", "MB", "GB", "TB", "PB"}
	if n == 0 {
		return "0 Bytes"
	}
	i := int(math.Floor(math.Log(float64(n)) / math.Log(1000)))
	return fmt.Sprintf("%d %s", int64(math.Round(float64(n)/math.Pow(1000, float64(i)))), sizes[i])
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
	dir, err := os.MkdirTemp("", "grapi-upload-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	req, err := readMultipart(r, dir)
	if err != nil {
		return err
	}
	if len(req.files) == 0 || len(req.files) == 1 && req.files[0].size == 0 {
		if r.URL.Query().Get("id") != "" {
			return newError(http.StatusNotImplemented, "NotImplementedError", "Updating a file's info is not supported yet")
		}
		return validationError("Files are empty", nil)
	}
	if r.URL.Query().Get("id") != "" {
		return newError(http.StatusNotImplemented, "NotImplementedError", "Replacing a file is not supported yet")
	}
	return s.uploadFiles(w, r, rc, req, dir)
}

func (s *Server) uploadFiles(w http.ResponseWriter, r *http.Request, rc *requestContext, req *multipartRequest, dir string) error {
	ctx := r.Context()
	for _, f := range req.files {
		f.detected = detectMime(f.path)
	}
	infos, err := fileInfos(req.fields["fileInfo"], len(req.files))
	if err != nil {
		return err
	}
	folder, err := s.apiUploadFolder(ctx)
	if err != nil {
		return err
	}
	var settings uploadSettings
	if err := s.uploadSettings(ctx, &settings); err != nil {
		return err
	}
	folderID, _ := folder.Get("id")
	folderPath, _ := folder.Get("path")
	metas := map[string]string{}
	for _, k := range []string{"ref", "refId", "field", "path"} {
		if v := req.fields[k]; len(v) > 0 {
			metas[k] = v[0]
		}
	}
	out := make([]any, 0, len(req.files))
	for i, f := range req.files {
		info := infos[i]
		info["folder"] = folderID
		row, err := s.uploadOne(ctx, f, info, folderPath, metas, settings, dir)
		if err != nil {
			return err
		}
		row.Set("isUrlSigned", false)
		out = append(out, s.sanitizeOutput(row, s.schemaOf(fileUID), rc.auth))
	}
	writeJSON(w, http.StatusCreated, out)
	return nil
}

// fileInfos parses and validates the fileInfo fields (validateUploadBody,
// strict off): one object per file, focalPoint null by default.
func fileInfos(raw []string, n int) ([]map[string]any, error) {
	var parsed []any
	for _, s := range raw {
		var v any
		if err := json.Unmarshal([]byte(s), &v); err != nil {
			return nil, fmt.Errorf("fileInfo: %w", err)
		}
		if list, ok := v.([]any); ok {
			parsed = append(parsed, list...)
		} else {
			parsed = append(parsed, v)
		}
	}
	out := make([]map[string]any, n)
	for i := range out {
		info := map[string]any{}
		if i < len(parsed) {
			if m, ok := parsed[i].(map[string]any); ok {
				for k, v := range m {
					switch k {
					case "name", "alternativeText", "caption", "focalPoint":
						info[k] = v
					default:
						return nil, validationError("fileInfo field has unspecified keys: "+k, nil)
					}
				}
			}
		}
		if info["focalPoint"] == nil {
			info["focalPoint"] = nil
		}
		out[i] = info
	}
	return out, nil
}

func (s *Server) uploadSettings(ctx context.Context, v *uploadSettings) error {
	*v = uploadSettings{SizeOptimization: true, ResponsiveDimensions: true}
	var raw *string
	err := s.auth.db.QueryRow(ctx, `select "value" from "strapi_core_store_settings" where "key" = `+s.auth.db.Dialect.Placeholder(1), "plugin_upload_settings").Scan(&raw)
	if err != nil || raw == nil {
		return nil //nolint:nilerr // no stored settings: the plugin defaults
	}
	return json.Unmarshal([]byte(*raw), v)
}

// apiUploadFolder is the "API Uploads" folder, created on first use and
// remembered in the core store.
func (s *Server) apiUploadFolder(ctx context.Context) (query.Row, error) {
	var stored struct {
		ID any `json:"id"`
	}
	var raw *string
	key := "plugin_upload_api-folder"
	err := s.auth.db.QueryRow(ctx, `select "value" from "strapi_core_store_settings" where "key" = `+s.auth.db.Dialect.Placeholder(1), key).Scan(&raw)
	if err == nil && raw != nil {
		_ = json.Unmarshal([]byte(*raw), &stored)
	}
	if stored.ID != nil {
		folder, err := s.docs.E.FindOne(ctx, folderUID, query.Params{Where: map[string]any{"id": stored.ID}})
		if err != nil || folder != nil {
			return folder, err
		}
	}
	name := "API Uploads"
	for i := 1; ; i++ {
		n, err := s.docs.E.Count(ctx, folderUID, query.Params{Where: map[string]any{"name": name, "parent": nil}})
		if err != nil {
			return nil, err
		}
		if n == 0 {
			break
		}
		name = fmt.Sprintf("API Uploads (%d)", i)
	}
	var maxID *float64
	if err := s.auth.db.QueryRow(ctx, `select max("path_id") from "upload_folders"`).Scan(&maxID); err != nil {
		return nil, err
	}
	pathID := 1
	if maxID != nil {
		pathID = int(*maxID) + 1
	}
	folder, err := s.docs.E.Create(ctx, folderUID, query.Data{"name": name, "pathId": pathID, "path": "/" + strconv.Itoa(pathID)}, query.Params{})
	if err != nil {
		return nil, err
	}
	id, _ := folder.Get("id")
	value, _ := json.Marshal(map[string]any{"id": id})
	if stored.ID != nil || raw != nil {
		_, err = s.auth.db.Exec(ctx, `update "strapi_core_store_settings" set "value" = `+s.auth.db.Dialect.Placeholder(1)+` where "key" = `+s.auth.db.Dialect.Placeholder(2), string(value), key)
	} else {
		_, err = s.auth.db.Exec(ctx, `insert into "strapi_core_store_settings" ("key", "value", "type") values (`+s.auth.db.Dialect.Placeholder(1)+`, `+s.auth.db.Dialect.Placeholder(2)+`, 'object')`, key, string(value))
	}
	return folder, err
}

func (s *Server) uploadOne(ctx context.Context, f *inputFile, info map[string]any, folderPath any, metas map[string]string, settings uploadSettings, dir string) (query.Row, error) {
	if !isValidFilename(f.filename) {
		return nil, appError("File name contains invalid characters")
	}
	mimeType := f.detected
	if mimeType == "" && f.declared != "" && f.declared != "application/octet-stream" {
		mimeType = f.declared
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	ext := path.Ext(f.filename)
	if ext == "" {
		ext = "." + extension(mimeType)
	}
	usedName := f.filename
	if n, ok := info["name"].(string); ok && n != "" {
		usedName = n
	}
	usedName = norm.NFC.String(usedName)
	hash := media.GenerateFileName(nodeBasename(usedName, ext))
	file := query.Data{
		"name":            usedName,
		"alternativeText": info["alternativeText"],
		"caption":         info["caption"],
		"focalPoint":      info["focalPoint"],
		"folder":          info["folder"],
		"folderPath":      folderPath,
		"hash":            hash,
		"ext":             ext,
		"mime":            mimeType,
		"size":            media.BytesToKbytes(f.size),
	}
	if metas["refId"] != "" && metas["ref"] != "" && metas["field"] != "" {
		file["related"] = []any{map[string]any{"id": metas["refId"], "__type": metas["ref"], "__pivot": map[string]any{"field": metas["field"]}}}
	}
	src := f.path
	img, imgErr := media.Probe(src)
	isImage := imgErr == nil && map[string]bool{"jpeg": true, "png": true, "webp": true, "tiff": true, "svg": true, "gif": true, "avif": true}[img.Format]
	if isImage && (settings.SizeOptimization || settings.AutoOrientation) {
		quality := 100
		if settings.SizeOptimization {
			quality = 80
		}
		optimized := filepath.Join(dir, "optimized-"+hash)
		res, ok, err := media.Optimize(src, optimized, img, quality)
		if err != nil {
			return nil, appError("File is not a valid image")
		}
		if ok {
			src = optimized
			file["size"] = media.BytesToKbytes(res.Size)
		}
	}
	if s.cfg.Upload.SizeLimit > 0 {
		if kb, _ := file["size"].(float64); kb*1000 > float64(s.cfg.Upload.SizeLimit) {
			return nil, newError(http.StatusRequestEntityTooLarge, "PayloadTooLargeError", fmt.Sprintf("%s exceeds size limit of %s.", usedName, bytesToHumanReadable(s.cfg.Upload.SizeLimit)))
		}
	}
	if isImage {
		file["width"], file["height"] = nilIfZero(img.Width), nilIfZero(img.Height)
		if img.PageHeight > 0 {
			file["height"] = img.PageHeight
		}
	}
	url, err := s.storeLocal(src, hash, ext)
	if err != nil {
		return nil, err
	}
	file["url"] = url
	if isImage && media.CanResize(img.Format) {
		formats := map[string]any{}
		pageH := img.Height
		if img.PageHeight > 0 {
			pageH = img.PageHeight
		}
		add := func(key string, maxW, maxH int) error {
			fh := key + "_" + hash
			dst := filepath.Join(dir, fh)
			res, err := media.Resize(src, dst, img, maxW, maxH)
			if err != nil {
				return err
			}
			u, err := s.storeLocal(dst, fh, ext)
			if err != nil {
				return err
			}
			formats[key] = map[string]any{
				"name": key + "_" + usedName, "hash": fh, "ext": ext, "mime": mimeType, "path": nilIfEmpty(metas["path"]),
				"width": res.Width, "height": res.Height, "size": media.BytesToKbytes(res.Size), "sizeInBytes": res.Size, "url": u,
			}
			return nil
		}
		if img.Width > 245 || pageH > 156 {
			if err := add("thumbnail", 245, 156); err != nil {
				return nil, err
			}
		}
		if settings.ResponsiveDimensions {
			for _, bp := range s.cfg.Upload.Breakpoints {
				if bp.Size < img.Width || bp.Size < pageH {
					if err := add(bp.Name, bp.Size, bp.Size); err != nil {
						return nil, err
					}
				}
			}
		}
		if len(formats) > 0 {
			file["formats"] = formats
		}
	}
	file["provider"] = "local"
	return s.docs.E.Create(ctx, fileUID, file, query.Params{})
}

func nilIfZero(n int) any {
	if n == 0 {
		return nil
	}
	return n
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// storeLocal is the local provider: <public>/uploads/<hash><ext>.
func (s *Server) storeLocal(src, hash, ext string) (string, error) {
	dir := filepath.Join(s.cfg.Upload.PublicDir, "uploads")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	name := filepath.Base(hash + ext)
	in, err := os.Open(src) //nolint:gosec // a temporary upload file
	if err != nil {
		return "", err
	}
	defer func() { _ = in.Close() }()
	tmp, err := os.CreateTemp(dir, ".upload-")
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, name)); err != nil {
		return "", err
	}
	return "/uploads/" + name, nil
}

func (s *Server) uploadFind(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
	rc.model = s.schemaOf(fileUID)
	q, err := s.sanitizedQuery(rc)
	if err != nil {
		return err
	}
	qp, err := s.docs.TransformQuery(fileUID, userQuery(q))
	if err != nil {
		return err
	}
	rows, err := s.docs.E.FindMany(r.Context(), fileUID, qp)
	if err != nil {
		return err
	}
	out := make([]any, 0, len(rows))
	for _, row := range rows {
		row.Set("isUrlSigned", false)
		out = append(out, s.sanitizeOutput(row, rc.model, rc.auth))
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

var digits = regexp.MustCompile(`^\d+$`)

func (s *Server) resolveFileID(ctx context.Context, id string) (any, error) {
	if digits.MatchString(id) {
		return id, nil
	}
	row, err := s.docs.E.FindOne(ctx, fileUID, query.Params{Where: map[string]any{"documentId": id}, Select: []string{"id"}})
	if err != nil || row == nil {
		return nil, err
	}
	v, _ := row.Get("id")
	return v, nil
}

func (s *Server) uploadFindOne(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
	rc.model = s.schemaOf(fileUID)
	q, err := s.sanitizedQuery(rc)
	if err != nil {
		return err
	}
	id, err := s.resolveFileID(r.Context(), rc.params["id"])
	if err != nil {
		return err
	}
	var row query.Row
	if id != nil {
		qp, err := s.docs.TransformQuery(fileUID, map[string]any{"populate": q["populate"]})
		if err != nil {
			return err
		}
		row, err = s.docs.E.FindOne(r.Context(), fileUID, query.Params{Where: map[string]any{"id": id}, Populate: qp.Populate})
		if err != nil {
			return err
		}
	}
	if row == nil {
		return newError(http.StatusNotFound, "NotFoundError", "file.notFound")
	}
	row.Set("isUrlSigned", false)
	writeJSON(w, http.StatusOK, s.sanitizeOutput(row, rc.model, rc.auth))
	return nil
}

func (s *Server) uploadDestroy(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
	ctx := r.Context()
	id, err := s.resolveFileID(ctx, rc.params["id"])
	if err != nil {
		return err
	}
	var row query.Row
	if id != nil {
		if row, err = s.docs.E.FindOne(ctx, fileUID, query.Params{Where: map[string]any{"id": id}}); err != nil {
			return err
		}
	}
	if row == nil {
		return newError(http.StatusNotFound, "NotFoundError", "file.notFound")
	}
	if provider, _ := row.Get("provider"); provider == "local" {
		s.removeLocal(row)
	}
	if _, err := s.docs.E.Delete(ctx, fileUID, map[string]any{"id": id}, query.Params{}); err != nil {
		return err
	}
	row.Set("isUrlSigned", false)
	writeJSON(w, http.StatusOK, s.sanitizeOutput(row, s.schemaOf(fileUID), rc.auth))
	return nil
}

func (s *Server) removeLocal(row query.Row) {
	dir := filepath.Join(s.cfg.Upload.PublicDir, "uploads")
	remove := func(hash, ext any) {
		h, _ := hash.(string)
		e, _ := ext.(string)
		if h != "" {
			_ = os.Remove(filepath.Join(dir, filepath.Base(h+e)))
		}
	}
	hash, _ := row.Get("hash")
	ext, _ := row.Get("ext")
	remove(hash, ext)
	formats, _ := row.Get("formats")
	if fm, ok := formats.(map[string]any); ok {
		keys := make([]string, 0, len(fm))
		for k := range fm {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if f, ok := fm[k].(map[string]any); ok {
				remove(f["hash"], f["ext"])
			}
		}
	}
}

// servePublic is Strapi's public middleware: GET and HEAD on a file of
// the public directory.
func (s *Server) servePublic(w http.ResponseWriter, r *http.Request) bool {
	if s.cfg.Upload.PublicDir == "" {
		return false
	}
	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	public := os.DirFS(s.cfg.Upload.PublicDir)
	st, err := fs.Stat(public, name)
	if err != nil || st.IsDir() {
		return false
	}
	http.ServeFileFS(w, r, public, name) //nolint:gosec // os.DirFS rejects names that leave the directory
	return true
}
