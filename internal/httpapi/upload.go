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
	"github.com/aluzed/grapi/internal/db/store"
	"github.com/aluzed/grapi/internal/media"
	"github.com/aluzed/grapi/internal/media/provider"
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
	// Provider stores the files (the local provider when nil) under
	// ProviderName, the value of files.provider.
	Provider     provider.Provider
	ProviderName string
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
		route{"POST", []string{base, "upload"}, uid, "upload", s.upload, "", false, false, nil},
		route{"GET", []string{base, "upload", "files"}, uid, "find", s.uploadFind, "", false, false, nil},
		route{"GET", []string{base, "upload", "files", "page"}, uid, "findPage", s.uploadFindPage, "", false, false, nil},
		route{"GET", []string{base, "upload", "files", ":id"}, uid, "findOne", s.uploadFindOne, "", false, false, nil},
		route{"DELETE", []string{base, "upload", "files", ":id"}, uid, "destroy", s.uploadDestroy, "", false, false, nil},
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
	id := r.URL.Query().Get("id")
	if len(req.files) == 0 || len(req.files) == 1 && req.files[0].size == 0 {
		if id != "" {
			return s.updateFileInfo(w, r, rc, req, id)
		}
		return validationError("Files are empty", nil)
	}
	if id != "" {
		return s.replaceFile(w, r, rc, req, dir, id)
	}
	return s.uploadFiles(w, r, rc, req, dir)
}

// fileByParam is resolveFileId then findOne.
func (s *Server) fileByParam(ctx context.Context, id string) (query.Row, error) {
	fid, err := s.resolveFileID(ctx, id)
	if err != nil || fid == nil {
		return nil, err
	}
	return s.docs.E.FindOne(ctx, fileUID, query.Params{Where: map[string]any{"id": fid}, Populate: []any{"folder"}})
}

func (s *Server) emitMedia(event string, row query.Row) {
	if s.docs.Events == nil || row == nil {
		return
	}
	media := plain(s.sanitizeOutput(row, s.schemaOf(fileUID), nil))
	s.docs.Events.Emit(event, map[string]any{"media": media})
}

// fileInfoFromBody is validateUploadBody for an update: the fileInfo
// field, JSON or multipart.
func fileInfoFromBody(r *http.Request, req *multipartRequest) (map[string]any, bool, error) {
	if raw := req.fields["fileInfo"]; len(raw) > 0 {
		infos, err := fileInfos(raw[:1], 1)
		if err != nil {
			return nil, false, err
		}
		return infos[0], true, nil
	}
	if r.Header.Get("Content-Type") == "" || strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		body, _, err := readBody(r)
		if err != nil {
			return nil, false, err
		}
		if fi, ok := body["fileInfo"]; ok {
			b, _ := json.Marshal(fi)
			infos, err := fileInfos([]string{string(b)}, 1)
			if err != nil {
				return nil, false, err
			}
			return infos[0], true, nil
		}
	}
	return map[string]any{"focalPoint": nil}, false, nil
}

func (s *Server) updateFileInfo(w http.ResponseWriter, r *http.Request, rc *requestContext, req *multipartRequest, id string) error {
	ctx := r.Context()
	info, _, err := fileInfoFromBody(r, req)
	if err != nil {
		return err
	}
	row, err := s.applyFileInfo(ctx, id, info)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, s.sanitizeOutput(row, s.schemaOf(fileUID), rc.auth))
	return nil
}

// applyFileInfo is the upload service's updateFileInfo: the infos left
// out keep their value.
func (s *Server) applyFileInfo(ctx context.Context, id string, info map[string]any) (query.Row, error) {
	file, err := s.fileByParam(ctx, id)
	if err != nil {
		return nil, err
	}
	if file == nil {
		return nil, errNotFound
	}
	keep := func(k string) any {
		if v := info[k]; v != nil {
			return v
		}
		v, _ := file.Get(k)
		return v
	}
	data := query.Data{"name": keep("name"), "alternativeText": keep("alternativeText"), "caption": keep("caption"), "focalPoint": keep("focalPoint")}
	fid, _ := file.Get("id")
	row, err := s.docs.E.Update(ctx, fileUID, map[string]any{"id": fid}, data, query.Params{})
	if err != nil {
		return nil, err
	}
	s.emitMedia("media.update", row)
	s.signFile(ctx, row)
	return row, nil
}

func (s *Server) replaceFile(w http.ResponseWriter, r *http.Request, rc *requestContext, req *multipartRequest, dir, id string) error {
	ctx := r.Context()
	if len(req.files) > 1 {
		return validationError("Cannot replace a file with multiple ones", nil)
	}
	f := req.files[0]
	f.detected = detectMime(f.path)
	infos, err := fileInfos(req.fields["fileInfo"], 1)
	if err != nil {
		return err
	}
	file, err := s.fileByParam(ctx, id)
	if err != nil {
		return err
	}
	if file == nil {
		return errNotFound
	}
	var settings uploadSettings
	if err := s.uploadSettings(ctx, &settings); err != nil {
		return err
	}
	folderPath, _ := file.Get("folderPath")
	row, err := s.uploadOne(ctx, f, infos[0], folderPath, nil, settings, dir, file)
	if err != nil {
		return err
	}
	s.signFile(ctx, row)
	writeJSON(w, http.StatusOK, s.sanitizeOutput(row, s.schemaOf(fileUID), rc.auth))
	return nil
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
		row, err := s.uploadOne(ctx, f, info, folderPath, metas, settings, dir, nil)
		if err != nil {
			return err
		}
		s.signFile(ctx, row)
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
	_, err := store.Store{DB: s.auth.db}.Get(ctx, uploadScope, "settings", v)
	return err
}

var uploadScope = store.Scope{Type: "plugin", Name: "upload"}

// apiUploadFolder is the "API Uploads" folder, created on first use and
// remembered in the core store.
func (s *Server) apiUploadFolder(ctx context.Context) (query.Row, error) {
	var stored struct {
		ID any `json:"id"`
	}
	st := store.Store{DB: s.auth.db}
	if _, err := st.Get(ctx, uploadScope, "api-folder", &stored); err != nil {
		return nil, err
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
	return folder, st.Set(ctx, uploadScope, "api-folder", map[string]any{"id": id})
}

// uploadOne stores f and its formats then creates the file row, or, with
// existing, replaces that file's bytes keeping its hash and extension.
func (s *Server) uploadOne(ctx context.Context, f *inputFile, info map[string]any, folderPath any, metas map[string]string, settings uploadSettings, dir string, existing query.Row) (query.Row, error) {
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
	if existing != nil {
		h, _ := existing.Get("hash")
		e, _ := existing.Get("ext")
		hash, _ = h.(string)
		ext, _ = e.(string)
	}
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
	url, err := s.store(ctx, src, hash, ext, mimeType, metas["path"])
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
			u, err := s.store(ctx, dst, fh, ext, mimeType, metas["path"])
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
	_, providerName := s.provider()
	file["provider"] = providerName
	if existing == nil {
		row, err := s.docs.E.Create(ctx, fileUID, file, query.Params{})
		if err == nil {
			s.emitMedia("media.create", row)
		}
		return row, err
	}
	if _, ok := file["formats"]; !ok {
		file["formats"] = map[string]any{}
	}
	if old, ok := existing.Get("formats"); ok {
		newFormats, _ := file["formats"].(map[string]any)
		if om, ok := old.(map[string]any); ok {
			for k, f := range om {
				if _, kept := newFormats[k]; kept {
					continue
				}
				if fm, ok := f.(map[string]any); ok {
					s.removeStored(ctx, fm["hash"], fm["ext"], fm["path"])
				}
			}
		}
	}
	delete(file, "folder")
	delete(file, "folderPath")
	delete(file, "related")
	// Absent fileInfo keys are undefined in Strapi, which knex leaves out.
	for _, k := range []string{"alternativeText", "caption"} {
		if _, given := info[k]; !given {
			delete(file, k)
		}
	}
	id, _ := existing.Get("id")
	row, err := s.docs.E.Update(ctx, fileUID, map[string]any{"id": id}, file, query.Params{})
	if err == nil {
		s.emitMedia("media.update", row)
	}
	return row, err
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

func (s *Server) provider() (provider.Provider, string) {
	if s.cfg.Upload.Provider != nil {
		return s.cfg.Upload.Provider, s.cfg.Upload.ProviderName
	}
	return &provider.Local{PublicDir: s.cfg.Upload.PublicDir}, "local"
}

// store hands the file to the provider and returns its URL.
func (s *Server) store(ctx context.Context, src, hash, ext, mimeType, path string) (string, error) {
	in, err := os.Open(src) //nolint:gosec // a temporary upload file
	if err != nil {
		return "", err
	}
	defer func() { _ = in.Close() }()
	st, err := in.Stat()
	if err != nil {
		return "", err
	}
	p, _ := s.provider()
	f := &provider.File{Hash: hash, Ext: ext, Mime: mimeType, Path: path, Size: st.Size()}
	if err := p.Upload(ctx, f, in); err != nil {
		return "", err
	}
	return f.URL, nil
}

// signFile is signFileUrls: private providers get signed URLs, for the
// file and its formats.
func (s *Server) signFile(ctx context.Context, row query.Row) {
	row.Set("isUrlSigned", false)
	p, name := s.provider()
	if prov, _ := row.Get("provider"); prov != name || !p.IsPrivate() {
		return
	}
	sign := func(m interface {
		Get(string) (any, bool)
	}, set func(string, any),
	) {
		hash, _ := m.Get("hash")
		ext, _ := m.Get("ext")
		path, _ := m.Get("path")
		h, _ := hash.(string)
		e, _ := ext.(string)
		pa, _ := path.(string)
		if u, err := p.SignedURL(ctx, &provider.File{Hash: h, Ext: e, Path: pa}); err == nil {
			set("url", u)
		}
	}
	sign(row, func(k string, v any) { row.Set(k, v) })
	if formats, ok := row.Get("formats"); ok {
		if fm, ok := formats.(map[string]any); ok {
			for _, f := range fm {
				if m, ok := f.(map[string]any); ok {
					sign(mapGetter(m), func(k string, v any) { m[k] = v })
				}
			}
		}
	}
	row.Set("isUrlSigned", true)
}

type mapGetter map[string]any

func (m mapGetter) Get(k string) (any, bool) {
	v, ok := m[k]
	return v, ok
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
		s.signFile(r.Context(), row)
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

// uploadFindPage is findAndCountPage: the REST pagination (api.rest
// defaultLimit, maxLimit, withCount) on the files.
func (s *Server) uploadFindPage(w http.ResponseWriter, r *http.Request, rc *requestContext) error {
	ctx := r.Context()
	rc.model = s.schemaOf(fileUID)
	q, err := s.sanitizedQuery(rc)
	if err != nil {
		return err
	}
	pag, _ := q["pagination"].(map[string]any)
	info, err := s.paginationInfo(pag)
	if err != nil {
		return err
	}
	_, hasStart := pag["start"]
	_, hasLimit := pag["limit"]
	uq := userQuery(q)
	delete(uq, "pagination")
	uq["start"], uq["limit"] = info.start, info.limit
	qp, err := s.docs.TransformQuery(fileUID, uq)
	if err != nil {
		return err
	}
	rows, err := s.docs.E.FindMany(ctx, fileUID, qp)
	if err != nil {
		return err
	}
	out := make([]any, 0, len(rows))
	for _, row := range rows {
		s.signFile(ctx, row)
		out = append(out, s.sanitizeOutput(row, rc.model, rc.auth))
	}
	count, err := s.shouldCount(pag)
	if err != nil {
		return err
	}
	var total *int64
	if count {
		n, err := s.docs.E.Count(ctx, fileUID, query.Params{Where: qp.Where, Filters: qp.Filters})
		if err != nil {
			return err
		}
		total = &n
	}
	meta := map[string]any{}
	if hasStart || hasLimit {
		meta["start"], meta["limit"] = info.start, info.limit
		if total != nil {
			meta["total"] = *total
		}
	} else {
		meta["page"], meta["pageSize"] = math.Floor(info.start/info.limit)+1, info.limit
		if total != nil {
			pc := float64(0)
			if info.limit > 0 {
				pc = math.Ceil(float64(*total) / info.limit)
			}
			meta["pageCount"], meta["total"] = pc, *total
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out, "meta": map[string]any{"pagination": meta}})
	return nil
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
	s.signFile(r.Context(), row)
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
	s.removeFromProvider(ctx, row)
	if _, err := s.docs.E.Delete(ctx, fileUID, map[string]any{"id": id}, query.Params{}); err != nil {
		return err
	}
	s.emitMedia("media.delete", row)
	s.signFile(ctx, row)
	writeJSON(w, http.StatusOK, s.sanitizeOutput(row, s.schemaOf(fileUID), rc.auth))
	return nil
}

func (s *Server) removeStored(ctx context.Context, hash, ext, path any) {
	h, _ := hash.(string)
	e, _ := ext.(string)
	pa, _ := path.(string)
	if h == "" {
		return
	}
	p, _ := s.provider()
	if err := p.Delete(ctx, &provider.File{Hash: h, Ext: e, Path: pa}); err != nil {
		s.log.Error("upload: could not delete a file from the provider", "hash", h, "error", err)
	}
}

// removeFromProvider deletes a file and its formats when they belong to
// the configured provider.
func (s *Server) removeFromProvider(ctx context.Context, row query.Row) {
	_, name := s.provider()
	if prov, _ := row.Get("provider"); prov != name {
		return
	}
	hash, _ := row.Get("hash")
	ext, _ := row.Get("ext")
	s.removeStored(ctx, hash, ext, nil)
	formats, _ := row.Get("formats")
	if fm, ok := formats.(map[string]any); ok {
		keys := make([]string, 0, len(fm))
		for k := range fm {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if f, ok := fm[k].(map[string]any); ok {
				s.removeStored(ctx, f["hash"], f["ext"], f["path"])
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
