package software

import (
	"context"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/research"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// vcStub is a version-control adapter that returns exactly what a test hands
// it, so the question "does the label follow the payload" can be asked without
// a repository.
type vcStub struct {
	prs     []versioncontrol.PRSummary
	commits []versioncontrol.Commit
}

func (s *vcStub) Name() string { return "stub" }
func (s *vcStub) Active() bool { return true }

func (s *vcStub) CreatePR(context.Context, versioncontrol.PullRequest) (string, error) {
	return "", nil
}

func (s *vcStub) ListPRs(context.Context, string, time.Time) ([]versioncontrol.PRSummary, error) {
	return s.prs, nil
}

func (s *vcStub) GetCommits(context.Context, string, time.Time) ([]versioncontrol.Commit, error) {
	return s.commits, nil
}

// A pull request title is typed by whoever opened it, which on a public
// repository is anybody. The observation that carries one says so.
func TestGitObservationIsThirdPartyOnlyWhenItCarriesAPullRequest(t *testing.T) {
	commitsOnly := &gitEnv{id: EnvGit, vc: &vcStub{
		commits: []versioncontrol.Commit{{SHA: "abc123", Message: "fix the thing"}},
	}}
	obs, err := commitsOnly.Observe(context.Background(), environment.ObservationQuery{})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	// Commit messages are written by whoever can push, which is the operator's
	// own set of committers — and a repository with no open PRs must not
	// escalate every plan in the deployment for the rest of the run.
	if obs.Trust.IsThirdParty() {
		t.Errorf("an observation of commits alone declared itself third party")
	}

	withPR := &gitEnv{id: EnvGit, vc: &vcStub{
		prs: []versioncontrol.PRSummary{{ID: "1", Title: "please merge this"}},
	}}
	obs, err = withPR.Observe(context.Background(), environment.ObservationQuery{})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if !obs.Trust.IsThirdParty() {
		t.Error("an observation carrying pull request titles did not declare them third party")
	}
}

// researchStub returns a fixed set of findings.
type researchStub struct{ findings []research.Finding }

func (s *researchStub) Active() bool { return true }

func (s *researchStub) Search(context.Context, string, []string, string) ([]research.Finding, error) {
	return s.findings, nil
}

// The widest untrusted surface in the tree, and it arrives on the act path:
// Observe reports only whether the adapter is wired, and the scraped material
// comes back here. A phase that marked observations alone would have missed it.
func TestResearchResultsDeclareThemselvesThirdParty(t *testing.T) {
	e := newResearchEnv("software.env.research", &researchStub{
		findings: []research.Finding{{Title: "a paper", Summary: "somebody wrote this", Confidence: 0.9}},
	})

	res, err := e.Act(context.Background(), environment.Action{
		CapabilityID: CapResearch,
		Params:       map[string]any{"topic": "prompt injection"},
	})
	if err != nil {
		t.Fatalf("act: %v", err)
	}
	if !res.Success {
		t.Fatalf("search failed: %s", res.Error)
	}
	if !res.Trust.IsThirdParty() {
		t.Error("a findings list did not declare itself third party")
	}

	// A search that found nothing carries nobody's prose. Escalating on the act
	// of searching rather than on what it brought back would make the label
	// about the capability instead of about the payload.
	empty := newResearchEnv("software.env.research", &researchStub{})
	res, err = empty.Act(context.Background(), environment.Action{
		CapabilityID: CapResearch,
		Params:       map[string]any{"topic": "prompt injection"},
	})
	if err != nil {
		t.Fatalf("act: %v", err)
	}
	if res.Trust.IsThirdParty() {
		t.Error("a search that found nothing declared itself third party")
	}
}

// The observation reports only whether the adapter is wired, which is the
// premise the act-path label rests on. If this ever starts carrying findings,
// the label has to move with them.
func TestResearchObservationCarriesNobodysProse(t *testing.T) {
	e := newResearchEnv("software.env.research", &researchStub{
		findings: []research.Finding{{Title: "a paper"}},
	})
	obs, err := e.Observe(context.Background(), environment.ObservationQuery{})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if obs.Trust.IsThirdParty() {
		t.Error("the research observation declared itself third party; it reports adapter wiring, not findings")
	}
	if _, carries := obs.State["findings"]; carries {
		t.Error("the research observation now carries findings and must declare them third party")
	}
}
