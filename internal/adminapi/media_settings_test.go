package adminapi

import "testing"

func TestPublicOnly(t *testing.T) {
	for addr, ok := range map[string]bool{
		"169.254.169.254:80": false, "127.0.0.1:80": false, "[::1]:443": false, "10.1.2.3:80": false, "192.168.1.1:80": false,
		"172.16.0.1:80": false, "100.64.0.1:80": false, "0.0.0.0:80": false, "[fe80::1]:80": false, "[fd00::1]:80": false,
		"93.184.216.34:443": true, "[2606:4700::1111]:443": true,
	} {
		if err := publicOnly("tcp", addr, nil); (err == nil) != ok {
			t.Errorf("%s: %v", addr, err)
		}
	}
}
