package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// S3 is @strapi/provider-upload-aws-s3 on any S3 compatible store, signed
// with SigV4.
type S3 struct {
	Bucket, Region    string
	AccessKey, Secret string
	SessionToken      string
	Endpoint          string // custom endpoint (MinIO, R2, Scaleway...)
	ForcePathStyle    bool
	BaseURL, RootPath string
	ACL               string // "public-read" by default; "" sends none
	SignedURLExpires  time.Duration
	Client            *http.Client
	now               func() time.Time
}

var unsafeKey = regexp.MustCompile(`[^a-zA-Z0-9_\-./]`)

func sanitizeComponent(s string) string {
	s = strings.ReplaceAll(s, "..", "")
	return strings.Trim(unsafeKey.ReplaceAllString(s, "_"), "/")
}

// Key is getFileKey: rootPath/path/hash+ext.
func (p *S3) Key(f *File) string {
	prefix := ""
	if p.RootPath != "" {
		prefix = strings.TrimRight(p.RootPath, "/") + "/"
	}
	path := ""
	if sp := sanitizeComponent(f.Path); sp != "" {
		path = sp + "/"
	}
	ext := regexp.MustCompile(`[^a-zA-Z0-9.]`).ReplaceAllString(f.Ext, "")
	return prefix + path + sanitizeComponent(f.Hash) + ext
}

func (p *S3) objectURL(key string) *url.URL {
	escaped := (&url.URL{Path: "/" + key}).EscapedPath()
	if p.Endpoint != "" {
		base := p.Endpoint
		if !strings.HasPrefix(base, "http") {
			base = "https://" + base
		}
		u, _ := url.Parse(strings.TrimRight(base, "/"))
		if p.ForcePathStyle {
			u.RawPath = u.Path + "/" + p.Bucket + escaped
			u.Path = u.Path + "/" + p.Bucket + "/" + key
			return u
		}
		u.Host = p.Bucket + "." + u.Host
		u.RawPath, u.Path = escaped, "/"+key
		return u
	}
	host := p.Bucket + ".s3." + p.Region + ".amazonaws.com"
	if p.Region == "" || p.Region == "us-east-1" {
		host = p.Bucket + ".s3.amazonaws.com"
	}
	return &url.URL{Scheme: "https", Host: host, Path: "/" + key, RawPath: escaped}
}

// fileURL follows the provider's constructFileUrl: baseUrl, then the
// object's location.
func (p *S3) fileURL(key string) string {
	if p.BaseURL != "" {
		return strings.TrimRight(p.BaseURL, "/") + "/" + key
	}
	return p.objectURL(key).String()
}

func (p *S3) client() *http.Client {
	if p.Client != nil {
		return p.Client
	}
	return &http.Client{Timeout: 5 * time.Minute}
}

func (p *S3) clock() time.Time {
	if p.now != nil {
		return p.now()
	}
	return time.Now().UTC()
}

func hmacSHA256(key []byte, s string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(s))
	return m.Sum(nil)
}

func (p *S3) signingKey(date string) []byte {
	k := hmacSHA256([]byte("AWS4"+p.Secret), date)
	k = hmacSHA256(k, p.region())
	k = hmacSHA256(k, "s3")
	return hmacSHA256(k, "aws4_request")
}

func (p *S3) region() string {
	if p.Region == "" {
		return "us-east-1"
	}
	return p.Region
}

func canonicalQuery(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		for _, v := range q[k] {
			parts = append(parts, awsEscape(k)+"="+awsEscape(v))
		}
	}
	return strings.Join(parts, "&")
}

func awsEscape(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

// sign adds the SigV4 Authorization header (payload unsigned).
func (p *S3) sign(req *http.Request) {
	now := p.clock()
	amzDate := now.Format("20060102T150405Z")
	date := now.Format("20060102")
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", "UNSIGNED-PAYLOAD")
	if p.SessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", p.SessionToken)
	}
	headers := map[string]string{"host": req.URL.Host}
	for k, v := range req.Header {
		lk := strings.ToLower(k)
		if lk == "content-type" || strings.HasPrefix(lk, "x-amz-") {
			headers[lk] = strings.TrimSpace(strings.Join(v, ","))
		}
	}
	names := make([]string, 0, len(headers))
	for k := range headers {
		names = append(names, k)
	}
	sort.Strings(names)
	var canonHeaders strings.Builder
	for _, k := range names {
		canonHeaders.WriteString(k + ":" + headers[k] + "\n")
	}
	signed := strings.Join(names, ";")
	canonical := strings.Join([]string{req.Method, req.URL.EscapedPath(), canonicalQuery(req.URL.Query()), canonHeaders.String(), signed, "UNSIGNED-PAYLOAD"}, "\n")
	scope := date + "/" + p.region() + "/s3/aws4_request"
	h := sha256.Sum256([]byte(canonical))
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(h[:])
	sig := hex.EncodeToString(hmacSHA256(p.signingKey(date), toSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+p.AccessKey+"/"+scope+", SignedHeaders="+signed+", Signature="+sig)
}

func (p *S3) do(req *http.Request) error {
	p.sign(req)
	res, err := p.client().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return fmt.Errorf("s3 %s %s: %d %s", req.Method, req.URL.Path, res.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

// Upload is a PutObject.
func (p *S3) Upload(ctx context.Context, f *File, r io.Reader) error {
	key := p.Key(f)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, p.objectURL(key).String(), r)
	if err != nil {
		return err
	}
	if f.Size > 0 {
		req.ContentLength = f.Size
	}
	if f.Mime != "" {
		req.Header.Set("Content-Type", f.Mime)
	}
	if p.ACL != "" {
		req.Header.Set("X-Amz-Acl", p.ACL)
	}
	if err := p.do(req); err != nil {
		return err
	}
	f.URL = p.fileURL(key)
	return nil
}

// Delete is a DeleteObject.
func (p *S3) Delete(ctx context.Context, f *File) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, p.objectURL(p.Key(f)).String(), nil)
	if err != nil {
		return err
	}
	return p.do(req)
}

// IsPrivate is ACL private.
func (p *S3) IsPrivate() bool { return p.ACL == "private" }

// SignedURL presigns a GetObject (15 minutes by default).
func (p *S3) SignedURL(_ context.Context, f *File) (string, error) {
	expires := p.SignedURLExpires
	if expires == 0 {
		expires = 15 * time.Minute
	}
	now := p.clock()
	amzDate := now.Format("20060102T150405Z")
	date := now.Format("20060102")
	u := p.objectURL(p.Key(f))
	scope := date + "/" + p.region() + "/s3/aws4_request"
	q := url.Values{}
	q.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	q.Set("X-Amz-Credential", p.AccessKey+"/"+scope)
	q.Set("X-Amz-Date", amzDate)
	q.Set("X-Amz-Expires", strconv.Itoa(int(expires.Seconds())))
	q.Set("X-Amz-SignedHeaders", "host")
	if p.SessionToken != "" {
		q.Set("X-Amz-Security-Token", p.SessionToken)
	}
	canonical := strings.Join([]string{http.MethodGet, u.EscapedPath(), canonicalQuery(q), "host:" + u.Host + "\n", "host", "UNSIGNED-PAYLOAD"}, "\n")
	h := sha256.Sum256([]byte(canonical))
	toSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + hex.EncodeToString(h[:])
	q.Set("X-Amz-Signature", hex.EncodeToString(hmacSHA256(p.signingKey(date), toSign)))
	u.RawQuery = canonicalQuery(q)
	return u.String(), nil
}
