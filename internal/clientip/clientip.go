// Package clientip reads the client IP the way Koa does for Strapi.
package clientip

import (
	"net"
	"net/http"
	"strings"
)

// Proxy is server.proxy: koa, ipHeader and maxIpsCount.
type Proxy struct {
	// Koa trusts the proxy headers.
	Koa bool
	// IPHeader holds the client chain, X-Forwarded-For when empty.
	IPHeader string
	// MaxIPs keeps only the last MaxIPs addresses of the chain when > 0.
	MaxIPs int
}

// IPs is Koa's request.ips: the proxy chain, empty unless trusted.
func (p Proxy) IPs(r *http.Request) []string {
	if !p.Koa {
		return nil
	}
	header := p.IPHeader
	if header == "" {
		header = "X-Forwarded-For"
	}
	v := r.Header.Get(header)
	if v == "" {
		return nil
	}
	ips := strings.Split(v, ",")
	for i := range ips {
		ips[i] = strings.TrimSpace(ips[i])
	}
	if p.MaxIPs > 0 && len(ips) > p.MaxIPs {
		ips = ips[len(ips)-p.MaxIPs:]
	}
	return ips
}

// IP is Koa's request.ip: the first address of the chain, else the peer.
func (p Proxy) IP(r *http.Request) string {
	if ips := p.IPs(r); len(ips) > 0 && ips[0] != "" {
		return ips[0]
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
