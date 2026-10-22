package cron

import (
	"testing"
	"time"
)

// The DST behavior of robfig/cron, pinned (docs/framework.md): a time that
// does not exist is skipped, a time that happens twice runs twice.
func TestDaylightSavingTime(t *testing.T) {
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skip("no tzdata:", err)
	}
	s, err := parser.Parse("CRON_TZ=Europe/Paris 30 2 * * *")
	if err != nil {
		t.Fatal(err)
	}
	if next := s.Next(time.Date(2026, 3, 28, 12, 0, 0, 0, paris)); next.Day() != 30 {
		t.Errorf("spring forward: 02:30 does not exist on March 29, next run %v", next)
	}
	first := s.Next(time.Date(2026, 10, 24, 12, 0, 0, 0, paris))
	second := s.Next(first)
	if first.Day() != 25 || second.Day() != 25 || second.Sub(first) != time.Hour {
		t.Errorf("fall back: 02:30 happens twice on October 25: %v, %v", first, second)
	}
}
