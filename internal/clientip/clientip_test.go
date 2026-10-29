package clientip

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIP(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.0.2.1:4000"
	r.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.2 ,10.0.0.3")
	r.Header.Set("X-Real-Ip", "198.51.100.9")
	for _, c := range []struct {
		p    Proxy
		want string
	}{
		{Proxy{}, "192.0.2.1"},
		{Proxy{Koa: true}, "203.0.113.7"},
		{Proxy{Koa: true, MaxIPs: 2}, "10.0.0.2"},
		{Proxy{Koa: true, MaxIPs: 9}, "203.0.113.7"},
		{Proxy{Koa: true, IPHeader: "X-Real-Ip"}, "198.51.100.9"},
		{Proxy{Koa: true, IPHeader: "X-Missing"}, "192.0.2.1"},
	} {
		if got := c.p.IP(r); got != c.want {
			t.Errorf("%+v: %s, want %s", c.p, got, c.want)
		}
	}
}

// Behind one proxy that appends the peer, with maxIpsCount 1, what the
// client writes in the header never becomes its identity.
func TestIPForgedBehindProxy(t *testing.T) {
	p := Proxy{Koa: true, MaxIPs: 1}
	for _, forged := range [][]string{nil, {"127.0.0.1"}, {"1.2.3.4, 5.6.7.8"}} {
		for _, client := range []string{"203.0.113.7", "198.51.100.9"} {
			for _, sameLine := range []bool{true, false} {
				r := httptest.NewRequest("GET", "/", nil)
				r.RemoteAddr = "10.0.0.2:4000"
				h := http.Header{}
				for _, f := range forged {
					h.Add("X-Forwarded-For", f)
				}
				if v := h.Get("X-Forwarded-For"); sameLine && v != "" {
					h.Set("X-Forwarded-For", v+", "+client)
				} else {
					h.Add("X-Forwarded-For", client)
				}
				r.Header = h
				if got := p.IP(r); got != client {
					t.Errorf("forged %v, same line %v: %s, want %s", forged, sameLine, got, client)
				}
			}
		}
	}
}
