package email

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-msgauth/dkim"
)

func TestTemplate(t *testing.T) {
	data := map[string]any{
		"URL":   "https://ex.com/reset",
		"USER":  map[string]any{"email": "a@ex.com", "username": "alex", "blocked": false, "role": map[string]any{}},
		"TOKEN": "abc",
	}
	cases := map[string]string{
		"Hi <%= USER.username %>, <%=URL%>?code=<%= TOKEN %>": "Hi alex, https://ex.com/reset?code=abc",
		"<% process.exit() %> <%- USER.email %>":              "<% process.exit() %> <%- USER.email %>",
		"<%= USER %> <%= nope %> ${TOKEN}":                    "<%= USER %> <%= nope %> ${TOKEN}",
		"<%= USER.blocked %>":                                 "false",
		"<%= USER.role %>":                                    "<%= USER.role %>",
	}
	for in, want := range cases {
		if got := Template(in, data); got != want {
			t.Errorf("Template(%q) = %q, want %q", in, got, want)
		}
	}
}

// fakeSMTP accepts one message and returns its DATA.
func fakeSMTP(t *testing.T) (string, <-chan []byte) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan []byte, 1)
	go func() {
		defer func() { _ = ln.Close() }()
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		r := bufio.NewReader(c)
		say := func(s string) { _, _ = c.Write([]byte(s + "\r\n")) }
		say("220 fake")
		var data bytes.Buffer
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(cmd, "EHLO"):
				say("250-fake")
				say("250 AUTH PLAIN")
			case strings.HasPrefix(cmd, "AUTH"), strings.HasPrefix(cmd, "MAIL"), strings.HasPrefix(cmd, "RCPT"):
				if strings.HasPrefix(cmd, "AUTH") {
					say("235 ok")
				} else {
					say("250 ok")
				}
			case cmd == "DATA":
				say("354 go")
				for {
					l, err := r.ReadString('\n')
					if err != nil {
						return
					}
					if l == ".\r\n" {
						break
					}
					data.WriteString(strings.TrimPrefix(l, "."))
				}
				say("250 queued")
				got <- data.Bytes()
			case cmd == "QUIT":
				say("221 bye")
				return
			default:
				say("250 ok")
			}
		}
	}()
	return ln.Addr().String(), got
}

func TestSMTPSendsSignedMail(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	addr, got := fakeSMTP(t)
	host, port, _ := net.SplitHostPort(addr)
	n, _ := strconv.Atoi(port)
	p := &SMTP{Host: host, Port: n, User: "u", Pass: "p", DKIM: &DKIM{DomainName: "dev-freelance.com", KeySelector: "sel2026", PrivateKey: pemKey}}
	s := &Service{Provider: p, Settings: Settings{DefaultFrom: "Dev Freelance <contact@dev-freelance.com>"}}
	if err := s.Send(context.Background(), Message{To: "a@ex.com", Subject: "Réinitialisation", Text: "Bonjour", HTML: "<p>Bonjour</p>"}); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	select {
	case raw = <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("no message")
	}
	pub, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	verifications, err := dkim.VerifyWithOptions(bytes.NewReader(raw), &dkim.VerifyOptions{
		LookupTXT: func(domain string) ([]string, error) {
			if domain != "sel2026._domainkey.dev-freelance.com" {
				t.Errorf("lookup %s", domain)
			}
			return []string{"v=DKIM1; k=rsa; p=" + base64.StdEncoding.EncodeToString(pub)}, nil
		},
	})
	if err != nil || len(verifications) != 1 || verifications[0].Err != nil {
		t.Fatalf("dkim: %v %+v", err, verifications)
	}
	if v := verifications[0]; v.Domain != "dev-freelance.com" {
		t.Fatalf("d=%s", v.Domain)
	}
	if !bytes.Contains(raw, []byte("multipart/alternative")) || !bytes.Contains(raw, []byte("=?utf-8?q?R=C3=A9initialisation?=")) {
		t.Fatalf("message:\n%s", raw)
	}
}
