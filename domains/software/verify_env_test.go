package software

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

func newTestVerifyEnv() *verifyEnv {
	return newVerifyEnv("software.env.verify", newShellEnv("software.env.verify", "", 30*time.Second))
}

// The whole point: a criterion verified by run_tests is now settled by running
// them, not by asking a model what it thinks happened.
func TestVerifyEnvRunsTheCommandAndReportsTheVerdict(t *testing.T) {
	env := newTestVerifyEnv()

	ok, err := env.Act(context.Background(), environment.Action{
		CapabilityID: "software.verify.run_tests",
		Params:       map[string]any{"cmd": "exit 0"},
	})
	if err != nil {
		t.Fatalf("Act: %v", err)
	}
	if !ok.Success {
		t.Errorf("a passing command must settle the criterion, got error %q", ok.Error)
	}
	if ok.StateDelta["verifier"] != "software.verify.run_tests" {
		t.Errorf("result should name the verifier that ran, got %v", ok.StateDelta["verifier"])
	}

	bad, _ := env.Act(context.Background(), environment.Action{
		CapabilityID: "software.verify.lint",
		Params:       map[string]any{"cmd": "exit 1"},
	})
	if bad.Success {
		t.Error("a failing command must fail the criterion; that is the entire value of a deterministic verifier")
	}
}

// No default command. Guessing `go test ./...` would report a green suite on a
// repository that is not Go.
func TestVerifyEnvRefusesWithoutACommand(t *testing.T) {
	res, _ := newTestVerifyEnv().Act(context.Background(), environment.Action{
		CapabilityID: "software.verify.run_tests",
	})
	if res.Success {
		t.Fatal("a verifier with nothing to run must not report success")
	}
	if !strings.Contains(res.Error, "params.cmd") {
		t.Errorf("error should name the missing parameter, got %q", res.Error)
	}
}

func TestVerifyEnvRejectsForeignCapabilities(t *testing.T) {
	res, _ := newTestVerifyEnv().Act(context.Background(), environment.Action{
		CapabilityID: "software.act.write_code",
		Params:       map[string]any{"cmd": "exit 0"},
	})
	if res.Success {
		t.Error("verifyEnv serves two capabilities and must refuse the rest")
	}
}

// Routing: the two verification capabilities must resolve to this env, or the
// criteria fall back to a model again without anything saying so.
func TestVerifyCapabilitiesAreServed(t *testing.T) {
	for _, id := range []capability.CapabilityID{"software.verify.run_tests", "software.verify.lint"} {
		envID, ok := servedBy(id)
		if !ok {
			t.Errorf("%s is served by no environment", id)
			continue
		}
		if envID != "software.env.verify" {
			t.Errorf("%s routes to %s, want software.env.verify", id, envID)
		}
	}
}

// A criterion may not name a verifier no environment serves: it does not fail,
// it silently costs a judgement call per iteration while claiming otherwise.
func TestNoTemplateNamesAVerifierNothingServes(t *testing.T) {
	for _, tmpl := range New().ObjectiveTemplates() {
		for _, c := range tmpl.SuccessCriteria {
			if c.Verifier == "" || c.Domain != "" {
				continue // judged, or another pack's to resolve
			}
			if _, ok := servedBy(c.Verifier); !ok {
				t.Errorf("%s/%s names verifier %q, which no environment serves",
					tmpl.ID, c.ID, c.Verifier)
			}
		}
	}
}

// Delivery's review criteria are judgements and now say so.
func TestDeliveryReviewCriteriaAreJudged(t *testing.T) {
	var delivery objective.Template
	for _, tmpl := range New().ObjectiveTemplates() {
		if tmpl.ID == "software.objective.delivery" {
			delivery = tmpl
		}
	}
	for _, c := range delivery.SuccessCriteria {
		if c.ID == "peer-review" || c.ID == "lead-review" {
			if c.Verifier != "" {
				t.Errorf("%s still names verifier %q; nothing serves it", c.ID, c.Verifier)
			}
		}
	}
}

// ── the git observation window ───────────────────────────────────────────────

type windowVC struct {
	sinceCommits time.Time
	sincePRs     time.Time
}

func (w *windowVC) Name() string { return "window_vc" }
func (w *windowVC) Active() bool { return true }
func (w *windowVC) CreatePR(_ context.Context, _ versioncontrol.PullRequest) (string, error) {
	return "", nil
}
func (w *windowVC) ListPRs(_ context.Context, _ string, since time.Time) ([]versioncontrol.PRSummary, error) {
	w.sincePRs = since
	return nil, nil
}
func (w *windowVC) GetCommits(_ context.Context, _ string, since time.Time) ([]versioncontrol.Commit, error) {
	w.sinceCommits = since
	return nil, nil
}

// Every byte here is re-serialised into the reason prompt of every iteration of
// every loop watching the repository. An unfiltered read pulled the adapter's
// whole first page each time.
func TestGitObservationIsBounded(t *testing.T) {
	vc := &windowVC{}
	env := &gitEnv{id: EnvGit, vc: vc}

	if _, err := env.Observe(context.Background(), environment.ObservationQuery{}); err != nil {
		t.Fatalf("Observe: %v", err)
	}

	if vc.sinceCommits.IsZero() {
		t.Error("commits were fetched with no lower bound")
	}
	if vc.sincePRs.IsZero() {
		t.Error("pull requests were fetched with no lower bound")
	}
	age := time.Since(vc.sinceCommits)
	if age < gitObservationWindow-time.Minute || age > gitObservationWindow+time.Minute {
		t.Errorf("commit window is %v, want about %v", age, gitObservationWindow)
	}
}
