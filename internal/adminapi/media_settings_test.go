package adminapi

import "testing"

func TestPublicOnly(t *testing.T) {
	for addr, ok := range map[string]bool{
		"169.254.169.254:80": false, "127.0.0.1:80": false, "[::1]:443": false, "10.1.2.3:80": false, "192.168.1.1:80": false,
		"172.16.0.1:80": false, "100.64.0.1:80": false, "0.0.0.0:80": false, "[fe80::1]:80": false, "[fd00::1]:80": false,
		"0.1.2.3:80": false, "[::ffff:0.1.2.3]:80": false, "[::ffff:100.64.0.1]:80": false,
		"93.184.216.34:443": true, "[2606:4700::1111]:443": true,
	} {
		if err := publicOnly("tcp", addr, nil); (err == nil) != ok {
			t.Errorf("%s: %v", addr, err)
		}
	}
}

func TestWebhookURLInProduction(t *testing.T) {
	ctx := t.Context()
	for url, ok := range map[string]bool{
		"http://127.0.0.1:9000/hook": false, "http://10.0.0.5/hook": false, "http://[::1]/hook": false,
		"http://0.1.2.3/hook": false, "http://100.64.0.1/hook": false,
		"https://93.184.216.34/hook": true,
	} {
		b := webhookBody{Name: "w", URL: url}
		if _, err := b.webhook(ctx, true); (err == nil) != ok {
			t.Errorf("production %s: %v", url, err)
		}
		if _, err := b.webhook(ctx, false); err != nil {
			t.Errorf("development accepts %s: %v", url, err)
		}
	}
}
