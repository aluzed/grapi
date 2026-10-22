// Package cron is Strapi's cron service: tasks on 5 or 6 field cron rules
// (seconds optional), or once at a date.
package cron

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	rc "github.com/robfig/cron/v3"
)

// Task is a scheduled function.
type Task func(ctx context.Context) error

// Options of a task.
type Options struct {
	// TZ is the IANA zone the rule is read in (server local time by default).
	TZ string
	// Start and End bound the runs when set.
	Start, End time.Time
	// AllowOverlap lets a run start while the previous one is still running.
	AllowOverlap bool
}

type job struct {
	id      rc.EntryID
	timer   *time.Timer
	running atomic.Bool
}

// Scheduler runs the tasks once started.
type Scheduler struct {
	mu   sync.Mutex
	c    *rc.Cron
	jobs map[string]*job
	log  *slog.Logger
	ctx  context.Context
	stop context.CancelFunc
}

var parser = rc.NewParser(rc.SecondOptional | rc.Minute | rc.Hour | rc.Dom | rc.Month | rc.Dow | rc.Descriptor)

// wallClock runs the rule on the wall clock of loc, as croner does (Strapi's
// cron): a time skipped by a DST change runs once the clock jumped (02:30
// at 03:30), a time repeated by one runs once.
type wallClock struct {
	inner rc.Schedule
	loc   *time.Location
}

func (w wallClock) Next(t time.Time) time.Time {
	lt := t.In(w.loc)
	wall := time.Date(lt.Year(), lt.Month(), lt.Day(), lt.Hour(), lt.Minute(), lt.Second(), lt.Nanosecond(), time.UTC)
	for {
		next := w.inner.Next(wall)
		if next.IsZero() {
			return next
		}
		at := time.Date(next.Year(), next.Month(), next.Day(), next.Hour(), next.Minute(), next.Second(), 0, w.loc)
		// A wall time repeated by a DST change: the first occurrence.
		_, before := at.Add(-3 * time.Hour).Zone()
		if _, now := at.Zone(); before > now {
			earlier := at.Add(-time.Duration(before-now) * time.Second)
			if earlier.In(w.loc).Hour() == next.Hour() && earlier.In(w.loc).Minute() == next.Minute() && earlier.After(t) {
				at = earlier
			}
		}
		if at.After(t) {
			return at
		}
		wall = next
	}
}

// New returns a stopped scheduler.
func New(log *slog.Logger) *Scheduler {
	if log == nil {
		log = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{c: rc.New(rc.WithParser(parser)), jobs: map[string]*job{}, log: log, ctx: ctx, stop: cancel}
}

func (s *Scheduler) wrap(name string, j *job, opts Options, task Task) func() {
	return func() {
		now := time.Now()
		if !opts.Start.IsZero() && now.Before(opts.Start) || !opts.End.IsZero() && now.After(opts.End) {
			return
		}
		if !opts.AllowOverlap && !j.running.CompareAndSwap(false, true) {
			s.log.Warn("cron: previous run still going, skipped", "task", name)
			return
		}
		defer j.running.Store(false)
		s.log.Debug("cron: start", "task", name)
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("cron: task panicked", "task", name, "panic", fmt.Sprint(r))
			}
		}()
		err := task(s.ctx)
		attrs := []any{"task", name, "duration", time.Since(now).String()}
		if err != nil {
			s.log.Error("cron: task failed", append(attrs, "error", err)...)
			return
		}
		s.log.Debug("cron: done", attrs...)
	}
}

// Add schedules task on rule ("*/5 * * * * *", "0 3 * * *", "@daily").
func (s *Scheduler) Add(name, rule string, opts Options, task Task) error {
	loc := time.Local
	if opts.TZ != "" {
		l, err := time.LoadLocation(opts.TZ)
		if err != nil {
			return fmt.Errorf("cron %s: %w", name, err)
		}
		loc = l
	}
	inner, err := parser.Parse("CRON_TZ=UTC " + rule)
	if err != nil {
		return fmt.Errorf("cron %s: %w", name, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, dup := s.jobs[name]; dup {
		return fmt.Errorf("cron %s: already scheduled", name)
	}
	j := &job{}
	j.id = s.c.Schedule(wallClock{inner: inner, loc: loc}, rc.FuncJob(s.wrap(name, j, opts, task)))
	s.jobs[name] = j
	return nil
}

// At runs task once at the given time.
func (s *Scheduler) At(name string, at time.Time, task Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, dup := s.jobs[name]; dup {
		return fmt.Errorf("cron %s: already scheduled", name)
	}
	j := &job{}
	run := s.wrap(name, j, Options{}, task)
	j.timer = time.AfterFunc(time.Until(at), func() {
		run()
		s.Remove(name)
	})
	s.jobs[name] = j
	return nil
}

// Remove unschedules a task.
func (s *Scheduler) Remove(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[name]
	if !ok {
		return
	}
	if j.timer != nil {
		j.timer.Stop()
	} else {
		s.c.Remove(j.id)
	}
	delete(s.jobs, name)
}

// Jobs lists the scheduled task names.
func (s *Scheduler) Jobs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.jobs))
	for name := range s.jobs {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Start runs the cron rules.
func (s *Scheduler) Start() { s.c.Start() }

// Stop stops the rules and cancels the running tasks' context.
func (s *Scheduler) Stop() {
	<-s.c.Stop().Done()
	s.stop()
}
