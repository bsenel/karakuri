// Package schedule turns a declared cadence into timestamps.
//
// It is the only place that knows how to read a cron expression, and it is
// deliberately a calculator rather than a scheduler: it answers "when next",
// and the reconcile supervisor owns the ticking, the lease and the dispatch.
// robfig/cron ships a scheduler too; using it would put a second timing
// authority in the process, each with its own idea of what is running.
//
// The split of responsibility across the two tiers is the important part:
//
//   - Sensing is cheap and read-only, so it is never held back by quiet
//     windows or by the minimum interval. Sensing through the night is how
//     the morning reconcile knows what happened while nobody was watching.
//   - Reconciling costs money and changes the world, so it respects both.
package schedule

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/bsenel/karakuri/internal/core/objective"
)

// Reference is what the caller already knows: the clock, and when this
// objective was last looked at.
type Reference struct {
	Now              time.Time
	LastSensedAt     time.Time
	LastReconciledAt time.Time
}

// Plan is the resolved timing for one objective. A zero time means "never on
// its own" — an objective can legitimately have no schedule at all and
// reconcile only when somebody asks.
type Plan struct {
	Sense     time.Time
	Reconcile time.Time
	// Due is the earlier of the two, and the only value the supervisor's
	// due-wheel indexes on.
	Due time.Time
}

// Next resolves a cadence into the next sense time, the next reconcile time,
// and the earlier of the two.
//
// An objective that has never run is due immediately, on both tiers. That is
// not impatience: somebody has just declared a desired state, and the first
// useful thing to tell them is whether the world already matches it. Waiting
// out a full interval to answer a question that is already answerable would
// also mean a server that restarts more often than the interval never gets
// round to it.
func Next(c objective.Cadence, ref Reference) (Plan, error) {
	loc, err := location(c)
	if err != nil {
		return Plan{}, err
	}
	now := ref.Now.UTC()

	var plan Plan

	if iv := c.SenseInterval(); iv > 0 {
		if ref.LastSensedAt.IsZero() {
			plan.Sense = now
		} else {
			plan.Sense = ref.LastSensedAt.Add(iv).UTC()
		}
	}

	plan.Reconcile, err = nextReconcile(c, ref, loc)
	if err != nil {
		return Plan{}, err
	}
	if !plan.Reconcile.IsZero() {
		plan.Reconcile = deferPastQuiet(plan.Reconcile, c, loc)
	}

	plan.Due = earliest(plan.Sense, plan.Reconcile)
	return plan, nil
}

// nextReconcile combines the declared schedule with the resync horizon. Both
// can be set, and the earlier wins: resync is a ceiling on staleness, not an
// alternative to the schedule.
func nextReconcile(c objective.Cadence, ref Reference, loc *time.Location) (time.Time, error) {
	now := ref.Now.UTC()
	last := ref.LastReconciledAt

	var scheduled time.Time
	switch {
	case c.Cron != "" || c.DailyAt != "":
		expr, err := cronExpression(c)
		if err != nil {
			return time.Time{}, err
		}
		sched, err := cron.ParseStandard(expr)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse cron %q: %w", expr, err)
		}
		if last.IsZero() {
			// Never reconciled: due now, and the cron takes over from
			// the next firing.
			scheduled = now
		} else {
			// Next() is evaluated in the location of the time handed
			// to it, which is how "08:00 Europe/Istanbul" stays 08:00
			// across a daylight-saving change.
			scheduled = sched.Next(last.In(loc)).UTC()
		}
	case c.ReconcileInterval() > 0:
		if last.IsZero() {
			scheduled = now
		} else {
			scheduled = last.Add(c.ReconcileInterval()).UTC()
		}
	}

	var resync time.Time
	if iv := c.ResyncInterval(); iv > 0 {
		if last.IsZero() {
			resync = now
		} else {
			resync = last.Add(iv).UTC()
		}
	}

	return earliest(scheduled, resync), nil
}

// Allowed reports whether an expensive reconcile may start at the given
// moment, and if not, the earliest moment it may.
//
// Two things hold it back. The minimum interval is the anti-thrash floor: an
// environment that changes every few seconds must not be able to drive a loop
// that costs money every few seconds. Quiet windows are the operator saying
// there are hours in which this system does not touch anything.
//
// Neither drops the work. Both return a later time, and the caller re-arms for
// it — a report that would have gone out at 3am arrives at 7am.
func Allowed(c objective.Cadence, ref Reference, at time.Time) (bool, time.Time, error) {
	loc, err := location(c)
	if err != nil {
		return false, time.Time{}, err
	}
	at = at.UTC()

	if iv := c.MinIntervalDuration(); iv > 0 && !ref.LastReconciledAt.IsZero() {
		if floor := ref.LastReconciledAt.Add(iv).UTC(); floor.After(at) {
			at = floor
		}
	}
	if open := deferPastQuiet(at, c, loc); open.After(at) {
		at = open
	}

	if at.After(ref.Now.UTC()) {
		return false, at, nil
	}
	return true, at, nil
}

// Validate rejects a cadence at declaration time rather than at 3am.
//
// Everything here is a mistake somebody makes once: two schedules that
// disagree, a timezone that does not exist on the host, a cron expression with
// a typo. A cadence that parses is not necessarily wise, but a cadence that
// does not parse is a standing objective that silently never runs, and that
// failure is invisible precisely because nothing happening is what it looks
// like when everything is fine.
func Validate(c objective.Cadence) error {
	declared := 0
	for _, set := range []bool{c.Every != "", c.Cron != "", c.DailyAt != ""} {
		if set {
			declared++
		}
	}
	if declared > 1 {
		return fmt.Errorf("cadence declares more than one reconcile schedule: set exactly one of every, cron, daily_at")
	}

	for field, raw := range map[string]string{
		"sense":        c.Sense,
		"every":        c.Every,
		"resync":       c.Resync,
		"min_interval": c.MinInterval,
	} {
		if raw == "" {
			continue
		}
		d, err := time.ParseDuration(raw)
		if err != nil {
			return fmt.Errorf("cadence %s %q: %w", field, raw, err)
		}
		if d <= 0 {
			return fmt.Errorf("cadence %s %q must be positive", field, raw)
		}
	}

	if _, err := location(c); err != nil {
		return err
	}

	if c.Cron != "" || c.DailyAt != "" {
		expr, err := cronExpression(c)
		if err != nil {
			return err
		}
		if _, err := cron.ParseStandard(expr); err != nil {
			return fmt.Errorf("cadence cron %q: %w", expr, err)
		}
	}

	for _, w := range c.Quiet {
		if _, err := parseWindow(w); err != nil {
			return err
		}
	}
	return nil
}

// cronExpression renders whichever wall-clock form was declared into a
// standard five-field expression. daily_at is sugar, and turning it into cron
// here means there is one schedule evaluator rather than two.
func cronExpression(c objective.Cadence) (string, error) {
	if c.Cron != "" {
		return c.Cron, nil
	}
	h, m, err := parseClock(c.DailyAt)
	if err != nil {
		return "", fmt.Errorf("cadence daily_at %q: %w", c.DailyAt, err)
	}
	return fmt.Sprintf("%d %d * * *", m, h), nil
}

func location(c objective.Cadence) (*time.Location, error) {
	if c.Timezone == "" {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return nil, fmt.Errorf("cadence timezone %q: %w", c.Timezone, err)
	}
	return loc, nil
}

// window is a blackout expressed in local wall-clock minutes from midnight.
// End may be less than Start, which is how a window wraps midnight.
type window struct {
	start int
	end   int
}

func (w window) wraps() bool { return w.end <= w.start }

// contains reports whether a local minute-of-day falls inside the window. The
// start is inclusive and the end exclusive, so two adjacent windows do not
// both claim the boundary minute.
func (w window) contains(minute int) bool {
	if w.wraps() {
		return minute >= w.start || minute < w.end
	}
	return minute >= w.start && minute < w.end
}

func parseWindow(raw string) (window, error) {
	from, to, ok := strings.Cut(raw, "-")
	if !ok {
		return window{}, fmt.Errorf("cadence quiet window %q: want HH:MM-HH:MM", raw)
	}
	sh, sm, err := parseClock(strings.TrimSpace(from))
	if err != nil {
		return window{}, fmt.Errorf("cadence quiet window %q: %w", raw, err)
	}
	eh, em, err := parseClock(strings.TrimSpace(to))
	if err != nil {
		return window{}, fmt.Errorf("cadence quiet window %q: %w", raw, err)
	}
	return window{start: sh*60 + sm, end: eh*60 + em}, nil
}

func parseClock(raw string) (hour, minute int, err error) {
	h, m, ok := strings.Cut(strings.TrimSpace(raw), ":")
	if !ok {
		return 0, 0, fmt.Errorf("want HH:MM, got %q", raw)
	}
	hour, err = strconv.Atoi(h)
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, fmt.Errorf("hour out of range in %q", raw)
	}
	minute, err = strconv.Atoi(m)
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("minute out of range in %q", raw)
	}
	return hour, minute, nil
}

// deferPastQuiet pushes a time forward to the first moment outside every quiet
// window, or returns it unchanged when it is already outside them.
//
// It steps window by window rather than minute by minute, and gives up after
// one pass per window plus one. Adjacent windows chain (22:00-02:00 followed
// by 02:00-06:00 defers to 06:00); windows that between them cover the whole
// day would otherwise loop forever, so the bound returns the original time and
// the work happens rather than never happening. An operator who blacks out
// every hour of the day has said something they did not mean, and the useful
// response is to run anyway rather than to go silently dark.
func deferPastQuiet(at time.Time, c objective.Cadence, loc *time.Location) time.Time {
	if len(c.Quiet) == 0 {
		return at
	}
	windows := make([]window, 0, len(c.Quiet))
	for _, raw := range c.Quiet {
		w, err := parseWindow(raw)
		if err != nil {
			continue // Validate rejects these at declaration time.
		}
		windows = append(windows, w)
	}
	if len(windows) == 0 {
		return at
	}

	original := at
	for step := 0; step <= len(windows); step++ {
		local := at.In(loc)
		minute := local.Hour()*60 + local.Minute()

		var hit *window
		for i := range windows {
			if windows[i].contains(minute) {
				hit = &windows[i]
				break
			}
		}
		if hit == nil {
			return at
		}

		day := local
		if hit.wraps() && minute >= hit.start {
			// Inside the pre-midnight half; the window ends tomorrow.
			day = local.AddDate(0, 0, 1)
		}
		// Built from wall-clock fields rather than by adding a duration to
		// midnight: on the day a zone springs forward, midnight plus seven
		// hours is 08:00 local, and a window declared to end at 07:00 would
		// end an hour late once a year.
		end := time.Date(day.Year(), day.Month(), day.Day(), hit.end/60, hit.end%60, 0, 0, loc)
		at = end.UTC()
	}
	return original
}

// earliest returns the earlier of two times, ignoring zero values. Zero means
// "no such deadline", not "the beginning of time", and treating it as the
// latter would make every unscheduled objective permanently overdue.
func earliest(a, b time.Time) time.Time {
	switch {
	case a.IsZero():
		return b
	case b.IsZero():
		return a
	case a.Before(b):
		return a
	default:
		return b
	}
}
