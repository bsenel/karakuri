package software

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/bsenel/karakuri/config"
	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/storage"
	"github.com/bsenel/karakuri/internal/platform/tools"
	"github.com/bsenel/karakuri/internal/platform/tools/cliagent"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// Phase 40 step 12a, the recording side. A delegation on a branch lists the
// branch's pull request comments before and after the run and puts the new
// ones Karakuri authored in its result as posted_comment_ids; a read that
// fails puts posted_comment_ids_error there instead of an empty list. The
// reader over the audit log collects those IDs, and fails on an error row so
// the observation goes blind rather than take Karakuri's comment for the owner's.

const (
	postedKey    = "posted_comment_ids"
	postedErrKey = "posted_comment_ids_error"
	postedBranch = "karakuri/phase-40-step-12a"
)

var postedAuthors = config.OwnAuthorsConfig{
	Karakuri: []string{"karakuri-bot", "shared-acct"},
	Owners:   []string{"berat", "shared-acct"},
}

// scriptedForge serves its pull requests and comments by read number: the
// first ListPRs and the first ListPullRequestComments are the "before" reads,
// the second the "after" ones. Reads past the script repeat its last entry.
type scriptedForge struct {
	commentForge
	prsByRead      [][]versioncontrol.PRSummary
	commentsByRead [][]versioncontrol.PullRequestComment
	// failPRsAt and failCommentsAt are read numbers (1-based) that fail.
	failPRsAt, failCommentsAt int
	prReads, commentReads     int
}

func (f *scriptedForge) ListPRs(context.Context, string, time.Time) ([]versioncontrol.PRSummary, error) {
	f.prReads++
	if f.prReads == f.failPRsAt {
		return nil, errors.New("forge down")
	}
	if len(f.prsByRead) == 0 {
		return nil, nil
	}
	return f.prsByRead[min(f.prReads, len(f.prsByRead))-1], nil
}

func (f *scriptedForge) ListPullRequestComments(context.Context, string, int) ([]versioncontrol.PullRequestComment, error) {
	f.commentReads++
	if f.commentReads == f.failCommentsAt {
		return nil, errors.New("comments unreadable")
	}
	if len(f.commentsByRead) == 0 {
		return nil, nil
	}
	return f.commentsByRead[min(f.commentReads, len(f.commentsByRead))-1], nil
}

func (f *scriptedForge) Active() bool { return true }

func postedPR(branch string) []versioncontrol.PRSummary {
	return []versioncontrol.PRSummary{{ID: "7", Number: 7, HeadBranch: branch, Author: "karakuri-bot", UpdatedAt: time.Now()}}
}

func pc(id int64, author string) versioncontrol.PullRequestComment {
	return rcComment(id, author, time.Now())
}

func runPostedDelegation(t *testing.T, capName string, forge versioncontrol.VersionControlAdapter, branch string) environment.ActionResult {
	t.Helper()
	cli := &recordingCLI{}
	env := &cliEnv{id: "software.env.cli_agent", cli: cli, twinID: "twin-1", vc: forge, ownAuthors: postedAuthors, allowedTools: []string{"Edit"}}
	params := map[string]any{"prompt": "answer the review", "worktree_path": newSalvageFixture(t).worktree}
	if branch != "" {
		params["branch"] = branch
	}
	res, err := env.Act(context.Background(), environment.Action{CapabilityID: capability.CapabilityID(capName), Params: params})
	if err != nil {
		t.Fatalf("act: %v", err)
	}
	if len(cli.calls) != 1 {
		t.Fatalf("delegation ran %d times, want 1 (result: %+v)", len(cli.calls), res)
	}
	return res
}

func postedIDs(t *testing.T, res environment.ActionResult) []int64 {
	t.Helper()
	raw, ok := res.StateDelta[postedKey]
	if !ok {
		t.Fatalf("no %s in the result; keys: %v", postedKey, stateKeys(res.StateDelta))
	}
	var got []int64
	switch v := raw.(type) {
	case []int64:
		got = v
	case []any:
		for _, x := range v {
			switch n := x.(type) {
			case int64:
				got = append(got, n)
			case int:
				got = append(got, int64(n))
			case float64:
				got = append(got, int64(n))
			default:
				t.Fatalf("id of type %T", x)
			}
		}
	default:
		t.Fatalf("%s is a %T, want a list", postedKey, raw)
	}
	slices.Sort(got)
	return got
}

func TestRecordPostedComments_IDsAreTheNewOnesKarakuriAuthored(t *testing.T) {
	before := []versioncontrol.PullRequestComment{pc(1, "berat"), pc(2, "karakuri-bot")}
	after := append(slices.Clone(before),
		pc(3, "karakuri-bot"), // new, Karakuri's
		pc(4, "shared-acct"),  // new, shared account listed under own_authors.karakuri
		pc(5, "berat"),        // new, the owner's: not recorded
		pc(6, "stranger"),     // new, a third party's: not recorded
	)
	for _, capability := range []string{"software.act.write_code", "software.act.write_test", "software.act.delegate_to_cli"} {
		t.Run(capability, func(t *testing.T) {
			forge := &scriptedForge{
				prsByRead:      [][]versioncontrol.PRSummary{postedPR(postedBranch), postedPR(postedBranch)},
				commentsByRead: [][]versioncontrol.PullRequestComment{before, after},
			}
			res := runPostedDelegation(t, capability, forge, postedBranch)
			if got := postedIDs(t, res); !reflect.DeepEqual(got, []int64{3, 4}) {
				t.Errorf("posted ids = %v, want [3 4]", got)
			}
			if _, bad := res.StateDelta[postedErrKey]; bad {
				t.Errorf("unexpected %s: %v", postedErrKey, res.StateDelta[postedErrKey])
			}
		})
	}
}

func TestRecordPostedComments_PullRequestOpenedDuringTheRun(t *testing.T) {
	forge := &scriptedForge{
		prsByRead:      [][]versioncontrol.PRSummary{nil, postedPR(postedBranch)},
		commentsByRead: [][]versioncontrol.PullRequestComment{{pc(10, "karakuri-bot"), pc(11, "berat")}},
	}
	res := runPostedDelegation(t, "software.act.write_code", forge, postedBranch)
	if got := postedIDs(t, res); !reflect.DeepEqual(got, []int64{10}) {
		t.Errorf("posted ids = %v, want [10]", got)
	}
}

func TestRecordPostedComments_NoNewCommentIsAnEmptyListNotAnError(t *testing.T) {
	same := []versioncontrol.PullRequestComment{pc(1, "karakuri-bot")}
	forge := &scriptedForge{
		prsByRead:      [][]versioncontrol.PRSummary{postedPR(postedBranch)},
		commentsByRead: [][]versioncontrol.PullRequestComment{same, same},
	}
	res := runPostedDelegation(t, "software.act.write_code", forge, postedBranch)
	if got := postedIDs(t, res); len(got) != 0 {
		t.Errorf("posted ids = %v, want none", got)
	}
	if _, bad := res.StateDelta[postedErrKey]; bad {
		t.Errorf("a read that succeeded recorded %s", postedErrKey)
	}
}

func TestRecordPostedComments_AFailedReadIsAnErrorNeverAnEmptyList(t *testing.T) {
	prs := [][]versioncontrol.PRSummary{postedPR(postedBranch), postedPR(postedBranch)}
	comments := [][]versioncontrol.PullRequestComment{nil, {pc(3, "karakuri-bot")}}
	cases := map[string]*scriptedForge{
		"listing before":  {prsByRead: prs, commentsByRead: comments, failPRsAt: 1},
		"comments before": {prsByRead: prs, commentsByRead: comments, failCommentsAt: 1},
		"listing after":   {prsByRead: prs, commentsByRead: comments, failPRsAt: 2},
		"comments after":  {prsByRead: prs, commentsByRead: comments, failCommentsAt: 2},
	}
	for name, forge := range cases {
		t.Run(name, func(t *testing.T) {
			res := runPostedDelegation(t, "software.act.write_code", forge, postedBranch)
			msg, ok := res.StateDelta[postedErrKey].(string)
			if !ok || msg == "" {
				t.Fatalf("no %s; keys: %v", postedErrKey, stateKeys(res.StateDelta))
			}
			if _, bad := res.StateDelta[postedKey]; bad {
				t.Errorf("%s was left beside the error: %v", postedKey, res.StateDelta[postedKey])
			}
			if !res.Success {
				t.Errorf("the action failed for an unreadable forge: %s", res.Error)
			}
		})
	}
}

func TestRecordPostedComments_NothingWithoutAnAdapterOrAPullRequest(t *testing.T) {
	cases := map[string]struct {
		forge  versioncontrol.VersionControlAdapter
		branch string
	}{
		"no adapter bound":            {nil, postedBranch},
		"no-op adapter":               {versioncontrol.NewNoOp(), postedBranch},
		"no pull request":             {&scriptedForge{}, postedBranch},
		"pull request on another one": {&scriptedForge{prsByRead: [][]versioncontrol.PRSummary{postedPR("other")}}, postedBranch},
		"no branch param":             {&scriptedForge{prsByRead: [][]versioncontrol.PRSummary{postedPR(postedBranch)}}, ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			res := runPostedDelegation(t, "software.act.write_code", c.forge, c.branch)
			for _, k := range []string{postedKey, postedErrKey} {
				if v, ok := res.StateDelta[k]; ok {
					t.Errorf("%s recorded as %v", k, v)
				}
			}
		})
	}
}

// ── the reader over the audit log ────────────────────────────────────────────

func newPostedStore(t *testing.T) *storage.GORMStorage {
	t.Helper()
	db, err := platformdb.Open("sqlite", filepath.Join(t.TempDir(), "posted.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := platformdb.RunMigrations(db, ""); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return storage.NewGORMStorage(db)
}

var postedEventSeq int

// saveActionEvent saves the row internal/feature/loop/act.go saves: a payload
// of params, result and routed_by.
func saveActionEvent(t *testing.T, s storage.StorageAdapter, capability string, delta map[string]any) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"params":    map[string]any{"branch": postedBranch},
		"result":    environment.ActionResult{Success: true, StateDelta: delta},
		"routed_by": "pack",
	})
	if err != nil {
		t.Fatal(err)
	}
	postedEventSeq++
	err = s.SaveToolEvent(context.Background(), storage.ToolEvent{
		ID: fmt.Sprintf("te-posted-%d", postedEventSeq), ObjectiveID: "obj-1", Capability: capability,
		Success: true, Kind: storage.ToolEventExecute, PayloadJSON: string(payload),
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
}

func TestPostedCommentReader_UnionsIDsAcrossTheThreeCapabilities(t *testing.T) {
	s := newPostedStore(t)
	saveActionEvent(t, s, "software.act.write_code", map[string]any{postedKey: []int64{1, 2}})
	saveActionEvent(t, s, "software.act.write_test", map[string]any{postedKey: []int64{2, 3}})
	saveActionEvent(t, s, "software.act.delegate_to_cli", map[string]any{postedKey: []int64{4}})
	// Noise the reader must not take IDs from, or fail on.
	saveActionEvent(t, s, "software.act.write_code", map[string]any{"summary": "nothing posted"})
	saveActionEvent(t, s, "software.act.create_pr", map[string]any{postedKey: []int64{99}})

	got, err := newPostedCommentReader(s)(context.Background())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if want := map[int64]bool{1: true, 2: true, 3: true, 4: true}; !reflect.DeepEqual(got, want) {
		t.Errorf("ids = %v, want %v", got, want)
	}
}

func TestPostedCommentReader_NothingRecordedIsAnEmptyRecord(t *testing.T) {
	got, err := newPostedCommentReader(newPostedStore(t))(context.Background())
	if err != nil {
		t.Fatalf("an audit log with no such events is a record of none, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ids = %v, want none", got)
	}
}

func TestPostedCommentReader_AnErrorRowMakesItFail(t *testing.T) {
	for _, capability := range []string{"software.act.write_code", "software.act.write_test", "software.act.delegate_to_cli"} {
		t.Run(capability, func(t *testing.T) {
			s := newPostedStore(t)
			saveActionEvent(t, s, "software.act.write_code", map[string]any{postedKey: []int64{1}})
			saveActionEvent(t, s, capability, map[string]any{postedErrKey: "forge down"})

			got, err := newPostedCommentReader(s)(context.Background())
			if err == nil {
				t.Fatalf("read succeeded with %v beside an unrecorded run", got)
			}
		})
	}
}

// A shared-account comment Karakuri recorded is skipped end to end: the pack,
// given a storage adapter, builds a git environment that passes a reader, and
// the observation lists the owner's comment but not Karakuri's own.
func TestPackWithStorage_SharedAccountCommentRecordedIsSkipped(t *testing.T) {
	s := newPostedStore(t)
	saveActionEvent(t, s, "software.act.write_code", map[string]any{postedKey: []int64{402}})

	f := rcForge(rcBranch,
		rcComment(401, "shared-acct", rcLast.Add(time.Hour)),
		rcComment(402, "shared-acct", rcLast.Add(time.Hour)),
	)
	reg := tools.NewRegistry()
	reg.VC.Set("forge", "stub", versioncontrol.VersionControlAdapter(&activeCommentForge{f}))

	for name, pack := range map[string]*Pack{
		"with storage":    New().WithStorage(s),
		"without storage": New(),
	} {
		t.Run(name, func(t *testing.T) {
			pack.tools = reg
			pack.WithOwnAuthors(postedAuthors)
			env := buildGitEnv(t, pack)
			if (env.posted != nil) != (name == "with storage") {
				t.Fatalf("reader passed = %v", env.posted != nil)
			}
			obs, err := env.Observe(context.Background(), environment.ObservationQuery{})
			if err != nil {
				t.Fatalf("observe: %v", err)
			}
			if name == "without storage" {
				requireBlind(t, obs.State)
				return
			}
			got := rcListed(t, obs.State)
			if len(got) != 1 || got[0].CommentID != 401 || got[0].From != "owner" {
				t.Errorf("want only the owner's comment 401, got %+v", got)
			}
			if _, bad := obs.State["unanswered_review_comments_error"]; bad {
				t.Errorf("blind: %v", obs.State["unanswered_review_comments_error"])
			}
		})
	}
}

// activeCommentForge makes the test forge report itself active, which the git
// environment requires before it observes.
type activeCommentForge struct{ *commentForge }

func (activeCommentForge) Active() bool { return true }

func buildGitEnv(t *testing.T, p *Pack) *gitEnv {
	t.Helper()
	for _, fac := range p.EnvironmentFactories() {
		if fac.EnvID != EnvGit {
			continue
		}
		env, err := fac.Build(environment.BuildContext{AdapterBindings: map[string]string{"versioncontrol": "forge"}})
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		git, ok := env.(*gitEnv)
		if !ok {
			t.Fatalf("git environment is a %T", env)
		}
		return git
	}
	t.Fatal("no git environment factory")
	return nil
}

var _ cliagent.CLIAgentAdapter = (*recordingCLI)(nil)
