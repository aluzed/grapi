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
	d := &net.Dialer{Timeout: 10 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		ip := net.ParseIP(host)
		if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
			return fmt.Errorf("webhook to a non public address %s refused", host)
		}
		return nil
	}}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	t.DialContext = d.DialContext
	return t
}
