package software

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/core/objective"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// The deterministic stream verifiers (Phase 41 step 2). Each settles one
// criterion from what the forge reports and returns the criterion's state in
// StateDelta "state", and the reason in "reason" when it is inconclusive.
// Success is the criterion being met and nothing else, so an inconclusive
// result never reads as a pass.
const (
	// CapStreamPR verifies that the stream's pull request was opened or
	// updated in this pass.
	CapStreamPR = "software.verify.stream_pr"
	// CapCIGreen verifies that every check at the pull request's head is green.
	CapCIGreen = "software.verify.ci_green"
	// CapPathsDeclared verifies that the pull request's diff touches only the
	// declared path globs.
	CapPathsDeclared = "software.verify.paths_declared"
)

// Bounds on params.wait_sec for ci_green: how long a pending check is waited
// for before it counts as not run, and how often the checks are re-read.
const (
	ciGreenDefaultWaitSec = 120
	ciGreenMaxWaitSec     = 900
	ciGreenPollInterval   = 10 * time.Second
)

// verdict builds a verifier's result. Only an inconclusive state carries a
// reason, and only a met one is a success.
func verdict(verifier string, state objective.CriterionState, reason objective.InconclusiveReason, msg string, trust environment.Trust, extra map[string]any) environment.ActionResult {
	delta := map[string]any{"verifier": verifier, "state": string(state)}
	if state == objective.CriterionInconclusive {
		delta["reason"] = string(reason)
	}
	for k, v := range extra {
		delta[k] = v
	}
	res := environment.ActionResult{Success: state == objective.CriterionMet, Trust: trust, StateDelta: delta}
	if state != objective.CriterionMet {
		res.Error = fmt.Sprintf("%s: %s", verifier, msg)
	}
	return res
}

// inconclusive is a verifier that could not settle the criterion. What it
// carries is the caller's own words, so the operator's.
func inconclusive(verifier string, reason objective.InconclusiveReason, msg string) environment.ActionResult {
	return verdict(verifier, objective.CriterionInconclusive, reason, msg, environment.TrustOperator, nil)
}

// forgeFailure is a forge that could not answer: unsupported when the adapter
// says it has no such read, a verifier error otherwise. Never met, never not met.
func forgeFailure(verifier, what string, err error) environment.ActionResult {
	if errors.Is(err, versioncontrol.ErrUnsupported) {
		return inconclusive(verifier, objective.ReasonUnsupported, fmt.Sprintf("the forge cannot %s: %v", what, err))
	}
	return inconclusive(verifier, objective.ReasonVerifierError, fmt.Sprintf("could not %s: %v", what, err))
}

// streamVerify dispatches the three verifiers. The second result is false when
// the capability is not one of them.
func (e *gitEnv) streamVerify(ctx context.Context, a environment.Action) (environment.ActionResult, bool) {
	verifier := string(a.CapabilityID)
	switch verifier {
	case CapStreamPR, CapCIGreen, CapPathsDeclared:
	default:
		return environment.ActionResult{}, false
	}
	// No forge to ask is an error, never an empty success (rule 10).
	if e.vc == nil || !e.vc.Active() {
		return inconclusive(verifier, objective.ReasonVerifierError, "no version control instance is bound"), true
	}
	branch := asString(a.Params, "branch")
	if branch == "" {
		return inconclusive(verifier, objective.ReasonVerifierError, "needs the stream's branch: pass params.branch"), true
	}
	repo := asString(a.Params, "repo")
	switch verifier {
	case CapStreamPR:
		return e.verifyStreamPR(ctx, repo, branch, a.Params), true
	case CapCIGreen:
		return e.verifyCIGreen(ctx, repo, branch, a.Params), true
	default:
		return e.verifyPathsDeclared(ctx, repo, branch, a.Params), true
	}
}

// branchPR finds the open pull request whose head is branch among those
// updated since since; a zero since is every open one.
func (e *gitEnv) branchPR(ctx context.Context, repo, branch string, since time.Time) (versioncontrol.PRSummary, bool, error) {
	prs, err := e.vc.ListPRs(ctx, repo, since)
	if err != nil {
		return versioncontrol.PRSummary{}, false, err
	}
	for _, pr := range prs {
		if pr.HeadBranch == branch {
			return pr, true, nil
		}
	}
	return versioncontrol.PRSummary{}, false, nil
}

// prRef is what a verdict says about the pull request it read: an ID, a URL
// and a SHA, none of which its author typed.
func prRef(pr versioncontrol.PRSummary) map[string]any {
	return map[string]any{"pr": pr.ID, "url": pr.URL, "head_sha": pr.HeadSHA}
}

// verifyStreamPR is met when the pull request on the branch was opened or
// updated at or after params.since, the start of the pass. No such pull
// request is no data: a discovery pass that found nothing opens none, and that
// is neither a pass nor a failure.
func (e *gitEnv) verifyStreamPR(ctx context.Context, repo, branch string, params map[string]any) environment.ActionResult {
	since, err := time.Parse(time.RFC3339, asString(params, "since"))
	if err != nil {
		return inconclusive(CapStreamPR, objective.ReasonVerifierError,
			"needs the start of the pass: pass params.since as an RFC 3339 time")
	}
	pr, found, err := e.branchPR(ctx, repo, branch, since)
	if err != nil {
		return forgeFailure(CapStreamPR, "list pull requests", err)
	}
	if !found {
		return inconclusive(CapStreamPR, objective.ReasonNoData, fmt.Sprintf("no open pull request on %s", branch))
	}
	if pr.CreatedAt.Before(since) && pr.UpdatedAt.Before(since) {
		return verdict(CapStreamPR, objective.CriterionInconclusive, objective.ReasonNoData,
			fmt.Sprintf("pull request %s on %s was not opened or updated since %s", pr.ID, branch, since.Format(time.RFC3339)),
			environment.TrustOperator, prRef(pr))
	}
	return verdict(CapStreamPR, objective.CriterionMet, "", "", environment.TrustOperator, prRef(pr))
}

// verifyCIGreen is met when every check at the pull request's head completed
// with success. A check on an older commit settles nothing, a failure is not
// met, and a check pending when the bounded wait ends, skipped or neutral did
// not run as far as the verifier can tell.
func (e *gitEnv) verifyCIGreen(ctx context.Context, repo, branch string, params map[string]any) environment.ActionResult {
	pr, found, err := e.branchPR(ctx, repo, branch, time.Time{})
	if err != nil {
		return forgeFailure(CapCIGreen, "list pull requests", err)
	}
	if !found {
		return inconclusive(CapCIGreen, objective.ReasonNoData, fmt.Sprintf("no open pull request on %s", branch))
	}
	if pr.HeadSHA == "" {
		return inconclusive(CapCIGreen, objective.ReasonVerifierError, fmt.Sprintf("the forge reported no head commit for pull request %s", pr.ID))
	}
	// Check names come from the repository's CI configuration, which a pull
	// request can change, so the payload is as trusted as the pull request.
	trust := prTrust([]versioncontrol.PRSummary{pr})
	deadline := time.Now().Add(time.Duration(ciGreenWaitSec(params)) * time.Second)
	for {
		checks, err := e.vc.ListChecks(ctx, repo, pr.HeadSHA)
		if err != nil {
			return forgeFailure(CapCIGreen, "list checks", err)
		}
		var atHead []versioncontrol.CheckRun
		for _, c := range checks {
			if c.HeadSHA == pr.HeadSHA {
				atHead = append(atHead, c)
			}
		}
		failing, pending, notRun, green := sortChecks(atHead)
		extra := prRef(pr)
		extra["green"], extra["failing"], extra["pending"], extra["not_run"] = green, failing, pending, notRun
		switch {
		case len(failing) > 0:
			// Red wins over pending: a run still going cannot un-fail one that
			// already failed.
			return verdict(CapCIGreen, objective.CriterionNotMet, "",
				"failing at "+pr.HeadSHA+": "+strings.Join(failing, ", "), trust, extra)
		case len(pending) > 0:
			if wait := time.Until(deadline); wait > 0 {
				select {
				case <-ctx.Done():
					return forgeFailure(CapCIGreen, "wait for pending checks", ctx.Err())
				case <-time.After(min(wait, ciGreenPollInterval)):
				}
				continue
			}
			return verdict(CapCIGreen, objective.CriterionInconclusive, objective.ReasonNotRun,
				"still pending when the wait ended: "+strings.Join(pending, ", "), trust, extra)
		case len(notRun) > 0:
			return verdict(CapCIGreen, objective.CriterionInconclusive, objective.ReasonNotRun,
				"skipped or neutral: "+strings.Join(notRun, ", "), trust, extra)
		case len(green) == 0:
			return verdict(CapCIGreen, objective.CriterionInconclusive, objective.ReasonNoData,
				"no check has reported on the head commit "+pr.HeadSHA, trust, extra)
		}
		return verdict(CapCIGreen, objective.CriterionMet, "", "", trust, extra)
	}
}

// sortChecks splits checks by what they say: failed, still running, completed
// without running (skipped or neutral), and green.
func sortChecks(checks []versioncontrol.CheckRun) (failing, pending, notRun, green []string) {
	for _, c := range checks {
		switch {
		case c.Status != "completed":
			pending = append(pending, c.Name)
		case c.Conclusion == "success":
			green = append(green, c.Name)
		case c.Conclusion == "skipped", c.Conclusion == "neutral":
			notRun = append(notRun, c.Name)
		default:
			failing = append(failing, c.Name)
		}
	}
	return failing, pending, notRun, green
}

// ciGreenWaitSec reads params.wait_sec, where zero means "do not wait", so it
// cannot go through positiveIntParam.
func ciGreenWaitSec(params map[string]any) int {
	if _, ok := params["wait_sec"]; !ok {
		return ciGreenDefaultWaitSec
	}
	n := positiveIntParam(params, "wait_sec", 0)
	if n > ciGreenMaxWaitSec {
		return ciGreenMaxWaitSec
	}
	return n
}

// verifyPathsDeclared is met when every path the pull request's diff touches
// matches one of params.paths. An empty diff is no data: every one of zero
// paths being inside must not settle a criterion.
func (e *gitEnv) verifyPathsDeclared(ctx context.Context, repo, branch string, params map[string]any) environment.ActionResult {
	globs := pathGlobsParam(params)
	if len(globs) == 0 {
		return inconclusive(CapPathsDeclared, objective.ReasonVerifierError,
			"needs the declared paths: pass params.paths, a list of globs or one comma-separated string")
	}
	pr, found, err := e.branchPR(ctx, repo, branch, time.Time{})
	if err != nil {
		return forgeFailure(CapPathsDeclared, "list pull requests", err)
	}
	if !found {
		return inconclusive(CapPathsDeclared, objective.ReasonNoData, fmt.Sprintf("no open pull request on %s", branch))
	}
	files, err := e.vc.ChangedFiles(ctx, repo, pr.ID)
	if err != nil {
		return forgeFailure(CapPathsDeclared, "list changed files", err)
	}
	extra := prRef(pr)
	if len(files) == 0 {
		return verdict(CapPathsDeclared, objective.CriterionInconclusive, objective.ReasonNoData,
			fmt.Sprintf("pull request %s changes no files", pr.ID), environment.TrustOperator, extra)
	}
	// A file name is typed by whoever wrote the pull request.
	trust := prTrust([]versioncontrol.PRSummary{pr})
	var outside []string
	for _, f := range files {
		if !capability.TouchesAnyGlob([]string{f}, globs) {
			outside = append(outside, f)
		}
	}
	extra["paths"], extra["outside"] = globs, outside
	if len(outside) > 0 {
		return verdict(CapPathsDeclared, objective.CriterionNotMet, "",
			"outside "+strings.Join(globs, ", ")+": "+strings.Join(outside, ", "), trust, extra)
	}
	return verdict(CapPathsDeclared, objective.CriterionMet, "", "", trust, extra)
}

// pathGlobsParam reads params.paths as a list of strings, as []any off JSON, or
// as one comma-separated string, trimmed and without empties.
func pathGlobsParam(params map[string]any) []string {
	var raw []string
	switch v := params["paths"].(type) {
	case []string:
		raw = v
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				raw = append(raw, s)
			}
		}
	case string:
		raw = strings.Split(v, ",")
	}
	globs := make([]string, 0, len(raw))
	for _, s := range raw {
		if s = strings.TrimSpace(s); s != "" {
			globs = append(globs, s)
		}
	}
	return globs
}
