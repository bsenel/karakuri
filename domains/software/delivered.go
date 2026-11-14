package software

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

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
