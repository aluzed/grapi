package cron

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestScheduler(t *testing.T) {
	s := New(nil)
	var every, once atomic.Int32
	if err := s.Add("tick", "* * * * * *", Options{}, func(context.Context) error { every.Add(1); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("paris", "0 3 * * *", Options{TZ: "Europe/Paris"}, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("bad", "not a rule", Options{}, nil); err == nil {
		t.Fatal("want a parse error")
	}
	if err := s.At("once", time.Now().Add(100*time.Millisecond), func(context.Context) error { once.Add(1); return nil }); err != nil {
		t.Fatal(err)
	}
	s.Start()
	time.Sleep(2200 * time.Millisecond)
	s.Stop()
	if every.Load() < 2 || once.Load() != 1 {
		t.Fatalf("every %d, once %d", every.Load(), once.Load())
	}
	if jobs := s.Jobs(); len(jobs) != 2 || jobs[0] != "paris" || jobs[1] != "tick" {
		t.Fatalf("jobs %v (the one-shot task removes itself)", jobs)
	}
}

func TestNoOverlap(t *testing.T) {
	s := New(nil)
	var runs atomic.Int32
	j := &job{}
	run := s.wrap("slow", j, Options{}, func(context.Context) error { runs.Add(1); time.Sleep(300 * time.Millisecond); return nil })
	go run()
	time.Sleep(50 * time.Millisecond)
	run()
	time.Sleep(300 * time.Millisecond)
	if runs.Load() != 1 {
		t.Fatalf("runs %d, the second must be skipped", runs.Load())
	}
}

func schedule(t *testing.T, rule, tz string) wallClock {
	t.Helper()
	loc, err := time.LoadLocation(tz)
	if err != nil {
		t.Fatal(err)
	}
	inner, err := parser.Parse("CRON_TZ=UTC " + rule)
	if err != nil {
		t.Fatal(err)
	}
	return wallClock{inner: inner, loc: loc}
}

func TestEveryFiveSeconds(t *testing.T) {
	w := schedule(t, "*/5 * * * * *", "UTC")
	at := time.Date(2026, 10, 23, 9, 0, 3, 0, time.UTC)
	for i := 0; i < 4; i++ {
		next := w.Next(at)
		if next.Second()%5 != 0 || next.Sub(at) > 5*time.Second || !next.After(at) {
			t.Fatalf("after %v: %v", at, next)
		}
		at = next
	}
}

func TestDailyRuleAcrossDST(t *testing.T) {
	paris, _ := time.LoadLocation("Europe/Paris")
	w := schedule(t, "0 3 * * *", "Europe/Paris")
	// Clocks go back on 2026-10-25 at 03:00 (CEST to CET).
	first := w.Next(time.Date(2026, 10, 24, 12, 0, 0, 0, paris))
	second := w.Next(first)
	third := w.Next(second)
	for _, at := range []time.Time{first, second, third} {
		if l := at.In(paris); l.Hour() != 3 || l.Minute() != 0 {
			t.Fatalf("03:00 in Paris every day: %v", l)
		}
	}
	if d := second.Sub(first); d != 24*time.Hour && d != 25*time.Hour {
		t.Fatalf("one run a day: %v", d)
	}
	hourly := schedule(t, "0 * * * *", "Europe/Paris")
	at := time.Date(2026, 10, 25, 0, 30, 0, 0, paris)
	var runs []time.Time
	for i := 0; i < 5; i++ {
		at = hourly.Next(at)
		runs = append(runs, at)
	}
	for i := 1; i < len(runs); i++ {
		if d := runs[i].Sub(runs[i-1]); d < time.Hour || d > 2*time.Hour {
			t.Fatalf("hourly runs stay an hour or so apart: %v", runs)
		}
	}
}

func TestRecurrenceRule(t *testing.T) {
	for _, c := range []struct {
		r    Recurrence
		want string
	}{
		{Recurrence{Hour: 14, Minute: 30}, "0 30 14 * * *"},
		{Recurrence{DayOfWeek: []int{1, 3, 5}, Hour: 9, Minute: 0}, "0 0 9 * * 1,3,5"},
		{Recurrence{Month: 0, Date: 1}, "0 * * 1 1 *"},
		{Recurrence{Minute: Range{Start: 0, End: 30, Step: 10}}, "0 0-30/10 * * * *"},
		{Recurrence{Second: []any{0, 30.0}}, "0,30 * * * * *"},
	} {
		got, err := c.r.Rule()
		if err != nil || got != c.want {
			t.Errorf("%+v: %q %v, want %q", c.r, got, err, c.want)
		}
	}
	if _, err := (Recurrence{Hour: 1.5}).Rule(); err == nil {
		t.Error("a fraction is refused")
	}
	s := New(nil)
	if err := s.AddRecurrence("r", Recurrence{Hour: 3, TZ: "Europe/Paris"}, Options{}, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
}
