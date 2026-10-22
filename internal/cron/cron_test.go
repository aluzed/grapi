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
