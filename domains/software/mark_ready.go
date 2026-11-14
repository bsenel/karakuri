package software

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// CapMarkReady takes the open draft pull request on params.branch out of
// draft once every item of its body's checklist is ticked and every check at
// its head is green (Phase 44 step 1).
const CapMarkReady = "software.act.mark_ready"

// verifierRule is a forge verifier as mark_ready tries it on recorded data:
// its capability, the files that register or implement it, and its rule over
// the check runs the forge reports on one pull request's head commit.
type verifierRule struct {
	id    string
	files []string
	rule  func(checks []versioncontrol.CheckRun) objective.CriterionState
}

// forgeVerifiers are the deployed forge verifiers mark_ready tries on this
// forge's own pull requests.
var forgeVerifiers = []verifierRule{
	{id: CapCIGreen, files: []string{"domains/software/stream_verifiers.go"}, rule: ciGreenState},
}

// verifierSampleMerged is how many of the most recently merged pull requests a
// changed verifier is tried on, beside every other open one.
const verifierSampleMerged = 5

// verifierDataMarker opens the section of a pull request's body that holds the
// comparison; the next comparison replaces it rather than adding another.
const verifierDataMarker = "<!-- karakuri:verifier-meets-data -->"

// changedVerifiers are the rules whose files the diff touches.
func changedVerifiers(rules []verifierRule, files []string) []verifierRule {
	var out []verifierRule
	for _, r := range rules {
		for _, f := range r.files {
			if slices.Contains(files, f) {
				out = append(out, r)
				break
			}
		}
	}
	return out
}

// verifierMeetsData runs each changed rule, read-only, over the checks at the
// head of every other open pull request and of the most recently merged ones,
// and compares its verdict with the forge's own status: met where the forge
// says success, not met where it does not. It returns one body line per
// verdict, one finding per disagreement, and the provenance of what it read.
// A forge that cannot answer is an error: the comparison did not run.
func (e *gitEnv) verifierMeetsData(ctx context.Context, repo string, pr versioncontrol.PRSummary, rules []verifierRule) (lines, findings []string, trust environment.Trust, err error) {
	open, err := e.vc.ListPRs(ctx, repo, time.Time{})
	if err != nil {
		return nil, nil, "", fmt.Errorf("list open pull requests: %w", err)
	}
	merged, err := e.vc.ListMergedPRs(ctx, repo, verifierSampleMerged)
	if err != nil {
		return nil, nil, "", fmt.Errorf("list merged pull requests: %w", err)
	}
	type sampled struct {
		pr   versioncontrol.PRSummary
		kind string
	}
	var sample []sampled
	read := []versioncontrol.PRSummary{pr}
	for _, o := range open {
		if o.ID != pr.ID {
			sample = append(sample, sampled{o, "open"})
			read = append(read, o)
		}
	}
	for _, m := range merged {
		sample = append(sample, sampled{m, "merged"})
		read = append(read, m)
	}
	for _, s := range sample {
		if s.pr.HeadSHA == "" {
			lines = append(lines, fmt.Sprintf("- #%s (%s): the forge reported no head commit, not compared", s.pr.ID, s.kind))
			continue
		}
		checks, err := e.vc.ListChecks(ctx, repo, s.pr.HeadSHA)
		if err != nil {
			return nil, nil, "", fmt.Errorf("list checks at #%s's head %s: %w", s.pr.ID, s.pr.HeadSHA, err)
		}
		head := atHead(checks, s.pr.HeadSHA)
		for _, r := range rules {
			state := r.rule(head)
			if s.pr.CheckState == "" {
				lines = append(lines, fmt.Sprintf("- %s on #%s (%s, %s): %s; the forge reports no status, not compared",
					r.id, s.pr.ID, s.kind, s.pr.HeadSHA, state))
				continue
			}
			agrees := (state == objective.CriterionMet) == (s.pr.CheckState == "success")
			word := "agrees"
			if !agrees {
				word = "disagrees"
				findings = append(findings, fmt.Sprintf("%s calls #%s (%s, %s) %s where the forge reports %s",
					r.id, s.pr.ID, s.kind, s.pr.HeadSHA, state, s.pr.CheckState))
			}
			lines = append(lines, fmt.Sprintf("- %s on #%s (%s, %s): %s; the forge reports %s; %s",
				r.id, s.pr.ID, s.kind, s.pr.HeadSHA, state, s.pr.CheckState, word))
		}
	}
	return lines, findings, prTrust(read), nil
}

// withVerdicts is body with its comparison section replaced by lines, the rest
// of the body, checklist included, as it was.
func withVerdicts(body string, lines []string) string {
	if i := strings.Index(body, verifierDataMarker); i >= 0 {
		body = body[:i]
	}
	return strings.TrimRight(body, "\n") + "\n\n" + verifierDataMarker + "\n" +
		"Changed verifiers on this forge's own pull requests, beside what the forge reports:\n\n" +
		strings.Join(lines, "\n") + "\n"
}

// checklistItem is one Markdown task list item: a bullet, a box that is empty
// or holds an x of either case, and the item's text.
var checklistItem = regexp.MustCompile(`^\s*[-*+]\s+\[([ xX])\]\s+(.*)$`)

// checklist reads a pull request body's task list: how many items it has and
// the text of each one not ticked.
func checklist(body string) (items int, unticked []string) {
	for _, line := range strings.Split(body, "\n") {
		m := checklistItem.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if m == nil {
			continue
		}
		items++
		if m[1] == " " {
			unticked = append(unticked, strings.TrimSpace(m[2]))
		}
	}
	return items, unticked
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

	// A pull request that changes a verifier meets this forge's own data first
	// (Phase 44 step 5).
	if len(e.verifiers) > 0 {
		files, err := e.vc.ChangedFiles(ctx, repo, pr.ID)
		if err != nil {
			delta["verifier_comparison"] = "not_run"
			return fail("could not tell whether pull request "+pr.ID+" changes a verifier: "+err.Error(), trust, delta)
		}
		if changed := changedVerifiers(e.verifiers, files); len(changed) > 0 {
			lines, findings, readTrust, err := e.verifierMeetsData(ctx, repo, pr, changed)
			if err != nil {
				// Never read as agreement: a comparison that did not run leaves
				// the pull request a draft (rule 10).
				delta["verifier_comparison"] = "not_run"
				if errors.Is(err, versioncontrol.ErrUnsupported) {
					return fail("unsupported: the changed verifier was not tried on this forge's data: "+err.Error(), trust, delta)
				}
				return fail("the changed verifier was not tried on this forge's data: "+err.Error(), trust, delta)
			}
			if readTrust == environment.TrustThirdParty {
				trust = readTrust
			}
			delta["verifier_comparison"], delta["verdicts"], delta["findings"] = "run", lines, findings
			if err := e.vc.EditPRBody(ctx, repo, pr.ID, withVerdicts(pr.Body, lines)); err != nil {
				return fail("could not write the verdicts into pull request "+pr.ID+"'s body: "+err.Error(), trust, delta)
			}
			if len(findings) > 0 {
				return fail(fmt.Sprintf("the changed verifier disagrees with this forge on %d pull request(s): %s",
					len(findings), strings.Join(findings, "; ")), trust, delta)
			}
		}
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
