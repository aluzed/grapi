package httpapi

import "testing"

// The expected value is new Keygrip(["k1"]).sign("strapi_up_refresh=abc"),
// computed with the keygrip module of the reference Strapi image.
func TestKeygripSign(t *testing.T) {
	if got := keygripSign("k1", "strapi_up_refresh=abc"); got != "SqWNM7LPfWoFbIB03FsNVhndkv4" {
		t.Fatalf("%q", got)
	}
}
