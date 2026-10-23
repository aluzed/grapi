package cron

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Range is node-schedule's Range: start to end, every step.
type Range struct{ Start, End, Step int }

// Recurrence is node-schedule's RecurrenceRule (an object rule in Strapi's
// cron config). A field left nil matches any value, but Second defaults to
// 0; Month counts from 0 (January) and DayOfWeek from 0 (Sunday), as in
// node-schedule. A field is an int, a []int, a Range or a []any of those.
type Recurrence struct {
	Second, Minute, Hour, Date, Month, DayOfWeek any
	// TZ is the IANA zone the rule is read in.
	TZ string
}

// Rule is the 6 field cron rule of r.
func (r Recurrence) Rule() (string, error) {
	second := r.Second
	if second == nil {
		second = 0
	}
	fields := []struct {
		v      any
		offset int
	}{{second, 0}, {r.Minute, 0}, {r.Hour, 0}, {r.Date, 0}, {r.Month, 1}, {r.DayOfWeek, 0}}
	parts := make([]string, len(fields))
	for i, f := range fields {
		p, err := cronField(f.v, f.offset)
		if err != nil {
			return "", err
		}
		parts[i] = p
	}
	return strings.Join(parts, " "), nil
}

func cronField(v any, offset int) (string, error) {
	switch x := v.(type) {
	case nil:
		return "*", nil
	case int:
		return strconv.Itoa(x + offset), nil
	case float64:
		if x != float64(int(x)) {
			return "", fmt.Errorf("cron: %v is not a whole number", x)
		}
		return strconv.Itoa(int(x) + offset), nil
	case Range:
		step := x.Step
		if step <= 0 {
			step = 1
		}
		s := fmt.Sprintf("%d-%d", x.Start+offset, x.End+offset)
		if step > 1 {
			s += "/" + strconv.Itoa(step)
		}
		return s, nil
	case []int:
		list := make([]any, len(x))
		for i, n := range x {
			list[i] = n
		}
		return cronField(list, offset)
	case []any:
		if len(x) == 0 {
			return "", errors.New("cron: an empty list matches nothing")
		}
		out := make([]string, len(x))
		for i, e := range x {
			s, err := cronField(e, offset)
			if err != nil {
				return "", err
			}
			out[i] = s
		}
		return strings.Join(out, ","), nil
	}
	return "", fmt.Errorf("cron: unsupported recurrence value %v", v)
}

// AddRecurrence schedules task on a recurrence rule.
func (s *Scheduler) AddRecurrence(name string, r Recurrence, opts Options, task Task) error {
	rule, err := r.Rule()
	if err != nil {
		return fmt.Errorf("cron %s: %w", name, err)
	}
	if r.TZ != "" {
		opts.TZ = r.TZ
	}
	return s.Add(name, rule, opts, task)
}
