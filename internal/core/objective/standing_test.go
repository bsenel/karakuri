package objective

import "testing"

// The ceiling is the load-bearing field of the whole autonomy design: Karakuri
// may promote itself toward it and never past it, so the worst case of a trust
// ledger gone wrong is the level a human already wrote down.
func TestClampNeverExceedsTheCeiling(t *testing.T) {
	tests := []struct {
		name string
		decl Autonomy
		in   AutonomyLevel
		want AutonomyLevel
	}{
		{
			name: "below the ceiling passes through",
			decl: Autonomy{Level: AutonomyPropose, Ceiling: AutonomyAct},
			in:   AutonomyActWithNotice,
			want: AutonomyActWithNotice,
		},
		{
			name: "at the ceiling passes through",
			decl: Autonomy{Level: AutonomySense, Ceiling: AutonomyActWithNotice},
			in:   AutonomyActWithNotice,
			want: AutonomyActWithNotice,
		},
		{
			name: "above the ceiling is cut back",
			decl: Autonomy{Level: AutonomyPropose, Ceiling: AutonomyPropose},
			in:   AutonomyAct,
			want: AutonomyPropose,
		},
		{
			name: "an undeclared ceiling pins the objective at its starting level",
			decl: Autonomy{Level: AutonomyPropose},
			in:   AutonomyAct,
			want: AutonomyPropose,
		},
		{
			name: "a wholly undeclared autonomy pins it at propose",
			decl: Autonomy{},
			in:   AutonomyAct,
			want: AutonomyPropose,
		},
		{
			name: "a ceiling below the starting level still binds",
			decl: Autonomy{Level: AutonomyAct, Ceiling: AutonomySense},
			in:   AutonomyAct,
			want: AutonomySense,
		},
		{
			name: "an unrecognised level falls back rather than through",
			decl: Autonomy{Level: AutonomyPropose, Ceiling: AutonomyAct},
			in:   AutonomyLevel("superuser"),
			want: AutonomyPropose,
		},
		{
			// The case above cannot catch a fallback that skips the
			// ceiling, because its starting level is already under it.
			// This one can: a hand-edited row holding a typo, on an
			// objective whose ceiling was deliberately lowered beneath
			// where it started. The fallback must land on the ceiling.
			name: "an unrecognised level falls back through the ceiling, not around it",
			decl: Autonomy{Level: AutonomyAct, Ceiling: AutonomySense},
			in:   AutonomyLevel("act_wtih_notice"),
			want: AutonomySense,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.decl.Clamp(tc.in); got != tc.want {
				t.Errorf("Clamp(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// A typo in a stored declaration must fail toward asking, never toward acting.
func TestUnknownLevelRanksAsPropose(t *testing.T) {
	unknown := AutonomyLevel("full-send")
	if unknown.Valid() {
		t.Error("an unrecognised level reported itself valid")
	}
	if got, want := unknown.Rank(), AutonomyPropose.Rank(); got != want {
		t.Errorf("Rank() = %d, want %d (propose)", got, want)
	}
}

func TestAutonomyLadderOrder(t *testing.T) {
	ordered := []AutonomyLevel{AutonomySense, AutonomyPropose, AutonomyActWithNotice, AutonomyAct}
	for i, l := range ordered {
		if got := l.Rank(); got != i {
			t.Errorf("%q ranks %d, want %d", l, got, i)
		}
		if got := AutonomyByRank(i); got != l {
			t.Errorf("AutonomyByRank(%d) = %q, want %q", i, got, l)
		}
	}
	if got := AutonomyByRank(-1); got != AutonomySense {
		t.Errorf("AutonomyByRank(-1) = %q, want the bottom rung", got)
	}
	if got := AutonomyByRank(99); got != AutonomyAct {
		t.Errorf("AutonomyByRank(99) = %q, want the top rung", got)
	}
}

// The zero value of Mode is oneshot. Every objective written before standing
// mode existed deserializes with an empty mode, and must keep behaving exactly
// as it did.
func TestZeroModeIsOneshot(t *testing.T) {
	if (Objective{}).IsStanding() {
		t.Error("an objective with no mode reported itself standing")
	}
	if !(Objective{Mode: ModeStanding}).IsStanding() {
		t.Error("a standing objective did not report itself standing")
	}
	if (Objective{Mode: ModeOneshot}).IsStanding() {
		t.Error("an explicitly oneshot objective reported itself standing")
	}
}

func TestTerminalStatuses(t *testing.T) {
	terminal := []ObjectiveStatus{StatusCompleted, StatusFailed}
	resting := []ObjectiveStatus{StatusPending, StatusActive, StatusBlocked, StatusConverged}

	for _, s := range terminal {
		if !s.Terminal() {
			t.Errorf("%q should be terminal", s)
		}
	}
	for _, s := range resting {
		if s.Terminal() {
			t.Errorf("%q should not be terminal — a standing objective rests there and must be picked up again", s)
		}
	}
}

// Callers deciding what an objective may do go through the accessors, so an
// absent declaration and a zero-valued one mean the same thing.
func TestDeclarationAccessorsHandleNil(t *testing.T) {
	var o Objective
	if got := o.AutonomyDeclaration().EffectiveLevel(); got != AutonomyPropose {
		t.Errorf("nil autonomy resolved to %q, want propose", got)
	}
	if got := o.CadenceDeclaration(); got.SenseInterval() != 0 || got.HasSchedule() {
		t.Errorf("nil cadence resolved to %+v, want an empty one", got)
	}
}

func TestCadenceDurationParsing(t *testing.T) {
	c := Cadence{Sense: "15m", Every: "1h", Resync: "24h", MinInterval: "10m"}
	if c.SenseInterval().Minutes() != 15 {
		t.Errorf("sense = %s, want 15m", c.SenseInterval())
	}
	if c.ReconcileInterval().Hours() != 1 {
		t.Errorf("every = %s, want 1h", c.ReconcileInterval())
	}
	if c.ResyncInterval().Hours() != 24 {
		t.Errorf("resync = %s, want 24h", c.ResyncInterval())
	}
	if c.MinIntervalDuration().Minutes() != 10 {
		t.Errorf("min_interval = %s, want 10m", c.MinIntervalDuration())
	}

	// Unparseable and negative durations read as absent. schedule.Validate is
	// what rejects them at declaration time; by the time a value reaches here
	// it has already been checked, and a zero is an absent setting.
	bad := Cadence{Sense: "fifteen minutes", Every: "-1h"}
	if bad.SenseInterval() != 0 || bad.ReconcileInterval() != 0 {
		t.Error("a malformed duration did not read as absent")
	}
}
