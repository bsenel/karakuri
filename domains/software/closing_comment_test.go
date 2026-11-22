package software

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/bsenel/karakuri/internal/platform/tools"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// Phase 40 step 13, discovery half: after a discovery proposal is closed
// (not merged), the next empty-pipeline cycle reads why.

type closedForge struct {
	commentForge
	closed    []versioncontrol.PRSummary
	closedErr error
}

func (f *closedForge) ListClosedPRs(context.Context, string, int) ([]versioncontrol.PRSummary, error) {
	if f.closedErr != nil {
		return nil, f.closedErr
	}
	return f.closed, nil
}

func closedPR(number int, branch string) versioncontrol.PRSummary {
	return versioncontrol.PRSummary{ID: fmt.Sprint(number), Number: number, HeadBranch: branch}
}

func closingNote(id int64, author, body string) versioncontrol.PullRequestComment {
	return versioncontrol.PullRequestComment{ID: id, Kind: "issue_comment", Author: author, Body: body, CreatedAt: rcLast}
}

func readClosing(t *testing.T, f *closedForge, posted postedCommentIDs) (string, error) {
	t.Helper()
	return newClosingComment(f, postedAuthors, posted)(context.Background())
}

func TestClosingCommentIsTheLastCommentOnTheNewestClosedDiscoveryProposal(t *testing.T) {
	f := &closedForge{closed: []versioncontrol.PRSummary{
		closedPR(12, "karakuri/discovery-2026-11-10"),
		closedPR(9, "karakuri/discovery-2026-11-01"),
	}}
	f.comments = map[int][]versioncontrol.PullRequestComment{
		12: {closingNote(1, "berat", "first thought"), closingNote(2, "berat", "too broad, narrow it to the CLI")},
		9:  {closingNote(3, "berat", "an older proposal's reason")},
	}
	got, err := readClosing(t, f, nil)
	if err != nil {
		t.Fatalf("closing comment: %v", err)
	}
	if got != "too broad, narrow it to the CLI" {
		t.Errorf("closing comment = %q, want the last comment of the newest closed discovery proposal", got)
	}
}

func TestClosingCommentIgnoresAClosedPullRequestThatIsNotADiscoveryProposal(t *testing.T) {
	f := &closedForge{closed: []versioncontrol.PRSummary{
		closedPR(14, "karakuri/phase-40-step-13-x"),
		closedPR(9, "karakuri/discovery-2026-11-01"),
	}}
	f.comments = map[int][]versioncontrol.PullRequestComment{
		14: {closingNote(1, "berat", "wrong stream")},
		9:  {closingNote(2, "berat", "not what we need")},
	}
	got, err := readClosing(t, f, nil)
	if err != nil {
		t.Fatalf("closing comment: %v", err)
	}
	if got != "not what we need" {
		t.Errorf("closing comment = %q, want the discovery proposal's, not the delivery PR's", got)
	}
}

func TestClosingCommentSkipsACommentKarakuriRecordedPosting(t *testing.T) {
	f := &closedForge{closed: []versioncontrol.PRSummary{closedPR(12, "karakuri/discovery-2026-11-10")}}
	f.comments = map[int][]versioncontrol.PullRequestComment{
		12: {closingNote(1, "shared-acct", "owner reason"), closingNote(2, "shared-acct", "Karakuri: noted, closing the loop")},
	}
	recorded := func(context.Context) (map[int64]bool, error) { return map[int64]bool{2: true}, nil }
	got, err := readClosing(t, f, recorded)
	if err != nil {
		t.Fatalf("closing comment: %v", err)
	}
	if got != "owner reason" {
		t.Errorf("closing comment = %q, want the earlier owner comment", got)
	}

	// No record: every comment counts.
	got, err = readClosing(t, f, nil)
	if err != nil {
		t.Fatalf("closing comment without a record: %v", err)
	}
	if !strings.Contains(got, "noted, closing the loop") {
		t.Errorf("closing comment = %q, want the last comment when nothing is recorded", got)
	}
}

func TestClosingCommentIsEmptyWithNoClosedDiscoveryProposalOrNoComment(t *testing.T) {
	cases := map[string]*closedForge{
		"none closed": {},
		"closed with no comment": {
			closed: []versioncontrol.PRSummary{closedPR(12, "karakuri/discovery-2026-11-10")},
		},
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := readClosing(t, f, nil)
			if err != nil {
				t.Fatalf("closing comment: %v", err)
			}
			if got != "" {
				t.Errorf("closing comment = %q, want empty", got)
			}
		})
	}
}

// A forge that cannot be read is an error, never "no comment" (rule 10).
func TestClosingCommentIsAnErrorWhenTheForgeCannotBeRead(t *testing.T) {
	boom := errors.New("forge down")
	cases := map[string]*closedForge{
		"closed pull requests fail": {closedErr: boom},
		"unsupported by the forge":  {closedErr: versioncontrol.ErrUnsupported},
		"comments fail": func() *closedForge {
			f := &closedForge{closed: []versioncontrol.PRSummary{closedPR(12, "karakuri/discovery-2026-11-10")}}
			f.commentsErr = boom
			return f
		}(),
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := readClosing(t, f, nil)
			if err == nil {
				t.Fatalf("closing comment = %q with no error, want an error", got)
			}
		})
	}
}

// The registered discovery trigger reads the closing comment through the pack.
func TestRegisteredDiscoveryTriggerCarriesTheClosingCommentAsThirdParty(t *testing.T) {
	f := &closedForge{closed: []versioncontrol.PRSummary{closedPR(12, "karakuri/discovery-2026-11-10")}}
	f.comments = map[int][]versioncontrol.PullRequestComment{
		12: {closingNote(1, "berat", "too broad, narrow it to the CLI")},
	}
	forge := &workForge{commentForge: &f.commentForge, roadmap: roadmapNothingPlanned}
	// workForge answers FileAt; the closed list comes from closedForge.
	both := &registeredClosedForge{workForge: forge, closed: f}

	s := newPostedStore(t)
	env := buildRegisteredDiscoveryTrigger(t, s, both)
	obs, err := env.Observe(context.Background(), environment.ObservationQuery{})
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if got, _ := obs.State["closing_comment"].(string); !strings.Contains(got, "too broad") {
		t.Errorf("closing_comment = %v, want the closed proposal's comment (state %v)", obs.State["closing_comment"], obs.State)
	}
	if obs.Trust != environment.TrustThirdParty {
		t.Errorf("Trust = %q, want TrustThirdParty", obs.Trust)
	}
}

type registeredClosedForge struct {
	*workForge
	closed *closedForge
}

func (r *registeredClosedForge) ListClosedPRs(ctx context.Context, repo string, limit int) ([]versioncontrol.PRSummary, error) {
	return r.closed.ListClosedPRs(ctx, repo, limit)
}

func buildRegisteredDiscoveryTrigger(t *testing.T, s storage.StorageAdapter, forge versioncontrol.VersionControlAdapter) environment.Environment {
	t.Helper()
	reg := tools.NewRegistry()
	reg.VC.Set("forge", "stub", forge)
	pack := New().WithOwnAuthors(postedAuthors)
	pack.tools = reg
	pack.WithStorage(s)
	for _, fac := range pack.EnvironmentFactories() {
		if fac.EnvID != "software.env.discovery_trigger" {
			continue
		}
		env, err := fac.Build(environment.BuildContext{AdapterBindings: map[string]string{"versioncontrol": "forge"}})
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		return env
	}
	t.Fatal("no discovery_trigger factory")
	return nil
}
