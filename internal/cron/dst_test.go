package cron

import (
	"testing"
	"time"
)

// croner's (Strapi 5.59) runs on the same dates, measured in its image.
func TestDaylightSavingTime(t *testing.T) {
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skip("no tzdata:", err)
	}
	runs := func(rule, start string, n int) []string {
		inner, err := parser.Parse("CRON_TZ=UTC " + rule)
		if err != nil {
			t.Fatal(err)
		}
		s := wallClock{inner: inner, loc: paris}
		at, _ := time.Parse(time.RFC3339, start)
		var out []string
		for range n {
			at = s.Next(at)
			out = append(out, at.UTC().Format(time.RFC3339))
		}
		return out
	}
	for _, c := range []struct {
		rule, start string
		want        []string
	}{
		{"30 2 * * *", "2026-03-28T11:00:00Z", []string{"2026-03-29T01:30:00Z", "2026-03-30T00:30:00Z", "2026-03-31T00:30:00Z"}},
		{"30 2 * * *", "2026-10-24T10:00:00Z", []string{"2026-10-25T00:30:00Z", "2026-10-26T01:30:00Z", "2026-10-27T01:30:00Z"}},
		{"0 * * * *", "2026-10-24T23:30:00Z", []string{"2026-10-25T00:00:00Z", "2026-10-25T02:00:00Z", "2026-10-25T03:00:00Z"}},
	} {
		got := runs(c.rule, c.start, 3)
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s from %s: got %v, croner %v", c.rule, c.start, got, c.want)
				break
			}
		}
	}
}
