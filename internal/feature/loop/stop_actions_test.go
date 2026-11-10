package loop

import "testing"

// A stop reports the actions that returned as completed, and the one still
// running as cut and not known to have ended (Phase 37 step 5).
func TestLoopStateActionsNameTheCompletedAndTheCutAction(t *testing.T) {
	ls := &loopState{}
	ls.startAction(StopAction{CapabilityID: "read_file", EnvID: "software"})
	ls.endAction()
	ls.startAction(StopAction{
		CapabilityID: "delegate_to_cli",
		EnvID:        "software",
		Workdir:      "/tmp/wt",
		Branch:       "karakuri/wt",
	})

	done, cut := ls.actions()
	if len(done) != 1 || done[0].CapabilityID != "read_file" {
		t.Fatalf("completed = %+v, want the one read_file", done)
	}
	if cut == nil {
		t.Fatal("cut = nil, want the delegation in flight")
	}
	if cut.CapabilityID != "delegate_to_cli" || cut.Workdir != "/tmp/wt" || cut.Branch != "karakuri/wt" {
		t.Errorf("cut = %+v, want the delegation with its workdir and branch", *cut)
	}
	if cut.Ended != StopActionEndedUnknown {
		t.Errorf("cut ended = %q, want %q", cut.Ended, StopActionEndedUnknown)
	}
}

// Between actions nothing is cut, and the completed list is empty, not nil,
// before any action has run.
func TestLoopStateActionsBetweenActionsCutNothing(t *testing.T) {
	ls := &loopState{}
	done, cut := ls.actions()
	if done == nil || len(done) != 0 || cut != nil {
		t.Fatalf("actions = %v, %v; want an empty list and no cut", done, cut)
	}
	ls.startAction(StopAction{CapabilityID: "read_file"})
	ls.endAction()
	if _, cut := ls.actions(); cut != nil {
		t.Errorf("cut = %+v after the action returned, want none", *cut)
	}
}

// A step run with no loop state records nothing and does not panic.
func TestNilLoopStateRecordsNoAction(t *testing.T) {
	var ls *loopState
	ls.startAction(StopAction{CapabilityID: "read_file"})
	ls.endAction()
}
