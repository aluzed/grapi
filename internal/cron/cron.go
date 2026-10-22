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
	spec := rule
	if opts.TZ != "" {
		if _, err := time.LoadLocation(opts.TZ); err != nil {
			return fmt.Errorf("cron %s: %w", name, err)
		}
		spec = "CRON_TZ=" + opts.TZ + " " + rule
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, dup := s.jobs[name]; dup {
		return fmt.Errorf("cron %s: already scheduled", name)
	}
	j := &job{}
	id, err := s.c.AddFunc(spec, s.wrap(name, j, opts, task))
	if err != nil {
		return fmt.Errorf("cron %s: %w", name, err)
	}
	j.id = id
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
