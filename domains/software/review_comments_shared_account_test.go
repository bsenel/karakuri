package software

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bsenel/karakuri/config"
)

// Phase 40 step 12a: where Karakuri and the owner post from one account, a
// comment from that account is the owner's unless its ID is among the comments
// the audit log records Karakuri as having posted. With no record to consult
// the observation is blind rather than guessing (AGENTS.md rule 10).

var sharedAuthors = config.OwnAuthorsConfig{
	Karakuri: []string{"karakuri-bot", "shared-acct"},
	Owners:   []string{"berat", "shared-acct"},
}

func rcObserveShared(t *testing.T, f *commentForge, limit int, posted postedCommentIDs) map[string]any {
	t.Helper()
	state := map[string]any{}
	observeUnansweredReviewComments(context.Background(), f, "", sharedAuthors, limit, posted, state)
	return state
}

func recorded(ids ...int64) postedCommentIDs {
	return func(context.Context) (map[int64]bool, error) {
		m := map[int64]bool{}
		for _, id := range ids {
			m[id] = true
		}
		return m, nil
	}
}

func requireBlind(t *testing.T, state map[string]any) {
	t.Helper()
	if _, ok := state["unanswered_review_comments_error"]; !ok {
		t.Errorf("no unanswered_review_comments_error; state keys: %v", stateKeys(state))
	}
	if _, ok := state["unanswered_review_comments"]; ok {
		t.Errorf("a list was left beside the error: %v", state["unanswered_review_comments"])
	}
	if !strings.Contains(strings.ToLower(rcLines(state)), "not an empty list") {
		t.Errorf("line must say this is not an empty list:\n%s", rcLines(state))
	}
}

func TestSharedAccount_CommentNotRecordedIsTheOwners(t *testing.T) {
	f := rcForge(rcBranch, rcComment(401, "shared-acct", rcLast.Add(time.Hour)))
	state := rcObserveShared(t, f, 0, recorded(999))

	got := rcListed(t, state)
	if len(got) != 1 || got[0].From != "owner" || got[0].CommentID != 401 || got[0].Author != "shared-acct" {
		t.Fatalf("want one owner entry for 401, got %+v", got)
	}
	if !strings.Contains(strings.ToLower(rcLines(state)), "the owner (shared-acct)") {
		t.Errorf("no owner line:\n%s", rcLines(state))
	}
	if hasThirdPartyComment(state) {
		t.Error("an owner's comment made the observation third party")
	}
}

func TestSharedAccount_CommentRecordedIsKarakurisOwn(t *testing.T) {
	f := rcForge(rcBranch, rcComment(402, "shared-acct", rcLast.Add(time.Hour)))
	state := rcObserveShared(t, f, 0, recorded(402))

	if got := rcListed(t, state); len(got) != 0 {
		t.Errorf("Karakuri's own recorded comment was listed: %+v", got)
	}
	if rcLines(state) != "" {
		t.Errorf("a line was written for Karakuri's own comment:\n%s", rcLines(state))
	}
	if _, bad := state["unanswered_review_comments_error"]; bad {
		t.Errorf("unexpected error: %v", state["unanswered_review_comments_error"])
	}
}

func TestSharedAccount_NoRecordIsBlind(t *testing.T) {
	for name, posted := range map[string]postedCommentIDs{
		"nil reader": nil,
		"reader fails": func(context.Context) (map[int64]bool, error) {
			return nil, errors.New("audit log down")
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := rcForge(rcBranch, rcComment(403, "shared-acct", rcLast.Add(time.Hour)))
			requireBlind(t, rcObserveShared(t, f, 0, posted))
		})
	}
}

func TestSharedAccount_KarakuriOnlyLoginNeedsNoRecord(t *testing.T) {
	f := rcForge(rcBranch, rcComment(404, "karakuri-bot", rcLast.Add(time.Hour)))
	state := rcObserveShared(t, f, 0, nil)

	if _, bad := state["unanswered_review_comments_error"]; bad {
		t.Errorf("a nil reader made a Karakuri-only comment blind: %v", state["unanswered_review_comments_error"])
	}
	if got := rcListed(t, state); len(got) != 0 {
		t.Errorf("Karakuri-only comment was listed: %+v", got)
	}
}

func TestSharedAccount_OwnerOnlyLoginIsOwnerWithNilReader(t *testing.T) {
	f := rcForge(rcBranch, rcComment(405, "berat", rcLast.Add(time.Hour)))
	state := rcObserveShared(t, f, 0, nil)

	got := rcListed(t, state)
	if len(got) != 1 || got[0].From != "owner" {
		t.Errorf("want one owner entry, got %+v", got)
	}
}

// Shared-account comments are the owner's, so they are not rounds: three
// answered shared-account comments leave a third-party comment listed at limit 3.
func TestSharedAccount_CommentsDoNotCountTowardRoundLimit(t *testing.T) {
	f := rcRounds(3, "shared-acct", "stranger")
	state := rcObserveShared(t, f, 3, recorded())

	got := rcListed(t, state)
	if len(got) != 1 || got[0].From != "third_party" || got[0].CommentID != 300 {
		t.Errorf("want the stranger's comment listed, got %+v", got)
	}
	if w := rcWaiting(state); len(w) != 0 {
		t.Errorf("shared-account comments counted as rounds: %v", w)
	}
}
