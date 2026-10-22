package provider

import (
	"bytes"
	"context"
	"crypto/sha1" //nolint:gosec // Cloudinary's API signature is SHA-1
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Cloudinary is @strapi/provider-upload-cloudinary: providerOptions
// cloud_name, api_key, api_secret, and actionOptions.upload / delete.
type Cloudinary struct {
	CloudName, APIKey, APISecret string
	UploadOptions, DeleteOptions map[string]any
	// BaseURL is https://api.cloudinary.com by default.
	BaseURL string
	Client  *http.Client
	now     func() time.Time
}

func (c *Cloudinary) endpoint(resource, action string) string {
	base := c.BaseURL
	if base == "" {
		base = "https://api.cloudinary.com"
	}
	return fmt.Sprintf("%s/v1_1/%s/%s/%s", strings.TrimRight(base, "/"), url.PathEscape(c.CloudName), resource, action)
}

func paramString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = paramString(e)
		}
		return strings.Join(parts, ",")
	}
	return fmt.Sprint(v)
}

// signed adds timestamp, api_key and signature: the SHA-1 of the sorted
// parameters (file, api_key and resource_type left out) and the secret.
func (c *Cloudinary) signed(params map[string]string) map[string]string {
	now := time.Now()
	if c.now != nil {
		now = c.now()
	}
	params["timestamp"] = strconv.FormatInt(now.Unix(), 10)
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if k != "file" && k != "api_key" && k != "resource_type" && k != "cloud_name" && v != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + params[k]
	}
	sum := sha1.Sum([]byte(strings.Join(parts, "&") + c.APISecret)) //nolint:gosec // Cloudinary's scheme
	params["signature"] = hex.EncodeToString(sum[:])
	params["api_key"] = c.APIKey
	return params
}

func (c *Cloudinary) client() *http.Client {
	if c.Client != nil {
		return c.Client
	}
	return &http.Client{Timeout: 5 * time.Minute}
}

type cloudinaryResult struct {
	SecureURL    string `json:"secure_url"`
	PublicID     string `json:"public_id"`
	ResourceType string `json:"resource_type"`
	Result       string `json:"result"`
	Error        *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Cloudinary) call(req *http.Request) (*cloudinaryResult, error) {
	res, err := c.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("cloudinary: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	var out cloudinaryResult
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	_ = json.Unmarshal(body, &out)
	if res.StatusCode >= 300 || out.Error != nil {
		msg := strings.TrimSpace(string(body))
		if out.Error != nil {
			msg = out.Error.Message
		}
		return nil, fmt.Errorf("cloudinary: %s: %s", res.Status, msg)
	}
	return &out, nil
}

// Upload sends the file with resource_type auto and the hash as public_id.
func (c *Cloudinary) Upload(ctx context.Context, f *File, r io.Reader) error {
	params := map[string]string{"public_id": f.Hash}
	for k, v := range c.UploadOptions {
		params[k] = paramString(v)
	}
	params = c.signed(params)
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		_ = w.WriteField(k, params[k])
	}
	part, err := w.CreateFormFile("file", f.Hash+f.Ext)
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, r); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("auto", "upload"), &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	out, err := c.call(req)
	if err != nil {
		return err
	}
	f.URL = out.SecureURL
	f.Metadata = map[string]any{"public_id": out.PublicID, "resource_type": out.ResourceType}
	return nil
}

// Delete destroys the file of its provider_metadata; an absent file is not
// an error.
func (c *Cloudinary) Delete(ctx context.Context, f *File) error {
	publicID, resource := f.Hash, "image"
	if f.Metadata != nil {
		if id, ok := f.Metadata["public_id"].(string); ok && id != "" {
			publicID = id
		}
		if rt, ok := f.Metadata["resource_type"].(string); ok && rt != "" {
			resource = rt
		}
	}
	params := map[string]string{"public_id": publicID, "invalidate": "true"}
	for k, v := range c.DeleteOptions {
		params[k] = paramString(v)
	}
	params = c.signed(params)
	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(resource, "destroy"), strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	out, err := c.call(req)
	if err != nil {
		return err
	}
	if out.Result != "ok" && out.Result != "not found" {
		return fmt.Errorf("cloudinary: error deleting on cloudinary: %s", out.Result)
	}
	return nil
}

// IsPrivate is false: Cloudinary URLs are public.
func (c *Cloudinary) IsPrivate() bool { return false }

// SignedURL returns the plain URL.
func (c *Cloudinary) SignedURL(_ context.Context, f *File) (string, error) { return f.URL, nil }
