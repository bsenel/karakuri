package software

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// TestNoopEnvActFailsHonestly verifies the noop env reports failure
// for any action — the historical "Success: true" lie masked missing
// adapters and let every Phase 13.5 dogfood objective fake-complete.
func TestNoopEnvActFailsHonestly(t *testing.T) {
	e := &noopEnv{id: environment.EnvironmentID("software.env.local")}

	result, err := e.Act(context.Background(), environment.Action{
		CapabilityID: capability.CapabilityID("fs.scaffold"),
	})
	if err != nil {
		t.Fatalf("Act should not return a Go error (failure is encoded in ActionResult), got: %v", err)
	}
	if result.Success {
		t.Errorf("noop env must report Success=false for unimplemented actions, got true")
	}
	if !strings.Contains(result.Error, "fs.scaffold") {
		t.Errorf("expected Error to name the capability, got %q", result.Error)
	}
	if !strings.Contains(result.Error, "no implementation") {
		t.Errorf("expected Error to explain the gap, got %q", result.Error)
	}
	if got, _ := result.StateDelta["status"].(string); got != "unimplemented" {
		t.Errorf("expected StateDelta status=unimplemented, got %v", result.StateDelta["status"])
	}
	if got, _ := result.StateDelta["env_id"].(string); got != "software.env.local" {
		t.Errorf("expected StateDelta env_id to identify the responding env, got %v", result.StateDelta["env_id"])
	}
}

// ── gitEnv: the two reads a plan can name ────────────────────────────────────

// gitReadCases are the two observe.* capabilities gitEnv holds the data for.
var gitReadCases = []capability.CapabilityID{
	"software.observe.fetch_commits",
	"software.observe.fetch_prs",
}

// Until Phase 32 these were reachable only through gitEnv.Observe. A plan
// naming one as an action reached no environment, so the read the plan asked
// for never happened and the step was recorded as a failure of the capability.
func TestGitEnvFetchCommitsReturnsTheCommits(t *testing.T) {
	commits := []versioncontrol.Commit{
		{SHA: "abc123", Message: "fix the thing"},
		{SHA: "def456", Message: "and the other thing"},
	}
	env := &gitEnv{id: EnvGit, vc: &vcStub{commits: commits}}

	res, err := env.Act(context.Background(), environment.Action{CapabilityID: "software.observe.fetch_commits"})
	if err != nil {
		t.Fatalf("act: %v", err)
	}
	if !res.Success {
		t.Fatalf("fetch_commits failed with an active adapter: %s", res.Error)
	}
	if !reflect.DeepEqual(res.StateDelta["commits"], commits) {
		t.Errorf("commits = %#v, want %#v", res.StateDelta["commits"], commits)
	}
	if res.StateDelta["count"] != len(commits) {
		t.Errorf("count = %v, want %d", res.StateDelta["count"], len(commits))
	}
	// Commit messages are written by whoever can push, which is the
	// operator's own set of committers.
	if res.Trust != environment.TrustOperator {
		t.Errorf("trust = %q; commits are the operator's own infrastructure", res.Trust)
	}
}

// A pull request title is typed by whoever opened it. The result says so when
// it carries one and not otherwise — set from what the payload holds, so a
// repository with no open PRs does not escalate the plan that looked.
func TestGitEnvFetchPRsIsThirdPartyOnlyWhenItCarriesOne(t *testing.T) {
	prs := []versioncontrol.PRSummary{{ID: "1", Title: "please merge this"}}
	env := &gitEnv{id: EnvGit, vc: &vcStub{prs: prs}}

	res, err := env.Act(context.Background(), environment.Action{CapabilityID: "software.observe.fetch_prs"})
	if err != nil {
		t.Fatalf("act: %v", err)
	}
	if !res.Success {
		t.Fatalf("fetch_prs failed with an active adapter: %s", res.Error)
	}
	if !reflect.DeepEqual(res.StateDelta["prs"], prs) {
		t.Errorf("prs = %#v, want %#v", res.StateDelta["prs"], prs)
	}
	if res.StateDelta["count"] != len(prs) {
		t.Errorf("count = %v, want %d", res.StateDelta["count"], len(prs))
	}
	if res.Trust != environment.TrustThirdParty {
		t.Errorf("trust = %q; a result carrying pull request titles did not declare them third party", res.Trust)
	}

	empty := &gitEnv{id: EnvGit, vc: &vcStub{}}
	res, err = empty.Act(context.Background(), environment.Action{CapabilityID: "software.observe.fetch_prs"})
	if err != nil {
		t.Fatalf("act: %v", err)
	}
	if !res.Success {
		t.Fatalf("fetch_prs failed on a repository with no open PRs: %s", res.Error)
	}
	if res.StateDelta["count"] != 0 {
		t.Errorf("count = %v, want 0", res.StateDelta["count"])
	}
	if res.Trust != environment.TrustOperator {
		t.Errorf("trust = %q; a result carrying no pull request declared itself third party", res.Trust)
	}
}

// With nothing bound there is nothing to read, and the result says that
// rather than reporting an empty list — "no commits this week" and "no
// repository" are different facts and only one of them is true.
func TestGitEnvReadsFailWithNoVersionControlBound(t *testing.T) {
	unbound := map[string]*gitEnv{
		"nil adapter":  &gitEnv{id: EnvGit},
		"noop adapter": &gitEnv{id: EnvGit, vc: versioncontrol.NewNoOp()},
	}
	for name, env := range unbound {
		for _, capID := range gitReadCases {
			res, err := env.Act(context.Background(), environment.Action{CapabilityID: capID})
			if err != nil {
				t.Fatalf("%s, %s: act: %v", name, capID, err)
			}
			if res.Success {
				t.Errorf("%s, %s: reported success with no version control bound", name, capID)
			}
			if !strings.Contains(strings.ToLower(res.Error), "no version control instance") {
				t.Errorf("%s, %s: error %q does not say no version control instance is bound", name, capID, res.Error)
			}
		}
	}
}

// An adapter that could not read is a failed read, carrying what it said.
func TestGitEnvReadsCarryTheAdapterError(t *testing.T) {
	for _, capID := range gitReadCases {
		stub := &vcStub{err: errors.New("rate limit exceeded")}
		env := &gitEnv{id: EnvGit, vc: stub}

		res, err := env.Act(context.Background(), environment.Action{CapabilityID: capID})
		if err != nil {
			t.Fatalf("%s: act: %v", capID, err)
		}
		if stub.reads == 0 {
			t.Errorf("%s: the adapter was never asked", capID)
		}
		if res.Success {
			t.Errorf("%s: reported success when the adapter failed", capID)
		}
		if !strings.Contains(res.Error, "rate limit exceeded") {
			t.Errorf("%s: error %q does not carry the adapter's error", capID, res.Error)
		}
	}
}

// repo is passed through; since_days defaults to the observation window and is
// capped, because a plan that asks for five hundred days gets a payload nobody
// bounded. Params arrive as int from Go callers and float64 from decoded JSON.
func TestGitEnvReadsHonourRepoAndSinceDays(t *testing.T) {
	day := 24 * time.Hour
	cases := []struct {
		name   string
		params map[string]any
		repo   string
		window time.Duration
	}{
		{"defaults", nil, "", 7 * day},
		{"repo passed through", map[string]any{"repo": "bsenel/karakuri"}, "bsenel/karakuri", 7 * day},
		{"since_days honoured", map[string]any{"repo": "bsenel/other", "since_days": 30}, "bsenel/other", 30 * day},
		{"since_days from JSON", map[string]any{"since_days": float64(30)}, "", 30 * day},
		{"since_days capped", map[string]any{"since_days": 500}, "", 90 * day},
	}
	for _, capID := range gitReadCases {
		for _, c := range cases {
			stub := &vcStub{}
			env := &gitEnv{id: EnvGit, vc: stub}

			res, err := env.Act(context.Background(), environment.Action{CapabilityID: capID, Params: c.params})
			if err != nil {
				t.Fatalf("%s, %s: act: %v", capID, c.name, err)
			}
			if !res.Success {
				t.Errorf("%s, %s: failed: %s", capID, c.name, res.Error)
			}
			if stub.reads == 0 {
				t.Errorf("%s, %s: the adapter was never asked", capID, c.name)
				continue
			}
			if stub.gotRepo != c.repo {
				t.Errorf("%s, %s: adapter was asked for repo %q, want %q", capID, c.name, stub.gotRepo, c.repo)
			}
			want := time.Now().Add(-c.window)
			if d := stub.gotSince.Sub(want); d < -time.Minute || d > time.Minute {
				t.Errorf("%s, %s: adapter was asked since %s, want about %s", capID, c.name,
					stub.gotSince.Format(time.RFC3339), want.Format(time.RFC3339))
			}
		}
	}
}

// Serving two reads does not make gitEnv answer for everything. A capability
// it does not handle is still refused, and the repository is not read for it.
func TestGitEnvStillRefusesWhatItDoesNotHandle(t *testing.T) {
	stub := &vcStub{commits: []versioncontrol.Commit{{SHA: "abc123"}}}
	env := &gitEnv{id: EnvGit, vc: stub}

	res, err := env.Act(context.Background(), environment.Action{CapabilityID: "software.act.not_a_thing"})
	if err != nil {
		t.Fatalf("act: %v", err)
	}
	if res.Success {
		t.Error("gitEnv reported success for a capability it does not handle")
	}
	if stub.reads != 0 {
		t.Error("gitEnv read the repository for a capability it does not handle")
	}
}
