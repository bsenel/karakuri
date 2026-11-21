package software

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/bsenel/karakuri/internal/core/capability"
	"github.com/bsenel/karakuri/internal/core/environment"
	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
)

// Phase 40 step 8: the loop's act step receives what cliEnv.Act returns, and
// it can only stop spending actions on a quota refusal it can tell apart from
// a failure. So the coding agent's rate limit reaches it as an error that is
// still ErrProviderRateLimited, with the provider and reset time, for the two
// capabilities delivery writes with; a wrap that flattens it fails here.
func TestCLIEnv_CodingAgentQuotaRefusalReachesTheActStepTyped(t *testing.T) {
	refusal := &coreerrors.ProviderRateLimitError{
		Provider:   "claude_code",
		RetryAfter: 3 * time.Hour,
		Cause:      errors.New("claude_code: exit 1 (stderr: ); last output: You've hit your 5-hour limit · resets 4:20pm (Europe/Paris)"),
	}
	for _, capID := range []string{"software.act.write_code", "software.act.write_test"} {
		t.Run(capID, func(t *testing.T) {
			env := &cliEnv{id: "software.env.cli_agent", cli: &funcCLI{t: t, run: func(*testing.T, string) error {
				return fmt.Errorf("delegate: %w", refusal)
			}}}

			res, err := env.Act(context.Background(), environment.Action{
				CapabilityID: capability.CapabilityID(capID),
				Params: map[string]any{
					"prompt":        "implement the step",
					"allowed_tools": []any{"Read", "Edit"},
					"worktree_path": t.TempDir(),
				},
			})

			if !errors.Is(err, coreerrors.ErrProviderRateLimited) {
				t.Fatalf("Act error %v is not ErrProviderRateLimited", err)
			}
			var limit *coreerrors.ProviderRateLimitError
			if !errors.As(err, &limit) || limit.Provider != "claude_code" || limit.RetryAfter != 3*time.Hour {
				t.Errorf("Act error %v lost the provider or the reset time", err)
			}
			if res.Success || res.Error == "" {
				t.Errorf("result = %+v, want a failed result that says why", res)
			}
			if res.StateDelta["adapter"] != "func" {
				t.Errorf("StateDelta = %v, want the adapter named beside the error", res.StateDelta)
			}
		})
	}
}

// Any other failure of the coding agent stays a failed result with no error,
// as it was before step 8.
func TestCLIEnv_OtherCodingAgentFailureIsAFailedResultAsToday(t *testing.T) {
	res := actWith(t, t.TempDir(), salvageAction(t.TempDir()), func(*testing.T, string) error { return errExit1 })

	if res.Success || res.Error != errExit1.Error() {
		t.Errorf("result = %+v, want the failure as today", res)
	}
}
