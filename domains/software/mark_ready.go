package software

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// CapMarkReady takes the open draft pull request on params.branch out of
// draft once every item of its body's checklist is ticked and every check at
// its head is green (Phase 44 step 1).
const CapMarkReady = "software.act.mark_ready"

// checklistItem is one Markdown task list item: a bullet, a box that is empty
// or holds an x of either case, and the item's text.
var checklistItem = regexp.MustCompile(`^\s*[-*+]\s+\[([ xX])\]\s+(.*)$`)

// checkbox is one item of a pull request body's task list.
type checkbox struct {
	ticked bool
	text   string
}

// checkboxes reads a pull request body's task list in order, one checkbox per
// item. markReady and the git observation both read it here, so they cannot
// disagree about what the list says.
func checkboxes(body string) []checkbox {
	var boxes []checkbox
	for _, line := range strings.Split(body, "\n") {
		m := checklistItem.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if m == nil {
			continue
		}
		boxes = append(boxes, checkbox{ticked: m[1] != " ", text: strings.TrimSpace(m[2])})
	}
	return boxes
}

// checklist reads a pull request body's task list: how many items it has and
// the text of each one not ticked.
func checklist(body string) (items int, unticked []string) {
	boxes := checkboxes(body)
	for _, b := range boxes {
		if !b.ticked {
			unticked = append(unticked, b.text)
		}
	}
	return len(boxes), unticked
}

// markReady marks the draft pull request on params.branch ready for review
// when its group is done: at least one checklist item in its body, every item
// ticked, and every check at its head green by ci_green's rules, read once.
// Anything short of that leaves the pull request a draft, fails, and says
// what stands in the way. A pull request already out of draft is a success.
func (e *gitEnv) markReady(ctx context.Context, a environment.Action) environment.ActionResult {
	fail := func(msg string, trust environment.Trust, delta map[string]any) environment.ActionResult {
		return environment.ActionResult{Success: false, Trust: trust, StateDelta: delta,
			Error: fmt.Sprintf("%s: %s", CapMarkReady, msg)}
	}
	// No forge to ask is an error, never an empty success (rule 10).
	if e.vc == nil || !e.vc.Active() {
		return fail("no version control instance is bound", environment.TrustOperator, nil)
	}
	branch := asString(a.Params, "branch")
	if branch == "" {
		return fail("needs the group's branch: pass params.branch", environment.TrustOperator, nil)
	}
	repo := asString(a.Params, "repo")

	pr, found, err := e.branchPR(ctx, repo, branch, time.Time{})
	if err != nil {
		return fail("could not list pull requests: "+err.Error(), environment.TrustOperator, nil)
	}
	if !found {
		return fail("no open pull request on "+branch, environment.TrustOperator, nil)
	}
	// The checklist and the check names are text the pull request's author
	// controls, so every result past this point is as trusted as they are.
	trust := prTrust([]versioncontrol.PRSummary{pr})
	delta := prRef(pr)
	if !pr.Draft {
		delta["already_ready"] = true
		return environment.ActionResult{Success: true, Trust: trust, StateDelta: delta}
	}

	items, unticked := checklist(pr.Body)
	delta["checklist_items"], delta["unticked"] = items, unticked
	if items == 0 {
		// Every one of zero items being ticked must not mark a group done.
		return fail(fmt.Sprintf("pull request %s has no checklist in its body", pr.ID), trust, delta)
	}
	if len(unticked) > 0 {
		return fail(fmt.Sprintf("pull request %s has unticked items: %s", pr.ID, strings.Join(unticked, "; ")), trust, delta)
	}

	if pr.HeadSHA == "" {
		return fail(fmt.Sprintf("the forge reported no head commit for pull request %s", pr.ID), trust, delta)
	}
	failing, pending, notRun, nonBlocking, green, err := e.headChecks(ctx, repo, pr.HeadSHA)
	if err != nil {
		return fail("could not list checks: "+err.Error(), trust, delta)
	}
	delta["green"], delta["failing"], delta["pending"], delta["not_run"] = green, failing, pending, notRun
	delta["skipped_or_neutral"] = nonBlocking
	switch {
	case len(failing) > 0:
		return fail("failing at "+pr.HeadSHA+": "+strings.Join(failing, ", "), trust, delta)
	case len(pending) > 0:
		return fail("still pending at "+pr.HeadSHA+": "+strings.Join(pending, ", "), trust, delta)
	case len(notRun) > 0:
		return fail("cancelled or awaiting approval at "+pr.HeadSHA+": "+strings.Join(notRun, ", "), trust, delta)
	case len(green) == 0:
		return fail("no check has reported on the head commit "+pr.HeadSHA, trust, delta)
	}

	if err := e.vc.MarkReady(ctx, repo, pr.ID); err != nil {
		if errors.Is(err, versioncontrol.ErrUnsupported) {
			return fail("unsupported: the forge cannot mark a pull request ready: "+err.Error(), trust, delta)
		}
		return fail("could not mark pull request "+pr.ID+" ready: "+err.Error(), trust, delta)
	}
	delta["marked_ready"] = true
	return environment.ActionResult{Success: true, Trust: trust, StateDelta: delta}
}
