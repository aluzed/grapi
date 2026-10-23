package clientip

import (
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
