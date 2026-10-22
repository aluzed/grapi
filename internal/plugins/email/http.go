package email

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"
)

// The HTTP API providers: @strapi/provider-email-sendgrid, -mailgun and
// -amazon-ses.

func addresses(list string) []*mail.Address {
	if strings.TrimSpace(list) == "" {
		return nil
	}
	parsed, err := mail.ParseAddressList(list)
	if err != nil {
		var out []*mail.Address
		for _, a := range strings.Split(list, ",") {
			if a = strings.TrimSpace(a); a != "" {
				out = append(out, &mail.Address{Address: a})
			}
		}
		return out
	}
	return parsed
}

func do(client *http.Client, req *http.Request, provider string) error {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", provider, err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return fmt.Errorf("%s: %s: %s", provider, res.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

// SendGrid is @strapi/provider-email-sendgrid (providerOptions.apiKey),
// through the v3 mail/send API.
type SendGrid struct {
	APIKey string
	// BaseURL is https://api.sendgrid.com by default.
	BaseURL string
	Client  *http.Client
}

func sgAddress(a *mail.Address) map[string]any {
	m := map[string]any{"email": a.Address}
	if a.Name != "" {
		m["name"] = a.Name
	}
	return m
}

func sgList(list string) []any {
	var out []any
	for _, a := range addresses(list) {
		out = append(out, sgAddress(a))
	}
	return out
}

// Send posts the message.
func (p *SendGrid) Send(ctx context.Context, m Message) error {
	pers := map[string]any{"to": sgList(m.To)}
	if cc := sgList(m.Cc); cc != nil {
		pers["cc"] = cc
	}
	if bcc := sgList(m.Bcc); bcc != nil {
		pers["bcc"] = bcc
	}
	body := map[string]any{"personalizations": []any{pers}, "subject": m.Subject}
	if from := addresses(m.From); len(from) > 0 {
		body["from"] = sgAddress(from[0])
	}
	if rt := addresses(m.ReplyTo); len(rt) > 0 {
		body["reply_to"] = sgAddress(rt[0])
	}
	var content []any
	if m.Text != "" {
		content = append(content, map[string]any{"type": "text/plain", "value": m.Text})
	}
	if m.HTML != "" {
		content = append(content, map[string]any{"type": "text/html", "value": m.HTML})
	}
	body["content"] = content
	if len(m.Headers) > 0 {
		body["headers"] = m.Headers
	}
	b, _ := json.Marshal(body)
	base := p.BaseURL
	if base == "" {
		base = "https://api.sendgrid.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/v3/mail/send", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")
	return do(p.Client, req, "sendgrid")
}

// Mailgun is @strapi/provider-email-mailgun (providerOptions key, username,
// url, domain).
type Mailgun struct {
	Key, Username, URL, Domain string
	Client                     *http.Client
}

// Send posts the message form.
func (p *Mailgun) Send(ctx context.Context, m Message) error {
	form := url.Values{}
	for k, v := range map[string]string{"from": m.From, "to": m.To, "cc": m.Cc, "bcc": m.Bcc, "h:Reply-To": m.ReplyTo, "subject": m.Subject, "text": m.Text, "html": m.HTML} {
		if v != "" {
			form.Set(k, v)
		}
	}
	for k, v := range m.Headers {
		form.Set("h:"+k, v)
	}
	base := p.URL
	if base == "" {
		base = "https://api.mailgun.net"
	}
	user := p.Username
	if user == "" {
		user = "api"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+"/v3/"+url.PathEscape(p.Domain)+"/messages", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(user, p.Key)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return do(p.Client, req, "mailgun")
}

// SES is @strapi/provider-email-amazon-ses (providerOptions key, secret,
// amazon: the endpoint): the SendEmail action of the SES query API, signed
// with SigV4.
type SES struct {
	Key, Secret string
	// Endpoint is https://email.us-east-1.amazonaws.com by default; the
	// region is read from it.
	Endpoint string
	Client   *http.Client
	now      func() time.Time
}

func (p *SES) region(host string) string {
	parts := strings.Split(host, ".")
	if len(parts) >= 3 && parts[0] == "email" {
		return parts[1]
	}
	return "us-east-1"
}

func sum256(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func mac(key []byte, s string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(s))
	return m.Sum(nil)
}

// Send posts the SendEmail action.
func (p *SES) Send(ctx context.Context, m Message) error {
	form := url.Values{"Action": {"SendEmail"}, "Source": {m.From}, "Message.Subject.Data": {m.Subject}}
	for name, list := range map[string]string{"Destination.ToAddresses": m.To, "Destination.CcAddresses": m.Cc, "Destination.BccAddresses": m.Bcc, "ReplyToAddresses": m.ReplyTo} {
		for i, a := range addresses(list) {
			form.Set(fmt.Sprintf("%s.member.%d", name, i+1), a.String())
		}
	}
	if m.HTML != "" {
		form.Set("Message.Body.Html.Data", m.HTML)
	}
	if m.Text != "" {
		form.Set("Message.Body.Text.Data", m.Text)
	}
	endpoint := p.Endpoint
	if endpoint == "" {
		endpoint = "https://email.us-east-1.amazonaws.com"
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	body := []byte(form.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(endpoint, "/")+"/", bytes.NewReader(body))
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if p.now != nil {
		now = p.now()
	}
	stamp, date := now.Format("20060102T150405Z"), now.Format("20060102")
	region := p.region(u.Host)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Amz-Date", stamp)
	signed := "content-type;host;x-amz-date"
	canonical := strings.Join([]string{"POST", "/", "", "content-type:application/x-www-form-urlencoded", "host:" + u.Host, "x-amz-date:" + stamp, "", signed, sum256(body)}, "\n")
	scope := date + "/" + region + "/ses/aws4_request"
	toSign := strings.Join([]string{"AWS4-HMAC-SHA256", stamp, scope, sum256([]byte(canonical))}, "\n")
	k := mac([]byte("AWS4"+p.Secret), date)
	k = mac(k, region)
	k = mac(k, "ses")
	k = mac(k, "aws4_request")
	req.Header.Set("Authorization", fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s", p.Key, scope, signed, hex.EncodeToString(mac(k, toSign))))
	return do(p.Client, req, "amazon-ses")
}
