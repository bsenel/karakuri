package cliagent

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	coreerrors "github.com/bsenel/karakuri/internal/core/errors"
)

// claudeExitError is the Claude Code CLI exiting non-zero. It is a type of its
// own so Delegate looks for a quota refusal only there, never in a failure to
// start or a configuration error; the wording is the one the audit log has
// always carried.
type claudeExitError struct {
	code   int
	stderr string
}

func (e *claudeExitError) Error() string {
	return fmt.Sprintf("claude_code: exit %d (stderr: %s)", e.code, e.stderr)
}

// claudePlanLimit is the CLI's plan-limit refusal, "you've hit your <window>
// limit · resets <time>". It copies planLimit in internal/platform/llm/claude.go,
// which this package does not import; change both together. The reset is what
// makes it a quota refusal: a limit that does not reset is a real failure.
var claudePlanLimit = regexp.MustCompile(`(?i)hit your [^.]{0,40}\blimit\b.*\bresets\b`)

// claudeReset reads the reset in the form recorded on 2026-11-14, "resets
// 4:20pm (Europe/Paris)" or "resets 9pm (Europe/Paris)". Any other form gives
// no time.
var claudeReset = regexp.MustCompile(`(?i)\bresets\s+(\d{1,2})(?::(\d{2}))?\s*(am|pm)\s*\(([^()\s]+)\)`)

// classifyClaudeExit names a non-zero exit whose stderr or last output holds
// the CLI's plan-limit refusal as the domain's rate limit, wrapping err so its
// message, with the last output, is kept. Any other err is returned unchanged.
func classifyClaudeExit(err error, now time.Time) error {
	var exit *claudeExitError
	if !errors.As(err, &exit) {
		return err
	}
	// err's text is the stderr and the output tail withOutputTail added.
	text := err.Error()
	if !claudePlanLimit.MatchString(text) {
		return err
	}
	return &coreerrors.ProviderRateLimitError{
		Provider:   "claude_code",
		RetryAfter: untilClaudeReset(text, now),
		Cause:      err,
	}
}

// untilClaudeReset is how long from now until the next occurrence of the reset
// time text names, in the time zone it names; 0 when it cannot be read.
func untilClaudeReset(text string, now time.Time) time.Duration {
	m := claudeReset.FindStringSubmatch(text)
	if m == nil {
		return 0
	}
	hour, err := strconv.Atoi(m[1])
	if err != nil || hour < 1 || hour > 12 {
		return 0
	}
	minute := 0
	if m[2] != "" {
		if minute, err = strconv.Atoi(m[2]); err != nil || minute > 59 {
			return 0
		}
	}
	hour %= 12
	if strings.EqualFold(m[3], "pm") {
		hour += 12
	}
	loc, err := time.LoadLocation(m[4])
	if err != nil {
		return 0
	}
	local := now.In(loc)
	reset := time.Date(local.Year(), local.Month(), local.Day(), hour, minute, 0, 0, loc)
	if !reset.After(now) {
		reset = time.Date(local.Year(), local.Month(), local.Day()+1, hour, minute, 0, 0, loc)
	}
	return reset.Sub(now)
}
