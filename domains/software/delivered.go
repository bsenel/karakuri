package software

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/bsenel/karakuri/internal/core/environment"
	"github.com/bsenel/karakuri/internal/platform/tools/versioncontrol"
)

// Phase 44 step 3, "plan from what is delivered": the git observation shows
// each open delivery pull request's checklist, so a plan can see which steps
// are already ticked instead of redoing them.

// titlePhase reads the phase from a delivery pull request title's suffix,
// '(phase 44 step 3)' or '(phase 44 steps 4 to 6)'.
var titlePhase = regexp.MustCompile(`(?i)\(phase\s+(\d+)\s+steps?\b[^)]*\)\s*$`)

// itemStep reads the step an item is about from its start: 'Step 4',
// 'step 4', 'Steps 4 and 5' or '4.'.
var itemStep = regexp.MustCompile(`^(?:[Ss]teps?\s+(\d+)\b|(\d+)\.(?:\s|$))`)

// itemSHA is a commit an item names: a hex word of 7 to 40 characters.
var itemSHA = regexp.MustCompile(`\b[0-9a-f]{7,40}\b`)

// checklistStep is one item of an open pull request's checklist as the git
// observation carries it. Step is 'phase NN step MM', or empty when the title
// gives no phase or the item names no step.
type checklistStep struct {
	Text   string   `json:"text"`
	Ticked bool     `json:"ticked"`
	SHAs   []string `json:"shas,omitempty"`
	Step   string   `json:"step,omitempty"`
}

// observedPR is a pull request entry of the git observation: the forge's
// summary and, when its body has a task list, its checklist. A body with no
// task list gives no checklist field, because "no checklist" and "nothing
// ticked" are different things to a planner.
type observedPR struct {
	versioncontrol.PRSummary
	Checklist []checklistStep `json:"checklist,omitempty"`
	// The commits on the head branch since the base, read only when there is
	// a checklist: at most sinceBaseCap of them, how many more there are, or
	// why they could not be read.
	SinceBase            []sinceBaseCommit `json:"commits_since_base,omitempty"`
	SinceBaseMore        int               `json:"commits_since_base_more,omitempty"`
	SinceBaseUnavailable string            `json:"commits_since_base_unavailable,omitempty"`
}

// prChecklist reads pr's body into the checklist the observation carries.
func prChecklist(pr versioncontrol.PRSummary) []checklistStep {
	boxes := checkboxes(pr.Body)
	if len(boxes) == 0 {
		return nil
	}
	phase := 0
	if m := titlePhase.FindStringSubmatch(pr.Title); m != nil {
		phase, _ = strconv.Atoi(m[1])
	}
	steps := make([]checklistStep, 0, len(boxes))
	for _, b := range boxes {
		s := checklistStep{Text: b.text, Ticked: b.ticked, SHAs: itemSHA.FindAllString(b.text, -1)}
		if m := itemStep.FindStringSubmatch(b.text); phase > 0 && m != nil {
			// Atoi rather than the digits as written, so 'Step 03' and
			// 'step 3' name the same step.
			n, _ := strconv.Atoi(m[1] + m[2])
			s.Step = fmt.Sprintf("phase %d step %d", phase, n)
		}
		steps = append(steps, s)
	}
	return steps
}

// withChecklists pairs each pull request with its checklist, and writes the
// checklists as text for the planner, one line per item: the box, the step
// when it can be read, the item's text and the SHAs it names.
func withChecklists(prs []versioncontrol.PRSummary) (entries []observedPR, lines []string) {
	entries = make([]observedPR, 0, len(prs))
	for _, pr := range prs {
		steps := prChecklist(pr)
		entries = append(entries, observedPR{PRSummary: pr, Checklist: steps})
		for _, s := range steps {
			var line strings.Builder
			if s.Ticked {
				line.WriteString("[x] ")
			} else {
				line.WriteString("[ ] ")
			}
			if s.Step != "" {
				line.WriteString(s.Step + ": ")
			}
			line.WriteString(s.Text)
			if len(s.SHAs) > 0 {
				line.WriteString(" (" + strings.Join(s.SHAs, ", ") + ")")
			}
			lines = append(lines, line.String())
		}
	}
	return entries, lines
}

// deliveredWork is the ticked items of the entries' checklists that name a
// step, in the core's own terms, so the decide policy can ask about a plan
// that would do one again without reading pull requests or Markdown. An
// unticked item is not delivered, and a ticked one whose step cannot be read
// names nothing a plan could be matched against.
func deliveredWork(entries []observedPR) []environment.DeliveredWork {
	var delivered []environment.DeliveredWork
	for _, e := range entries {
		for _, s := range e.Checklist {
			if !s.Ticked || s.Step == "" {
				continue
			}
			delivered = append(delivered, environment.DeliveredWork{
				Ref: s.Step, Where: fmt.Sprintf("pull request #%d", e.Number), SHAs: s.SHAs,
			})
		}
	}
	return delivered
}

// sinceBaseCap is how many commits since base one pull request entry carries.
// The lines land in every reason prompt, so a long branch is cut and the rest
// counted.
const sinceBaseCap = 30

// sinceBaseCommit is one commit on a pull request's head branch since its
// base: the short SHA and the subject line.
type sinceBaseCommit struct {
	SHA     string `json:"sha"`
	Subject string `json:"subject"`
}

// withCommitsSinceBase reads, for each entry with a checklist, the commits on
// its head branch since its base, and writes them as lines for the planner.
// An entry without a checklist is not a delivery group a plan checks against,
// so it costs no compare call. A failed or unsupported compare is said with
// its reason, never left as an empty list (AGENTS.md rule 10).
func withCommitsSinceBase(ctx context.Context, vc versioncontrol.VersionControlAdapter, repo string, entries []observedPR) (lines []string) {
	for i := range entries {
		e := &entries[i]
		if len(e.Checklist) == 0 {
			continue
		}
		prefix := fmt.Sprintf("#%d ", e.Number)
		commits, err := vc.CompareCommits(ctx, repo, e.BaseBranch, e.HeadBranch)
		if err != nil {
			e.SinceBaseUnavailable = err.Error()
			lines = append(lines, prefix+"commits since base: unavailable: "+err.Error())
			continue
		}
		if len(commits) == 0 {
			// The entry's list is omitted when empty, so the line is what
			// tells "nothing on the branch yet" from "not read".
			lines = append(lines, prefix+"commits since base: none")
			continue
		}
		shown := commits[:min(len(commits), sinceBaseCap)]
		e.SinceBase = make([]sinceBaseCommit, 0, len(shown))
		for _, c := range shown {
			sc := sinceBaseCommit{SHA: c.SHA[:min(len(c.SHA), 7)], Subject: strings.SplitN(c.Message, "\n", 2)[0]}
			e.SinceBase = append(e.SinceBase, sc)
			lines = append(lines, prefix+sc.SHA+" "+sc.Subject)
		}
		if more := len(commits) - len(shown); more > 0 {
			e.SinceBaseMore = more
			lines = append(lines, fmt.Sprintf("%scommits since base: %d more not shown", prefix, more))
		}
	}
	return lines
}
