package grapi

import (
	"net"
	"net/http"
	"time"
)

// httpServer serves h with server.http.serverOptions, read as Node's
// http.createServer options (milliseconds): requestTimeout, headersTimeout
// and keepAliveTimeout. The defaults are Node's, except headers, which must
// arrive within 10 seconds instead of 60.
func (s *Server) httpServer(h http.Handler) *http.Server {
	srv := &http.Server{Addr: s.Addr, Handler: h, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 5 * time.Minute, IdleTimeout: 5 * time.Second}
	c := s.project.Conf
	// 0 disables a timeout, as in Node, except headersTimeout.
	ms := func(key string, min int) (time.Duration, bool) {
		n, ok := c.Int("server.http.serverOptions." + key)
		return time.Duration(n) * time.Millisecond, ok && n >= min
	}
	if d, ok := ms("requestTimeout", 0); ok {
		srv.ReadTimeout = d
	}
	if d, ok := ms("headersTimeout", 1); ok {
		srv.ReadHeaderTimeout = d
	}
	if d, ok := ms("keepAliveTimeout", 0); ok {
		srv.IdleTimeout = d
	}
	return srv
}

// listener is server.socket, a Unix socket, when set; else host and port.
func (s *Server) listener() (net.Listener, string, error) {
	if socket, _ := s.project.Conf.String("server.socket"); socket != "" {
		l, err := net.Listen("unix", socket)
		return l, socket, err
	}
	l, err := net.Listen("tcp", s.Addr)
	return l, s.Addr, err
}
