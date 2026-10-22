package naming

import "testing"

// Cases of Strapi's identifiers.test.ts.
func TestStrapiHashCases(t *testing.T) {
	for _, c := range []struct {
		in   string
		n    int
		want string
	}{
		{"123456789", 2, "24"},
		{"123456789", 4, "2434"},
		{"123456789", 3, "243"},
		{"123456789", 50, "24347b9c4b6da2fc9cde08c87f33edd2e603c8dcd6840e6b39"},
		{"myData", 5, "93da1"},
	} {
		if got := Hash(c.in, c.n); got != c.want {
			t.Errorf("Hash(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
		}
	}
}

func TestStrapiShortenCases(t *testing.T) {
	for _, c := range []struct {
		n    int
		want string
		err  bool
	}{
		{10, "1234567890", false},
		{100, "1234567890", false},
		{7, "", true},
		{9, "1234cd65a", false},
		{0, "", true},
		{-3, "", true},
	} {
		got, err := shorten("1234567890", c.n)
		if (err != nil) != c.err || got != c.want {
			t.Errorf("shorten(%d) = %q, %v; want %q, error %v", c.n, got, err, c.want, c.err)
		}
	}
}

func TestStrapiTokenCases(t *testing.T) {
	c := func(name string) Token { return Token{Name: name, Compressible: true} }
	f := func(name string) Token { return Token{Name: name} }
	for _, tc := range []struct {
		max    int
		tokens []Token
		want   string
	}{
		{22, []Token{c("1234567890"), c("12345"), f("links")}, "1234567890_12345_links"},
		{22, []Token{c("1234_56789"), c("123_4"), f("links")}, "1234_56789_123_4_links"},
		{13, []Token{c("123456789012345")}, "1234567878db8"},
		{13, []Token{c("1234567_9012345")}, "1234567_47b4e"},
		{13, []Token{c("12345678_012345")}, "12345678867f6"},
		{5, []Token{c("12345")}, "12345"},
		{10, []Token{c("12345")}, "12345"},
		{21, []Token{c("1234567890"), c("12345"), f("links")}, "1234cd65a_12345_links"},
		{21, []Token{c("1234567890"), c("12345"), {Name: "links", ShortName: "lnk"}}, "1234567890_12345_lnk"},
		{21, []Token{c("1234567890"), c("1234567890"), f("links")}, ""},
		{23, []Token{c("1234567890"), c("1234567890"), f("links")}, "123cd65a_123cd65a_links"},
		{12, []Token{c("12"), c("12"), c("12"), c("12")}, "12_12_12_12"},
		{12, []Token{c("12"), c("12"), c("12"), c("1"), c("12")}, ""},
		{5, []Token{f("123456")}, ""},
		{12, []Token{f("123456"), f("123456")}, ""},
		{30, []Token{c("1234567890"), c("12345"), c("0987654321"), f("links")}, "1234cd65a_12345_0984addb_links"},
		{34, []Token{f("inv_order"), c("1234567890"), c("12345"), c("0987654321")}, "inv_order_1234cd65a_12345_0984addb"},
		{31, []Token{f("pre"), c("1234567890"), f("in"), c("3456789012"), f("post")}, "pre_1234cd65a_in_3456be378_post"},
		{32, []Token{f("pre"), c("1234567890"), f("in"), c("3456789012"), f("post")}, "pre_1234567890_in_3456be378_post"},
		{34, []Token{f("1234567890"), f("2345678901"), f("3456789012")}, "1234567890_2345678901_3456789012"},
	} {
		got, err := New(tc.max).FromTokens(tc.tokens...)
		if tc.want == "" {
			if err == nil {
				t.Errorf("max %d %v: got %q, want an error", tc.max, tc.tokens, got)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("max %d %v: got %q, %v; want %q", tc.max, tc.tokens, got, err, tc.want)
		}
	}
}
