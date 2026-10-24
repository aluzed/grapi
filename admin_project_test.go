package grapi_test

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"testing"
)

func logoPNG(w, h int) []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h)))
	return b.Bytes()
}

func TestAdminProjectLogos(t *testing.T) {
	ts, _, stop := newAdminFixture(t, "basic")
	defer stop()
	c := newAdminClient(t, ts.URL)
	_, b, _ := c.do("POST", "/auth/register-admin", `{"firstname":"Ada","email":"ada@example.com","password":"Passw0rd"}`)
	c.token = dataOf(b)["token"].(string)

	if s, b, _ := c.do("GET", "/project-settings", ""); s != 200 || dataOf(b)["menuLogo"] != nil || dataOf(b)["authLogo"] != nil {
		t.Fatalf("no logos yet: %d %v", s, b)
	}
	post := func(fields map[string]string, files map[string][2]any) (int, map[string]any) {
		t.Helper()
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		for k, v := range fields {
			_ = mw.WriteField(k, v)
		}
		for k, f := range files {
			h := textproto.MIMEHeader{}
			h.Set("Content-Disposition", `form-data; name="`+k+`"; filename="logo.html"`)
			h.Set("Content-Type", f[0].(string))
			part, _ := mw.CreatePart(h)
			_, _ = part.Write(f[1].([]byte))
		}
		_ = mw.Close()
		req, _ := http.NewRequest("POST", ts.URL+"/admin/api/v1/project-settings", &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+c.token)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		var out map[string]any
		_ = json.NewDecoder(res.Body).Decode(&out)
		return res.StatusCode, out
	}
	s, b := post(nil, map[string][2]any{"menuLogo": {"image/png", logoPNG(40, 20)}})
	logo, _ := dataOf(b)["menuLogo"].(map[string]any)
	if s != 200 || logo == nil || logo["width"] != float64(40) || logo["ext"] != ".png" || logo["name"] != "logo.png" || dataOf(b)["authLogo"] != nil {
		t.Fatalf("upload: %d %v", s, b)
	}
	url := logo["url"].(string)
	if res, err := http.Get(ts.URL + url); err != nil || res.StatusCode != 200 {
		t.Fatalf("the logo is served: %v %v", res, err)
	}
	if s, b, _ := c.do("GET", "/auth/init", ""); s != 200 || dataOf(b)["menuLogo"] != url || dataOf(b)["hasAdmin"] != true {
		t.Fatalf("init: %d %v", s, b)
	}
	if s, _ := post(nil, map[string][2]any{"authLogo": {"text/plain", []byte("hi")}}); s != 400 {
		t.Fatalf("a text file: %d", s)
	}
	// The extension follows the checked type, not the file name: no HTML or script served by the admin.
	if s, _ := post(nil, map[string][2]any{"authLogo": {"image/svg+xml", []byte("<html><script>alert(1)</script></html>")}}); s != 400 {
		t.Fatalf("not an svg: %d", s)
	}
	if s, _ := post(nil, map[string][2]any{"authLogo": {"image/png", []byte("not a png")}}); s != 400 {
		t.Fatalf("not a png: %d", s)
	}
	if s, _ := post(nil, map[string][2]any{"authLogo": {"image/png", logoPNG(800, 10)}}); s != 400 {
		t.Fatalf("wider than 750: %d", s)
	}
	if s, b := post(map[string]string{"menuLogo": ""}, nil); s != 200 || dataOf(b)["menuLogo"] != nil {
		t.Fatalf("reset: %d %v", s, b)
	}
	if res, _ := http.Get(ts.URL + url); res.StatusCode != 404 {
		t.Fatalf("the previous logo is deleted: %d", res.StatusCode)
	}
}
