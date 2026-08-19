package schedule

import (
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/objective"
)

func mustLoad(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("zoneinfo for %s unavailable: %v", name, err)
	}
	return loc
}

// A newly declared standing objective is due immediately on both tiers. The
// first useful answer to "hold this state" is whether the state already holds.
func TestNeverRunIsDueNow(t *testing.T) {
	now := time.Date(2026, 3, 4, 14, 0, 0, 0, time.UTC)
	c := objective.Cadence{Sense: "15m", Every: "1h"}

	plan, err := Next(c, Reference{Now: now})
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if !plan.Sense.Equal(now) {
		t.Errorf("sense = %s, want %s", plan.Sense, now)
	}
	if !plan.Reconcile.Equal(now) {
		t.Errorf("reconcile = %s, want %s", plan.Reconcile, now)
	}
	if !plan.Due.Equal(now) {
		t.Errorf("due = %s, want %s", plan.Due, now)
	}
}

// A cadence that declares nothing never becomes due on its own. It is a
// standing objective that reconciles when asked, which is a legitimate thing
// to declare and must not be confused with one that is overdue since the epoch.
func TestNoScheduleIsNeverDue(t *testing.T) {
	now := time.Date(2026, 3, 4, 14, 0, 0, 0, time.UTC)

	plan, err := Next(objective.Cadence{}, Reference{Now: now})
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if !plan.Due.IsZero() {
		t.Errorf("due = %s, want zero", plan.Due)
	}
}

// The sense tier is cheap, so it keeps its own clock independent of reconciles.
func TestSenseAndReconcileAdvanceIndependently(t *testing.T) {
	now := time.Date(2026, 3, 4, 14, 0, 0, 0, time.UTC)
	c := objective.Cadence{Sense: "15m", Every: "6h"}
	ref := Reference{
		Now:              now,
		LastSensedAt:     now.Add(-5 * time.Minute),
		LastReconciledAt: now.Add(-1 * time.Hour),
	}

	plan, err := Next(c, ref)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if want := now.Add(10 * time.Minute); !plan.Sense.Equal(want) {
		t.Errorf("sense = %s, want %s", plan.Sense, want)
	}
	if want := now.Add(5 * time.Hour); !plan.Reconcile.Equal(want) {
		t.Errorf("reconcile = %s, want %s", plan.Reconcile, want)
	}
	if !plan.Due.Equal(plan.Sense) {
		t.Errorf("due = %s, want the sense time %s", plan.Due, plan.Sense)
	}
}

// "08:00 in New York" must stay 08:00 in New York across a daylight-saving
// change, which means the UTC interval between two firings is 23 hours in
// spring and 25 in autumn. A schedule pinned to UTC would drift an hour
// against the person reading the report.
func TestDailyAtHoldsLocalClockAcrossDST(t *testing.T) {
	ny := mustLoad(t, "America/New_York")
	c := objective.Cadence{DailyAt: "08:00", Timezone: "America/New_York"}

	tests := []struct {
		name string
		last time.Time
		want time.Time
		gap  time.Duration
	}{
		{
			name: "spring forward",
			last: time.Date(2026, 3, 7, 8, 0, 0, 0, ny),
			want: time.Date(2026, 3, 8, 8, 0, 0, 0, ny),
			gap:  23 * time.Hour,
		},
		{
			name: "fall back",
			last: time.Date(2026, 10, 31, 8, 0, 0, 0, ny),
			want: time.Date(2026, 11, 1, 8, 0, 0, 0, ny),
			gap:  25 * time.Hour,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := Next(c, Reference{Now: tc.last, LastReconciledAt: tc.last})
			if err != nil {
				t.Fatalf("Next: %v", err)
			}
			if !plan.Reconcile.Equal(tc.want) {
				t.Fatalf("reconcile = %s, want %s", plan.Reconcile.In(ny), tc.want)
			}
			if got := plan.Reconcile.Sub(tc.last); got != tc.gap {
				t.Errorf("elapsed = %s, want %s — the local clock, not the UTC offset, is what was promised", got, tc.gap)
			}
			if h := plan.Reconcile.In(ny).Hour(); h != 8 {
				t.Errorf("local hour = %d, want 8", h)
			}
		})
	}
}

// Istanbul has been at a fixed UTC+3 since 2016. It is the case that catches a
// schedule quietly evaluated in UTC: 08:00 there is 05:00 Z, all year.
func TestFixedOffsetZone(t *testing.T) {
	ist := mustLoad(t, "Europe/Istanbul")
	c := objective.Cadence{Cron: "0 8 * * 1-5", Timezone: "Europe/Istanbul"}

	// A Monday.
	last := time.Date(2026, 3, 2, 8, 0, 0, 0, ist)
	plan, err := Next(c, Reference{Now: last, LastReconciledAt: last})
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	want := time.Date(2026, 3, 3, 8, 0, 0, 0, ist)
	if !plan.Reconcile.Equal(want) {
		t.Fatalf("reconcile = %s, want %s", plan.Reconcile.In(ist), want)
	}
	if h := plan.Reconcile.UTC().Hour(); h != 5 {
		t.Errorf("UTC hour = %d, want 5", h)
	}
}

// Weekday-only schedules skip the weekend rather than firing on Saturday.
func TestCronSkipsWeekend(t *testing.T) {
	c := objective.Cadence{Cron: "0 8 * * 1-5"}
	friday := time.Date(2026, 3, 6, 8, 0, 0, 0, time.UTC)

	plan, err := Next(c, Reference{Now: friday, LastReconciledAt: friday})
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if got := plan.Reconcile.Weekday(); got != time.Monday {
		t.Errorf("next firing is a %s, want Monday", got)
	}
}

// Resync is a ceiling on staleness, not an alternative schedule: whichever of
// the two comes first is what happens.
func TestResyncWinsWhenSooner(t *testing.T) {
	now := time.Date(2026, 3, 4, 14, 0, 0, 0, time.UTC)
	last := now.Add(-2 * time.Hour)

	plan, err := Next(
		objective.Cadence{Cron: "0 8 * * *", Resync: "6h"},
		Reference{Now: now, LastReconciledAt: last},
	)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if want := last.Add(6 * time.Hour); !plan.Reconcile.Equal(want) {
		t.Errorf("reconcile = %s, want the resync horizon %s", plan.Reconcile, want)
	}
}

// A quiet window defers work to its end. It does not drop it.
func TestQuietWindowDefersRatherThanDrops(t *testing.T) {
	c := objective.Cadence{Quiet: []string{"22:00-07:00"}}

	tests := []struct {
		name string
		at   time.Time
		want time.Time
	}{
		{
			name: "after midnight defers to this morning",
			at:   time.Date(2026, 3, 4, 3, 0, 0, 0, time.UTC),
			want: time.Date(2026, 3, 4, 7, 0, 0, 0, time.UTC),
		},
		{
			name: "before midnight defers to tomorrow morning",
			at:   time.Date(2026, 3, 4, 23, 0, 0, 0, time.UTC),
			want: time.Date(2026, 3, 5, 7, 0, 0, 0, time.UTC),
		},
		{
			name: "outside the window is untouched",
			at:   time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC),
			want: time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC),
		},
		{
			name: "the closing edge is already open",
			at:   time.Date(2026, 3, 4, 7, 0, 0, 0, time.UTC),
			want: time.Date(2026, 3, 4, 7, 0, 0, 0, time.UTC),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := deferPastQuiet(tc.at, c, time.UTC)
			if !got.Equal(tc.want) {
				t.Errorf("deferred to %s, want %s", got, tc.want)
			}
		})
	}
}

// Adjacent windows chain: landing in one and being pushed into the next must
// keep going rather than stopping inside the second.
func TestAdjacentQuietWindowsChain(t *testing.T) {
	c := objective.Cadence{Quiet: []string{"22:00-02:00", "02:00-06:00"}}
	at := time.Date(2026, 3, 4, 23, 0, 0, 0, time.UTC)

	got := deferPastQuiet(at, c, time.UTC)
	want := time.Date(2026, 3, 5, 6, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("deferred to %s, want %s", got, want)
	}
}

// A blackout covering the whole day is a mistake. Running anyway beats going
// permanently dark, and it beats looping forever looking for an opening.
func TestQuietWindowsCoveringEveryHourStillRun(t *testing.T) {
	c := objective.Cadence{Quiet: []string{"00:00-12:00", "12:00-00:00"}}
	at := time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC)

	got := deferPastQuiet(at, c, time.UTC)
	if !got.Equal(at) {
		t.Errorf("deferred to %s, want the original %s", got, at)
	}
}

// Sensing is free and read-only, so it is never held back by a quiet window.
// Sensing through the night is how the morning reconcile knows what happened.
func TestQuietWindowsDoNotHoldBackSensing(t *testing.T) {
	c := objective.Cadence{Sense: "15m", Quiet: []string{"22:00-07:00"}}
	now := time.Date(2026, 3, 4, 23, 0, 0, 0, time.UTC)

	plan, err := Next(c, Reference{Now: now, LastSensedAt: now.Add(-15 * time.Minute)})
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if !plan.Sense.Equal(now) {
		t.Errorf("sense = %s, want %s — the cheap tier does not observe quiet hours", plan.Sense, now)
	}
}

func TestAllowedHonoursMinIntervalAndQuiet(t *testing.T) {
	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		cadence objective.Cadence
		ref     Reference
		wantOK  bool
		wantAt  time.Time
	}{
		{
			name:    "inside the floor defers to it",
			cadence: objective.Cadence{MinInterval: "30m"},
			ref:     Reference{Now: now, LastReconciledAt: now.Add(-10 * time.Minute)},
			wantOK:  false,
			wantAt:  now.Add(20 * time.Minute),
		},
		{
			name:    "past the floor is allowed",
			cadence: objective.Cadence{MinInterval: "30m"},
			ref:     Reference{Now: now, LastReconciledAt: now.Add(-45 * time.Minute)},
			wantOK:  true,
			wantAt:  now,
		},
		{
			name:    "a first run has no floor to clear",
			cadence: objective.Cadence{MinInterval: "30m"},
			ref:     Reference{Now: now},
			wantOK:  true,
			wantAt:  now,
		},
		{
			name:    "quiet hours defer to the opening",
			cadence: objective.Cadence{Quiet: []string{"11:00-14:00"}},
			ref:     Reference{Now: now},
			wantOK:  false,
			wantAt:  time.Date(2026, 3, 4, 14, 0, 0, 0, time.UTC),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ok, at, err := Allowed(tc.cadence, tc.ref, now)
			if err != nil {
				t.Fatalf("Allowed: %v", err)
			}
			if ok != tc.wantOK {
				t.Errorf("allowed = %v, want %v", ok, tc.wantOK)
			}
			if !at.Equal(tc.wantAt) {
				t.Errorf("at = %s, want %s", at, tc.wantAt)
			}
		})
	}
}

// A syntactically valid expression can still never fire — 30 February parses
// and matches nothing. It must resolve to "no schedule" rather than to a
// zero timestamp that reads as permanently overdue.
func TestCronThatNeverFiresIsNotOverdue(t *testing.T) {
	c := objective.Cadence{Cron: "0 0 30 2 *"}
	last := time.Date(2026, 3, 4, 14, 0, 0, 0, time.UTC)

	plan, err := Next(c, Reference{Now: last, LastReconciledAt: last})
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if !plan.Reconcile.IsZero() {
		t.Errorf("reconcile = %s, want zero", plan.Reconcile)
	}
	if !plan.Due.IsZero() {
		t.Errorf("due = %s, want zero", plan.Due)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cadence objective.Cadence
		wantErr bool
	}{
		{name: "empty is valid", cadence: objective.Cadence{}},
		{name: "interval", cadence: objective.Cadence{Sense: "15m", Every: "1h"}},
		{name: "cron with zone", cadence: objective.Cadence{Cron: "0 8 * * 1-5", Timezone: "Europe/Istanbul"}},
		{name: "daily_at", cadence: objective.Cadence{DailyAt: "08:00"}},
		{name: "cron descriptor", cadence: objective.Cadence{Cron: "@daily"}},
		{name: "quiet window", cadence: objective.Cadence{Quiet: []string{"22:00-07:00"}}},

		{name: "two schedules", cadence: objective.Cadence{Every: "1h", Cron: "0 8 * * *"}, wantErr: true},
		{name: "three schedules", cadence: objective.Cadence{Every: "1h", Cron: "0 8 * * *", DailyAt: "08:00"}, wantErr: true},
		{name: "unparseable duration", cadence: objective.Cadence{Sense: "fifteen minutes"}, wantErr: true},
		{name: "negative duration", cadence: objective.Cadence{Sense: "-15m"}, wantErr: true},
		{name: "zero duration", cadence: objective.Cadence{Every: "0s"}, wantErr: true},
		{name: "unknown zone", cadence: objective.Cadence{Timezone: "Mars/Olympus_Mons"}, wantErr: true},
		{name: "malformed cron", cadence: objective.Cadence{Cron: "not a cron"}, wantErr: true},
		{name: "cron with too many fields", cadence: objective.Cadence{Cron: "0 0 8 * * 1-5"}, wantErr: true},
		{name: "daily_at without minutes", cadence: objective.Cadence{DailyAt: "08"}, wantErr: true},
		{name: "daily_at hour out of range", cadence: objective.Cadence{DailyAt: "25:00"}, wantErr: true},
		{name: "quiet window without a dash", cadence: objective.Cadence{Quiet: []string{"22:00"}}, wantErr: true},
		{name: "quiet window minute out of range", cadence: objective.Cadence{Quiet: []string{"22:00-07:99"}}, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.cadence)
			if tc.wantErr && err == nil {
				t.Error("Validate accepted a cadence it should have rejected")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Validate rejected a valid cadence: %v", err)
			}
		})
	}
}
