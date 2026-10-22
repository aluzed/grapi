// Package email is the email plugin: providers (smtp like nodemailer,
// sendmail) and users-permissions' strict templates.
package email

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-msgauth/dkim"
)

// Message is what the email service sends.
type Message struct {
	From, To, Cc, Bcc, ReplyTo string
	Subject, Text, HTML        string
	Headers                    map[string]string
}

// Provider sends messages.
type Provider interface {
	Send(ctx context.Context, m Message) error
}

// Settings are the email plugin's defaults.
type Settings struct {
	DefaultFrom, DefaultReplyTo string
}

// Service is strapi.plugin('email').service('email').
type Service struct {
	Provider Provider
	Settings Settings
}

// Send fills the defaults then sends.
func (s *Service) Send(ctx context.Context, m Message) error {
	if s == nil || s.Provider == nil {
		return errors.New("no email provider configured")
	}
	if m.From == "" {
		m.From = s.Settings.DefaultFrom
	}
	if m.ReplyTo == "" {
		m.ReplyTo = s.Settings.DefaultReplyTo
	}
	return s.Provider.Send(ctx, m)
}

// DKIM is nodemailer's dkim option.
type DKIM struct {
	DomainName, KeySelector string
	PrivateKey              string
}

// SMTP is the nodemailer provider over SMTP.
type SMTP struct {
	Host string
	Port int
	// Secure is implicit TLS (port 465); otherwise STARTTLS is used when
	// the server offers it, like nodemailer.
	Secure     bool
	User, Pass string
	DKIM       *DKIM
	// TLSConfig is for tests.
	TLSConfig *tls.Config
	// Hostname is sent in EHLO ("localhost" by default).
	Hostname string
}

// Send delivers m.
func (p *SMTP) Send(ctx context.Context, m Message) error {
	raw, err := Build(m, time.Now())
	if err != nil {
		return err
	}
	if p.DKIM != nil && p.DKIM.PrivateKey != "" {
		if raw, err = p.DKIM.sign(raw); err != nil {
			return err
		}
	}
	port := p.Port
	if port == 0 {
		port = 587
		if p.Secure {
			port = 465
		}
	}
	addr := net.JoinHostPort(p.Host, strconv.Itoa(port))
	d := net.Dialer{Timeout: 30 * time.Second}
	var conn net.Conn
	tlsCfg := p.TLSConfig
	if tlsCfg == nil {
		tlsCfg = &tls.Config{ServerName: p.Host, MinVersion: tls.VersionTLS12}
	}
	if p.Secure {
		conn, err = tls.DialWithDialer(&d, "tcp", addr, tlsCfg)
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return err
	}
	c, err := smtp.NewClient(conn, p.Host)
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer func() { _ = c.Close() }()
	host := p.Hostname
	if host == "" {
		host = "localhost"
	}
	if err := c.Hello(host); err != nil {
		return err
	}
	if ok, _ := c.Extension("STARTTLS"); ok && !p.Secure {
		if err := c.StartTLS(tlsCfg); err != nil {
			return err
		}
	}
	if p.User != "" {
		if err := c.Auth(plainAuth{user: p.User, pass: p.Pass}); err != nil {
			return err
		}
	}
	from, err := mail.ParseAddress(m.From)
	if err != nil {
		return fmt.Errorf("from: %w", err)
	}
	if err := c.Mail(from.Address); err != nil {
		return err
	}
	for _, list := range []string{m.To, m.Cc, m.Bcc} {
		if list == "" {
			continue
		}
		addrs, err := mail.ParseAddressList(list)
		if err != nil {
			return err
		}
		for _, a := range addrs {
			if err := c.Rcpt(a.Address); err != nil {
				return err
			}
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(raw); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// plainAuth is PLAIN without net/smtp's TLS requirement, which nodemailer
// does not have either (servers that need TLS refuse it themselves).
type plainAuth struct{ user, pass string }

func (a plainAuth) Start(_ *smtp.ServerInfo) (string, []byte, error) {
	return "PLAIN", []byte("\x00" + a.user + "\x00" + a.pass), nil
}

func (a plainAuth) Next(_ []byte, more bool) ([]byte, error) {
	if more {
		return nil, errors.New("unexpected server challenge")
	}
	return nil, nil
}

func (k *DKIM) sign(raw []byte) ([]byte, error) {
	block, _ := pem.Decode([]byte(k.PrivateKey))
	if block == nil {
		return nil, errors.New("dkim: the private key is not PEM")
	}
	var signer crypto.Signer
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		signer = key
	} else {
		parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, errors.New("dkim: unreadable private key")
		}
		s, ok := parsed.(crypto.Signer)
		if !ok {
			return nil, errors.New("dkim: unsupported private key")
		}
		signer = s
	}
	var out bytes.Buffer
	err := dkim.Sign(&out, bytes.NewReader(raw), &dkim.SignOptions{
		Domain:                 k.DomainName,
		Selector:               k.KeySelector,
		Signer:                 signer,
		HeaderCanonicalization: dkim.CanonicalizationRelaxed,
		BodyCanonicalization:   dkim.CanonicalizationRelaxed,
	})
	return out.Bytes(), err
}

// Sendmail pipes the message to the local sendmail binary.
type Sendmail struct {
	Path string
}

// Send delivers m with sendmail -t -i.
func (p *Sendmail) Send(ctx context.Context, m Message) error {
	raw, err := Build(m, time.Now())
	if err != nil {
		return err
	}
	path := p.Path
	if path == "" {
		path = "sendmail"
	}
	cmd := exec.CommandContext(ctx, path, "-t", "-i") //nolint:gosec // the configured sendmail binary
	cmd.Stdin = bytes.NewReader(raw)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("sendmail: %w: %s", err, bytes.TrimSpace(out))
	}
	return nil
}

func header(h textproto.MIMEHeader, b *bytes.Buffer) {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, v := range h[k] {
			b.WriteString(k + ": " + v + "\r\n")
		}
	}
}

func encodeQP(s string) []byte {
	var b bytes.Buffer
	w := quotedprintable.NewWriter(&b)
	_, _ = w.Write([]byte(strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")))
	_ = w.Close()
	return b.Bytes()
}

// Build renders m as an RFC 5322 message: text, html or both
// (multipart/alternative), UTF-8, quoted-printable.
func Build(m Message, now time.Time) ([]byte, error) {
	var b bytes.Buffer
	h := textproto.MIMEHeader{}
	h.Set("From", m.From)
	h.Set("To", m.To)
	if m.Cc != "" {
		h.Set("Cc", m.Cc)
	}
	if m.ReplyTo != "" {
		h.Set("Reply-To", m.ReplyTo)
	}
	h.Set("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	h.Set("Date", now.Format(time.RFC1123Z))
	id := make([]byte, 12)
	_, _ = rand.Read(id)
	domain := "localhost"
	if a, err := mail.ParseAddress(m.From); err == nil {
		if at := strings.LastIndexByte(a.Address, '@'); at >= 0 {
			domain = a.Address[at+1:]
		}
	}
	h.Set("Message-ID", "<"+hex.EncodeToString(id)+"@"+domain+">")
	h.Set("MIME-Version", "1.0")
	for k, v := range m.Headers {
		h.Set(k, v)
	}
	part := func(ct, body string) (textproto.MIMEHeader, []byte) {
		ph := textproto.MIMEHeader{}
		ph.Set("Content-Type", ct+"; charset=utf-8")
		ph.Set("Content-Transfer-Encoding", "quoted-printable")
		return ph, encodeQP(body)
	}
	switch {
	case m.Text != "" && m.HTML != "":
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		for _, p := range []struct{ ct, body string }{{"text/plain", m.Text}, {"text/html", m.HTML}} {
			ph, data := part(p.ct, p.body)
			w, err := mw.CreatePart(ph)
			if err != nil {
				return nil, err
			}
			if _, err := w.Write(data); err != nil {
				return nil, err
			}
		}
		if err := mw.Close(); err != nil {
			return nil, err
		}
		h.Set("Content-Type", "multipart/alternative; boundary="+mw.Boundary())
		header(h, &b)
		b.WriteString("\r\n")
		b.Write(body.Bytes())
	default:
		ct, body := "text/plain", m.Text
		if m.HTML != "" {
			ct, body = "text/html", m.HTML
		}
		ph, data := part(ct, body)
		for k, v := range ph {
			h[k] = v
		}
		header(h, &b)
		b.WriteString("\r\n")
		b.Write(data)
	}
	return b.Bytes(), nil
}
