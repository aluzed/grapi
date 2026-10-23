package adminapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/aluzed/grapi/internal/auth/permissions"
	"github.com/aluzed/grapi/internal/db/store"
)

var uploadScope = store.Scope{Type: "plugin", Name: "upload"}

const (
	uploadSettingsRead = "plugin::upload.settings.read"
	uploadConfigure    = "plugin::upload.configure-view-settings"
)

func (s *Server) mediaSettingsRoutes() {
	s.allowed("GET /upload/settings", uploadSettingsRead, s.getUploadSettings)
	s.allowed("PUT /upload/settings", uploadSettingsRead, s.putUploadSettings)
	s.allowed("GET /upload/configuration", uploadRead, s.getUploadConfiguration)
	s.allowed("PUT /upload/configuration", uploadConfigure, s.putUploadConfiguration)
	s.allowed("POST /upload/actions/upload-from-urls", uploadCreate, s.uploadFromURLs)
}

// UploadSettings is plugin_upload_settings, read by every upload.
type UploadSettings struct {
	SizeOptimization     bool `json:"sizeOptimization"`
	ResponsiveDimensions bool `json:"responsiveDimensions"`
	AutoOrientation      bool `json:"autoOrientation"`
}

func (s *Server) getUploadSettings(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	v := UploadSettings{SizeOptimization: true, ResponsiveDimensions: true}
	if _, err := s.store.Get(r.Context(), uploadScope, "settings", &v); err != nil {
		return err
	}
	return data(w, v)
}

func (s *Server) putUploadSettings(w http.ResponseWriter, r *http.Request, u *User, _ *permissions.Ability) error {
	var v UploadSettings
	if err := decode(r, &v); err != nil {
		return err
	}
	if err := s.store.Set(r.Context(), uploadScope, "settings", v); err != nil {
		return err
	}
	s.audit(r, u, "media-settings.update", map[string]any{"sizeOptimization": v.SizeOptimization, "responsiveDimensions": v.ResponsiveDimensions, "autoOrientation": v.AutoOrientation})
	return data(w, v)
}

// UploadConfiguration is plugin_upload_view_configuration.
type UploadConfiguration struct {
	PageSize int    `json:"pageSize"`
	Sort     string `json:"sort"`
}

var uploadSorts = map[string]bool{
	"createdAt:DESC": true, "createdAt:ASC": true, "name:ASC": true, "name:DESC": true, "updatedAt:DESC": true, "updatedAt:ASC": true,
}

func (s *Server) getUploadConfiguration(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	v := UploadConfiguration{PageSize: 10, Sort: "createdAt:DESC"}
	if _, err := s.store.Get(r.Context(), uploadScope, "view_configuration", &v); err != nil {
		return err
	}
	return data(w, v)
}

func (s *Server) putUploadConfiguration(w http.ResponseWriter, r *http.Request, _ *User, _ *permissions.Ability) error {
	var v UploadConfiguration
	if err := decode(r, &v); err != nil {
		return err
	}
	var bad []FieldError
	if v.PageSize != 10 && v.PageSize != 20 && v.PageSize != 50 && v.PageSize != 100 {
		bad = append(bad, FieldError{Path: []string{"pageSize"}, Message: "pageSize must be one of 10, 20, 50, 100"})
	}
	if !uploadSorts[v.Sort] {
		bad = append(bad, FieldError{Path: []string{"sort"}, Message: "unknown sort"})
	}
	if bad != nil {
		return validation(bad)
	}
	if err := s.store.Set(r.Context(), uploadScope, "view_configuration", v); err != nil {
		return err
	}
	return data(w, v)
}

var errPrivateAddress = errors.New("this address is not reachable from the server")

// publicOnly refuses, at connect time, the loopback, private, link-local,
// CGNAT and unspecified addresses: redirects and DNS rebinding are checked
// too.
func publicOnly(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || localIP(ip) {
		return errPrivateAddress
	}
	return nil
}

var cgnat = func() *net.IPNet { _, n, _ := net.ParseCIDR("100.64.0.0/10"); return n }()

// localIP reports an address that is not reachable over the public
// internet: loopback, private, link-local, CGNAT, unspecified or multicast.
func localIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() || cgnat.Contains(ip)
}

func (s *Server) urlClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	if !s.cfg.UploadFromURLPrivate {
		dialer.Control = publicOnly
	}
	return &http.Client{
		Timeout:   s.cfg.UploadFromURLTimeout,
		Transport: &http.Transport{Proxy: nil, DialContext: dialer.DialContext, TLSHandshakeTimeout: 10 * time.Second},
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("too many redirects")
			}
			return nil
		},
	}
}

type downloaded struct {
	file *os.File
	name string
	mime string
}

// download fetches one URL into a temporary file, within the size limit.
func (s *Server) download(ctx context.Context, raw string) (*downloaded, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("%s: only http and https URLs", raw)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	res, err := s.urlClient().Do(req)
	if err != nil {
		if errors.Is(err, errPrivateAddress) {
			return nil, fmt.Errorf("%s: %w", raw, errPrivateAddress)
		}
		return nil, fmt.Errorf("%s: the download failed", raw)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", raw, res.Status)
	}
	f, err := os.CreateTemp("", "grapi-url-*")
	if err != nil {
		return nil, err
	}
	n, err := io.Copy(f, io.LimitReader(res.Body, s.cfg.UploadFromURLMaxSize+1))
	if err == nil && n > s.cfg.UploadFromURLMaxSize {
		err = fmt.Errorf("%s: larger than %d bytes", raw, s.cfg.UploadFromURLMaxSize)
	}
	if err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return nil, err
	}
	head := make([]byte, 512)
	k, _ := f.ReadAt(head, 0)
	sniffed := http.DetectContentType(head[:k])
	if strings.HasPrefix(sniffed, "text/html") {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return nil, fmt.Errorf("%s: a web page, not a file", raw)
	}
	name := path.Base(u.Path)
	if _, params, err := mime.ParseMediaType(res.Header.Get("Content-Disposition")); err == nil && params["filename"] != "" {
		name = path.Base(params["filename"])
	}
	if name == "" || name == "/" || name == "." {
		name = "file"
	}
	if path.Ext(name) == "" {
		if exts, _ := mime.ExtensionsByType(sniffed); len(exts) > 0 {
			name += exts[0]
		}
	}
	return &downloaded{file: f, name: name, mime: sniffed}, nil
}

// uploadFromURLs downloads each URL then hands the files to the regular
// upload, as one multipart request.
func (s *Server) uploadFromURLs(w http.ResponseWriter, r *http.Request, u *User, a *permissions.Ability) error {
	var b struct {
		URLs   []string `json:"urls"`
		Folder *int64   `json:"folder"`
	}
	if err := decode(r, &b); err != nil {
		return err
	}
	if len(b.URLs) == 0 || len(b.URLs) > 20 {
		return validation([]FieldError{{Path: []string{"urls"}, Message: "between 1 and 20 URLs"}})
	}
	var files []*downloaded
	defer func() {
		for _, f := range files {
			_ = f.file.Close()
			_ = os.Remove(f.file.Name())
		}
	}()
	var bad []FieldError
	for i, raw := range b.URLs {
		f, err := s.download(r.Context(), raw)
		if err != nil {
			bad = append(bad, FieldError{Path: []string{"urls", strconv.Itoa(i)}, Message: err.Error()})
			continue
		}
		files = append(files, f)
	}
	if bad != nil {
		return validation(bad)
	}
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		var err error
		for _, f := range files {
			var part io.Writer
			h := make(map[string][]string)
			h["Content-Disposition"] = []string{fmt.Sprintf(`form-data; name="files"; filename=%q`, f.name)}
			h["Content-Type"] = []string{f.mime}
			if part, err = mw.CreatePart(h); err != nil {
				break
			}
			if _, err = f.file.Seek(0, io.SeekStart); err != nil {
				break
			}
			if _, err = io.Copy(part, f.file); err != nil {
				break
			}
		}
		if err == nil {
			err = mw.Close()
		}
		_ = pw.CloseWithError(err)
	}()
	q := url.Values{}
	if b.Folder != nil {
		q.Set("folder", strconv.FormatInt(*b.Folder, 10))
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "/upload?"+q.Encode(), pr)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.RemoteAddr, req.Header["User-Agent"] = r.RemoteAddr, r.Header["User-Agent"]
	return s.uploadFiles(w, req, u, a)
}
