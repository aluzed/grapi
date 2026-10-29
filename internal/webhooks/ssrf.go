package webhooks

import (
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

// publicOnlyTransport checks the address at connect time, after DNS, so a
// name that resolves to a private address is refused too.
func publicOnlyTransport() *http.Transport {
	d := &net.Dialer{Timeout: 10 * time.Second, Control: publicOnly}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	t.DialContext = d.DialContext
	return t
}

var cgnat = func() *net.IPNet { _, n, _ := net.ParseCIDR("100.64.0.0/10"); return n }()

func publicOnly(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() || cgnat.Contains(ip) {
		return fmt.Errorf("webhook to a non public address %s refused", host)
	}
	// The whole 0/8 block denotes the local network, including mapped IPv4.
	if v4 := ip.To4(); v4 != nil && v4[0] == 0 {
		return fmt.Errorf("webhook to a non public address %s refused", host)
	}
	return nil
}
