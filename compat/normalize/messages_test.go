package normalize

import "testing"

func TestSortComponentUIDLists(t *testing.T) {
	cases := map[string]string{
		"body[0].__component must be one of the following values: shared.seo, blocks.hero, blocks.rich-text": "body[0].__component must be one of the following values: blocks.hero, blocks.rich-text, shared.seo",
		"stage must be one of the following values: writing, idea, done":                                     "stage must be one of the following values: writing, idea, done",
		"no list here": "no list here",
	}
	for in, want := range cases {
		if got := sortComponentUIDLists(in); got != want {
			t.Errorf("sortComponentUIDLists(%q) = %q, want %q", in, got, want)
		}
	}
}
