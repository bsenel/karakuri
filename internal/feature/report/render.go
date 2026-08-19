package report

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	coreagent "github.com/bsenel/karakuri/internal/core/agent"
	"github.com/bsenel/karakuri/internal/core/digest"
)

// render asks an agent to write the narrative, and falls back to a plain
// rendering when it cannot.
//
// The fallback is not a degraded mode to be tolerated — it is the contract.
// The structured digest already contains everything the reader needs, and a
// report that failed to arrive because a model was rate-limited would be a
// worse outcome than a plainer one that did. So the model writes the prose and
// nothing else: it never decides what is in the report, and it never decides
// what needs a decision.
func (s *Service) render(ctx context.Context, d digest.Digest) string {
	plain := Plain(d)
	if s.factory == nil {
		return plain
	}

	agent, err := s.factory.New(ctx, coreagent.Definition{
		ID:                "karakuri-reporter",
		Name:              "Reporter",
		Domain:            "universal",
		ReasoningStrategy: coreagent.ReasoningChainOfThought,
	})
	if err != nil {
		return plain
	}

	facts, err := json.MarshalIndent(struct {
		digest.Digest
		Prose string `json:"prose,omitempty"`
	}{Digest: d}, "", "  ")
	if err != nil {
		return plain
	}

	out, err := agent.Run(ctx, coreagent.Input{
		Task: reporterPrompt + "\n\n" + string(facts),
	})
	if err != nil || strings.TrimSpace(out.Content) == "" {
		slog.Debug("digest prose fell back to the plain rendering", "err", err)
		return plain
	}
	// The plain rendering is appended rather than replaced. The prose is a
	// summary and summaries lose things; the reader who wants the numbers
	// should not have to ask for a second report.
	return strings.TrimSpace(out.Content) + "\n\n---\n\n" + plain
}

const reporterPrompt = `Write a short status brief from the JSON below. You are summarising what an
autonomous system did on somebody's behalf while they were not watching.

Rules:
- Lead with anything that needs a decision, then anything that failed or was
  paused, then what went well. Do not bury a blocked objective under good news.
- Report only what the JSON says. Do not infer causes, do not estimate, and do
  not suggest actions the data does not support.
- If a number is absent, say it is absent rather than treating it as zero.
- Six sentences at most. No preamble, no sign-off, no markdown headings.`

// Plain renders the digest without a model.
//
// Deliberately complete rather than pretty: this is what gets delivered when
// no agent is available, and it is appended beneath the prose when one is, so
// it has to stand on its own.
func Plain(d digest.Digest) string {
	var b strings.Builder

	name := d.TwinName
	if name == "" {
		name = d.TwinID
	}
	fmt.Fprintf(&b, "%s — %s to %s\n",
		name, d.Since.Format(time.RFC1123), d.Until.Format(time.RFC1123))

	if len(d.Decisions) > 0 {
		// First, always. This is why the report is sent rather than left in a
		// console somebody might open.
		fmt.Fprintf(&b, "\nDecisions I need from you (%d)\n", len(d.Decisions))
		for _, dec := range d.Decisions {
			fmt.Fprintf(&b, "  · %s\n", dec.Summary)
			if dec.ObjectiveTitle != "" {
				fmt.Fprintf(&b, "    on: %s\n", dec.ObjectiveTitle)
			}
			if len(dec.Proposed) > 0 {
				fmt.Fprintf(&b, "    proposed: %s\n", strings.Join(dec.Proposed, ", "))
			}
			fmt.Fprintf(&b, "    waiting %s · %s · krk checkpoint resolve %s\n",
				roughly(dec.Age(d.Until)), strings.Join(dec.Options, "|"), dec.CheckpointID)
		}
	}

	if len(d.AutonomyChanges) > 0 {
		b.WriteString("\nAutonomy\n")
		for _, a := range d.AutonomyChanges {
			verb := "narrowed to"
			if a.Promoted() {
				verb = "widened to"
			}
			fmt.Fprintf(&b, "  · %s %s %s (%s)\n", a.ObjectiveTitle, verb, a.To, a.Reason)
		}
	}

	if len(d.Objectives) > 0 {
		b.WriteString("\nStanding objectives\n")
		for _, o := range d.Objectives {
			fmt.Fprintf(&b, "  · %s — %s\n", o.Title, o.Status)
			// Senses first: the ratio between the two is the answer to "is
			// this costing me anything", and a line that showed only the
			// reconciles would make a well-behaved objective look idle.
			fmt.Fprintf(&b, "    %d checks, %d reconciles, %d actions", o.Senses, o.Reconciles, o.Actions)
			if o.DriftDetected > 0 {
				fmt.Fprintf(&b, ", %d drifts", o.DriftDetected)
			}
			if o.Escalations > 0 {
				fmt.Fprintf(&b, ", %d escalations", o.Escalations)
			}
			if o.Failures > 0 {
				fmt.Fprintf(&b, ", %d failures", o.Failures)
			}
			fmt.Fprintf(&b, " · criteria %.0f%%\n", o.CriteriaMet*100)
			if o.Paused {
				fmt.Fprintf(&b, "    PAUSED: %s\n", o.PausedWhy)
			} else if o.LastError != "" {
				fmt.Fprintf(&b, "    last error: %s\n", o.LastError)
			}
		}
	}

	b.WriteString("\nSpend\n")
	switch {
	case !d.Spend.Priced:
		// Not the same as free, and saying so is the difference between a
		// report that is honest and one that is reassuring.
		b.WriteString("  not priced — no rate table is configured, so units were counted and nothing was costed\n")
	default:
		fmt.Fprintf(&b, "  %.2f\n", d.Spend.Cost)
		for provider, amount := range d.Spend.ByProvider {
			if provider == "" {
				provider = "(unattributed)"
			}
			fmt.Fprintf(&b, "  · %s %.2f\n", provider, amount)
		}
	}

	return b.String()
}

// roughly renders a duration the way somebody reads it aloud. "waiting 3 days"
// lands; "waiting 76h12m4.331s" does not.
func roughly(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "less than a minute"
	case d < time.Hour:
		return plural(int(d.Minutes()), "minute")
	case d < 48*time.Hour:
		return plural(int(d.Hours()), "hour")
	default:
		return plural(int(d.Hours()/24), "day")
	}
}

func plural(n int, unit string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", unit)
	}
	return fmt.Sprintf("%d %ss", n, unit)
}
