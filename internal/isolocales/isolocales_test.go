package isolocales

import "testing"

func TestName(t *testing.T) {
	if n, ok := Name("en"); !ok || n != "English (en)" {
		t.Fatalf("en: %q %v", n, ok)
	}
	if n, ok := Name("fr"); !ok || n != "French (fr)" {
		t.Fatalf("fr: %q %v", n, ok)
	}
	if _, ok := Name("xx"); ok {
		t.Fatal("xx is not a locale")
	}
}
