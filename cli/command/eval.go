package command

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"time"

	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

// The CLI may not import internal/feature/eval (cli/AGENTS.md), so the report
// and golden entries are decoded into local types. CalibrationReport carries no
// JSON tags, so its fields arrive under their Go names; GoldenEntry's tags are
// mirrored here so an exported file loads with eval.LoadGoldenSet.
type evalReport struct {
	TwinID       string
	Since, Until time.Time

	N, Agreed, Skipped int
	Agreement          float64
	Replayable         int

	Confusion struct {
		JudgePassHumanApprove, JudgePassHumanReject int
		JudgeFailHumanApprove, JudgeFailHumanReject int
	}
	ByDecision map[string]struct{ N, Agreed, JudgePass int }
	Items      []evalItem
}

type evalItem struct {
	CheckpointID, ObjectiveID, Choice string
	HumanApprove, JudgePass, Agreed   bool
	Reply, Error                      string
	Title, Criterion, Actions         string
}

type goldenEntry struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Criterion  string `json:"criterion"`
	Actions    string `json:"actions"`
	Label      string `json:"label"`
	Reply      string `json:"reply"`
	Provenance string `json:"provenance"`
	Note       string `json:"note,omitempty"`
}

// evalCalibrateTimeout bounds POST /eval/calibrate. The server spends one
// judge call per resolved checkpoint, and a CLI-backed provider takes several
// seconds or more per call, so a default 30-day window runs for minutes — past
// the 120s every other command is bounded by.
const evalCalibrateTimeout = 30 * time.Minute

// evalCalibrateClient is the client calibrate posts with: c, with the longer
// bound.
func evalCalibrateClient(c *client.Client) *client.Client {
	return c.WithTimeout(evalCalibrateTimeout)
}

func evalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "eval",
		Short: "Measure the judge against what humans decided",
	}
	cmd.AddCommand(evalCalibrateCmd())
	return cmd
}

func evalCalibrateCmd() *cobra.Command {
	var (
		twin     string
		since    time.Duration
		limit    int
		export   string
		markdown bool
	)
	cmd := &cobra.Command{
		Use:   "calibrate",
		Short: "Score the judge against resolved checkpoints",
		Long: `Asks the judge the loop's PASS/FAIL question about every plan a human already
approved, rejected or modified in the window, and reports how often the two
agree. It spends one model call per checkpoint and writes nothing back.`,
		Example: `  krk eval calibrate --since 720h --markdown
  krk eval calibrate --twin t_7f2a --since 72h --limit 50 --export golden.json`,
		RunE: func(c *cobra.Command, _ []string) error {
			now := time.Now().UTC()
			body := map[string]any{}
			if twin != "" {
				body["twin"] = twin
			}
			if since > 0 {
				body["since"] = now.Add(-since).Format(time.RFC3339)
			}
			if limit > 0 {
				body["limit"] = limit
			}
			// Ctrl-C drops the request, and the server stops judging when it
			// sees the caller go.
			ctx, stop := signal.NotifyContext(c.Context(), os.Interrupt)
			defer stop()
			data, _, err := evalCalibrateClient(api).PostContext(ctx, "/eval/calibrate", body)
			if err != nil {
				return err
			}
			var rep evalReport
			if err := json.Unmarshal(data, &rep); err != nil {
				return fmt.Errorf("decode calibration report: %w", err)
			}

			if export != "" {
				entries := exportGolden(rep, deploymentName(api.BaseURL), now)
				raw, err := json.MarshalIndent(entries, "", "  ")
				if err != nil {
					return err
				}
				if err := os.WriteFile(export, append(raw, '\n'), 0o644); err != nil {
					return fmt.Errorf("write %s: %w", export, err)
				}
			}

			w := c.OutOrStdout()
			switch {
			case markdown:
				writeCalibrationMarkdown(w, rep, now)
			case output == "json":
				client.PrintOutput(data, output)
			case output == "quiet":
			default:
				writeCalibrationSummary(w, rep)
			}
			if export != "" && output != "quiet" {
				fmt.Fprintf(c.ErrOrStderr(), "wrote %s\n", export)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&twin, "twin", "", "only this twin's checkpoints")
	cmd.Flags().DurationVar(&since, "since", 30*24*time.Hour, "how far back to read resolved checkpoints")
	cmd.Flags().IntVar(&limit, "limit", 0, "judge at most N checkpoints")
	cmd.Flags().StringVar(&export, "export", "", "write the judged items to FILE as golden entries")
	cmd.Flags().BoolVar(&markdown, "markdown", false, "print the report as markdown")
	return cmd
}

// The human decisions a report breaks agreement down by, in the order and with
// the names docs/benchmarks.md reads them.
var evalDecisions = []struct{ choice, kind string }{
	{"approve", "approval"},
	{"reject", "rejection"},
	{"modify", "modification"},
}

// exportGolden mirrors eval.ExportGolden, which the CLI may not import: one
// entry per item the judge answered, with provenance exported:<deployment>:<date>.
func exportGolden(rep evalReport, deployment string, at time.Time) []goldenEntry {
	provenance := fmt.Sprintf("exported:%s:%s", deployment, at.Format("2006-01-02"))
	out := []goldenEntry{}
	for _, it := range rep.Items {
		if it.Reply == "" {
			continue
		}
		out = append(out, goldenEntry{
			ID:         fmt.Sprintf("%s:%s", deployment, it.CheckpointID),
			Title:      it.Title,
			Criterion:  it.Criterion,
			Actions:    it.Actions,
			Label:      it.Choice,
			Reply:      it.Reply,
			Provenance: provenance,
		})
	}
	return out
}

// deploymentName is the server's host, which is what names a deployment in a
// golden entry's id and provenance.
func deploymentName(baseURL string) string {
	if u, err := url.Parse(baseURL); err == nil && u.Host != "" {
		return u.Host
	}
	return baseURL
}

func judgeErrors(rep evalReport) int {
	n := 0
	for _, it := range rep.Items {
		if it.Error != "" {
			n++
		}
	}
	return n
}

// calibrationWindow renders the window the report covers, or "all history"
// when neither end was bounded.
func calibrationWindow(rep evalReport) string {
	if rep.Since.IsZero() && rep.Until.IsZero() {
		return "all history"
	}
	from, to := "the start", "now"
	if !rep.Since.IsZero() {
		from = rep.Since.UTC().Format("2006-01-02")
	}
	if !rep.Until.IsZero() {
		to = rep.Until.UTC().Format("2006-01-02")
	}
	return from + " – " + to
}

func percent(agreed, n int) string {
	if n == 0 {
		return "—"
	}
	return fmt.Sprintf("%.1f%%", 100*float64(agreed)/float64(n))
}

func calibrationScope(rep evalReport) string {
	if rep.TwinID == "" {
		return "every twin"
	}
	return "twin " + rep.TwinID
}

func writeCalibrationSummary(w io.Writer, rep evalReport) {
	c := rep.Confusion
	fmt.Fprintf(w, "Judge calibration over %s, %s\n", calibrationScope(rep), calibrationWindow(rep))
	fmt.Fprintf(w, "  agreement      %s (%d of %d)\n", percent(rep.Agreed, rep.N), rep.Agreed, rep.N)
	fmt.Fprintf(w, "  human approve  judge pass %d, fail %d\n", c.JudgePassHumanApprove, c.JudgeFailHumanApprove)
	fmt.Fprintf(w, "  human reject   judge pass %d, fail %d\n", c.JudgePassHumanReject, c.JudgeFailHumanReject)
	fmt.Fprintf(w, "  skipped        %d\n", rep.Skipped)
	if n := judgeErrors(rep); n > 0 {
		fmt.Fprintf(w, "  judge errors   %d (scored as fail)\n", n)
	}
	fmt.Fprintf(w, "  replayable     %d\n", rep.Replayable)
}

// writeCalibrationMarkdown prints the section docs/benchmarks.md carries.
func writeCalibrationMarkdown(w io.Writer, rep evalReport, today time.Time) {
	c := rep.Confusion
	fmt.Fprintf(w, "## Real-history judge calibration\n\n")
	fmt.Fprintf(w, "Measured %s over %s, window: %s.\n\n",
		today.Format("2006-01-02"), calibrationScope(rep), calibrationWindow(rep))
	fmt.Fprintf(w, "- N (checkpoints judged): %d\n", rep.N)
	fmt.Fprintf(w, "- Agreement with the human decision: %s (%d of %d)\n", percent(rep.Agreed, rep.N), rep.Agreed, rep.N)
	fmt.Fprintf(w, "- Skipped (no usable label or objective): %d\n", rep.Skipped)
	if n := judgeErrors(rep); n > 0 {
		fmt.Fprintf(w, "- Judge errors (scored as FAIL): %d\n", n)
	}
	fmt.Fprintf(w, "- Replayable (recorded world state): %d\n\n", rep.Replayable)

	fmt.Fprintf(w, "Confusion matrix:\n\n")
	fmt.Fprintf(w, "| | Judge PASS | Judge FAIL |\n|---|---:|---:|\n")
	fmt.Fprintf(w, "| Human approve | %d | %d |\n", c.JudgePassHumanApprove, c.JudgeFailHumanApprove)
	fmt.Fprintf(w, "| Human reject or modify | %d | %d |\n\n", c.JudgePassHumanReject, c.JudgeFailHumanReject)

	fmt.Fprintf(w, "By human decision:\n\n")
	fmt.Fprintf(w, "| Kind | N | Agreed | Agreement | Judge PASS |\n|---|---:|---:|---:|---:|\n")
	for _, d := range evalDecisions {
		s := rep.ByDecision[d.choice]
		fmt.Fprintf(w, "| %s | %d | %d | %s | %d |\n", d.kind, s.N, s.Agreed, percent(s.Agreed, s.N), s.JudgePass)
	}
}
