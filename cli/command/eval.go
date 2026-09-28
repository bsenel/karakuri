package command

import (
	"errors"
	"time"

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
		RunE: func(_ *cobra.Command, _ []string) error {
			return errors.New("not implemented")
		},
	}
	cmd.Flags().StringVar(&twin, "twin", "", "only this twin's checkpoints")
	cmd.Flags().DurationVar(&since, "since", 30*24*time.Hour, "how far back to read resolved checkpoints")
	cmd.Flags().IntVar(&limit, "limit", 0, "judge at most N checkpoints")
	cmd.Flags().StringVar(&export, "export", "", "write the judged items to FILE as golden entries")
	cmd.Flags().BoolVar(&markdown, "markdown", false, "print the report as markdown")
	return cmd
}
