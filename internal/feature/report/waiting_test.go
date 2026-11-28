package report

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/digest"
	platformdb "github.com/bsenel/karakuri/internal/platform/db"
	"github.com/bsenel/karakuri/internal/platform/storage"
	karakuriquota "github.com/bsenel/karakuri/internal/quota"
)

// fakePulls is a PullRequestReader that returns what it was given.
type fakePulls struct {
	prs []StreamPullRequest
	err error
}

func (f *fakePulls) StreamPullRequests(_ context.Context, _ string) ([]StreamPullRequest, error) {
	return f.prs, f.err
}

// newServiceWithPulls is newService with a pull request reader injected (nil
// for none).
func newServiceWithPulls(t *testing.T, r PullRequestReader) (*Service, storage.StorageAdapter, time.Time) {
	t.Helper()
	db, err := platformdb.Open("sqlite", filepath.Join(t.TempDir(), "report.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := platformdb.RunMigrations(db, ""); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := storage.NewGORMStorage(db)
	clock := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	var opts []Option
	if r != nil {
		opts = append(opts, WithPullRequestReader(r))
	}
	svc := NewService(store, nil, nil, karakuriquota.Deps{}, Config{}, opts...)
	svc.now = func() time.Time { return clock }
	return svc, store, clock
}

func assembleDay(t *testing.T, svc *Service, until time.Time) digest.Digest {
	t.Helper()
	d, err := svc.Assemble(context.Background(), "twin-1", until.Add(-24*time.Hour), until)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	return d
}

func waitingNumbers(w []digest.WaitingPullRequest) []int {
	out := make([]int, len(w))
	for i, p := range w {
		out[i] = p.Number
	}
	return out
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Phase 40 step 14: the digest lists, per stream, the open pull requests
// waiting for a merge with the latest review's result, ordered by number.
func TestDigestListsPullRequestsWaitingForAMergeWithTheReview(t *testing.T) {
	reader := &fakePulls{prs: []StreamPullRequest{
		// Deliberately out of order: the digest orders by number.
		{ObjectiveID: "obj-a", Number: 240, Title: "feat(web): view", URL: "https://forge/pr/240", State: "open", Review: "changes_requested"},
		{ObjectiveID: "obj-a", Number: 231, Title: "feat(report): oversight", URL: "https://forge/pr/231", State: "open", Review: "approved", ReviewedHead: true},
		{ObjectiveID: "obj-a", Number: 236, Title: "fix(cli): flag", State: "open", Review: "none"},
		{ObjectiveID: "obj-a", Number: 238, Title: "docs: notes", State: "open", Review: "commented", ReviewedHead: true},
		// Another stream's pull request is not this stream's.
		{ObjectiveID: "obj-b", Number: 233, Title: "feat(ux): copy", State: "open", Review: "approved", ReviewedHead: true},
		// Not waiting for a merge: merged, closed, draft.
		{ObjectiveID: "obj-a", Number: 220, Title: "already merged", State: "merged", Review: "approved", ReviewedHead: true},
		{ObjectiveID: "obj-a", Number: 221, Title: "abandoned", State: "closed", Review: "none"},
		{ObjectiveID: "obj-a", Number: 239, Title: "wip", State: "open", Draft: true, Review: "none"},
	}}
	svc, store, until := newServiceWithPulls(t, reader)
	seedTwin(t, store, "twin-1", "CTO twin")
	seedStanding(t, store, "twin-1", "obj-a", "stream a")
	seedStanding(t, store, "twin-1", "obj-b", "stream b")
	seedStanding(t, store, "twin-1", "obj-c", "stream c, quiet")

	d := assembleDay(t, svc, until)
	sa, sb, sc := summaryOf(t, d, "obj-a"), summaryOf(t, d, "obj-b"), summaryOf(t, d, "obj-c")

	if got, want := waitingNumbers(sa.WaitingPullRequests), []int{231, 236, 238, 240}; !equalInts(got, want) {
		t.Fatalf("stream a waiting = %v, want %v (open, not draft, by number, none of b's)", got, want)
	}
	want := []digest.WaitingPullRequest{
		{Number: 231, Title: "feat(report): oversight", URL: "https://forge/pr/231", Review: "approved", ReviewedHead: true},
		{Number: 236, Title: "fix(cli): flag", Review: "none"},
		{Number: 238, Title: "docs: notes", Review: "commented", ReviewedHead: true},
		{Number: 240, Title: "feat(web): view", URL: "https://forge/pr/240", Review: "changes_requested"},
	}
	for i, w := range want {
		if sa.WaitingPullRequests[i] != w {
			t.Errorf("stream a waiting[%d] = %+v, want %+v", i, sa.WaitingPullRequests[i], w)
		}
	}
	if got := waitingNumbers(sb.WaitingPullRequests); !equalInts(got, []int{233}) {
		t.Errorf("stream b waiting = %v, want only 233", got)
	}
	if len(sc.WaitingPullRequests) != 0 {
		t.Errorf("quiet stream c waiting = %+v, want none", sc.WaitingPullRequests)
	}
	// A readable forge with nothing waiting is not blind.
	for _, s := range []digest.ObjectiveSummary{sa, sb, sc} {
		if b, ok := blockOf(s.Blocks, digest.BlockRemoteUnreadable); ok {
			t.Errorf("stream %s has block %+v though the pull requests were read", s.ID, b)
		}
	}

	// Regenerating the same window gives the same content.
	again := assembleDay(t, svc, until)
	first, _ := json.Marshal(d)
	second, _ := json.Marshal(again)
	if string(first) != string(second) {
		t.Errorf("assembling the same window twice differs:\n%s\n%s", first, second)
	}
}

// Blind, not quiet (AGENTS.md rule 10, ADR 026): pull requests that could not
// be read are a block on the stream, not an empty list that reads as nothing
// waiting.
func TestDigestSaysWhenPullRequestsCannotBeRead(t *testing.T) {
	cases := map[string]PullRequestReader{
		"the reader fails":     &fakePulls{err: errors.New("forge returned 502")},
		"no reader configured": nil,
	}
	for name, reader := range cases {
		t.Run(name, func(t *testing.T) {
			svc, store, until := newServiceWithPulls(t, reader)
			seedTwin(t, store, "twin-1", "CTO twin")
			seedStanding(t, store, "twin-1", "obj-a", "stream a")
			seedStanding(t, store, "twin-1", "obj-b", "stream b")

			d := assembleDay(t, svc, until)
			for _, id := range []string{"obj-a", "obj-b"} {
				s := summaryOf(t, d, id)
				if len(s.WaitingPullRequests) != 0 {
					t.Errorf("%s waiting = %+v, want none listed", id, s.WaitingPullRequests)
				}
				b, ok := blockOf(s.Blocks, digest.BlockRemoteUnreadable)
				if !ok {
					t.Fatalf("%s blocks = %+v, want a remote_unreadable block: an empty list must not read as nothing waiting", id, s.Blocks)
				}
				if !strings.Contains(b.Reason, "pull requests") {
					t.Errorf("%s block reason = %q, want it to name the pull requests", id, b.Reason)
				}
			}
		})
	}
}
