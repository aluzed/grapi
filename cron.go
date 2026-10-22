package grapi

import (
	"context"
	"time"

	"github.com/aluzed/grapi/internal/cron"
)

// CronOptions of a task.
type CronOptions struct {
	// TZ is the IANA zone of the rule ("Europe/Paris").
	TZ string
	// Start and End bound the runs when set.
	Start, End time.Time
	// AllowOverlap lets a run start while the previous one still runs.
	AllowOverlap bool
}

type cronTask struct {
	name, rule string
	at         time.Time
	opts       CronOptions
	task       func(ctx context.Context) error
}

// Cron schedules task on a 5 or 6 field cron rule ("*/5 * * * * *").
// Tasks run only when server.cron.enabled is true, as in Strapi.
func (a *App) Cron(name, rule string, opts CronOptions, task func(ctx context.Context) error) {
	a.cronTasks = append(a.cronTasks, cronTask{name: name, rule: rule, opts: opts, task: task})
}

// CronAt runs task once at the given time.
func (a *App) CronAt(name string, at time.Time, task func(ctx context.Context) error) {
	a.cronTasks = append(a.cronTasks, cronTask{name: name, at: at, task: task})
}

func (a *App) startCron(enabled bool, s *cron.Scheduler) error {
	if !enabled {
		return nil
	}
	for _, t := range a.cronTasks {
		var err error
		if t.rule == "" {
			err = s.At(t.name, t.at, t.task)
		} else {
			err = s.Add(t.name, t.rule, cron.Options{TZ: t.opts.TZ, Start: t.opts.Start, End: t.opts.End, AllowOverlap: t.opts.AllowOverlap}, t.task)
		}
		if err != nil {
			return err
		}
	}
	s.Start()
	return nil
}
