package digest

import (
	"time"

	"github.com/bsenel/karakuri/internal/core/objective"
)

// Schedule is a standing instruction to report: who to tell, how often, and
// through which adapter.
//
// It is keyed on a twin rather than on an objective, deliberately. The reader
// wants one morning brief covering everything they are responsible for, not
// one mail per objective — a CTO twin holding nine standing objectives should
// produce one message a day, not nine.
type Schedule struct {
	ID     string `json:"id"`
	TwinID string `json:"twin_id"`

	// Cadence reuses the objective cadence type, and only its reconcile half
	// means anything here: `every`, `cron` or `daily_at` with a timezone,
	// plus quiet windows. There is no cheap tier to report on — assembling a
	// digest is a handful of queries, so there is nothing to defer.
	Cadence objective.Cadence `json:"cadence"`

	// Channel is the adapter slot to deliver through: "messaging", "email",
	// "projectmgmt" or "versioncontrol". Instance is the named instance
	// within that slot, empty for the twin's bound default (ADR 006).
	Channel  string `json:"channel"`
	Instance string `json:"instance,omitempty"`

	// Target is the address within the channel — a Slack channel, an email
	// recipient, a repository. What it means is the adapter's business.
	Target string `json:"target"`

	// Window is how far back a digest looks. Empty means "since the last one
	// was sent", which is what almost everybody wants and what makes a
	// missed run catch up rather than lose a day.
	Window string `json:"window,omitempty"`

	// SendWhenEmpty forces delivery even when nothing happened. Off by
	// default: a daily mail that says "nothing happened" is a mail people
	// stop reading, which costs the ones that matter their audience.
	SendWhenEmpty bool `json:"send_when_empty,omitempty"`

	Enabled bool `json:"enabled"`

	// Runtime, written by the sender and by nobody else.
	NextDueAt  *time.Time `json:"next_due_at,omitempty"`
	LastSentAt *time.Time `json:"last_sent_at,omitempty"`
	LastError  string     `json:"last_error,omitempty"`

	// ConsecutiveFailures grows the gap between retries after a failed send.
	// Without it a schedule pointed at a misconfigured channel retries on
	// every tick forever — the failure path never advances LastSentAt, so the
	// schedule reads as never-run and is therefore always due. A daily report
	// then becomes 1,440 failed deliveries and 1,440 audit rows a day.
	ConsecutiveFailures int `json:"consecutive_failures,omitempty"`

	// The lease, exactly as reconcile_states carries one and for exactly the
	// same reason: two replicas sending the same morning report to the same
	// person is the failure this feature would otherwise introduce.
	Holder     string     `json:"holder,omitempty"`
	LeaseUntil *time.Time `json:"lease_until,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// WindowDuration parses Window, returning zero when it is unset or malformed.
// Zero means "since the last delivery", which the sender resolves against
// LastSentAt.
func (s Schedule) WindowDuration() time.Duration {
	if s.Window == "" {
		return 0
	}
	d, err := time.ParseDuration(s.Window)
	if err != nil || d <= 0 {
		return 0
	}
	return d
}

// Since resolves the start of the window this delivery should cover.
//
// From the last delivery rather than from a fixed offset, so a sender that was
// down for a day sends one digest covering two days instead of silently losing
// one. The fallback for a schedule that has never sent is the declared window,
// or a day.
func (s Schedule) Since(now time.Time) time.Time {
	if d := s.WindowDuration(); d > 0 {
		return now.Add(-d)
	}
	if s.LastSentAt != nil {
		return *s.LastSentAt
	}
	return now.Add(-24 * time.Hour)
}

// Channel slots a schedule may deliver through. They are the adapter slots
// that can send something outward; the rest of the registry reads.
const (
	ChannelMessaging      = "messaging"
	ChannelEmail          = "email"
	ChannelProjectMgmt    = "projectmgmt"
	ChannelVersionControl = "versioncontrol"
)

// ValidChannel reports whether a slot can deliver a digest.
func ValidChannel(c string) bool {
	switch c {
	case ChannelMessaging, ChannelEmail, ChannelProjectMgmt, ChannelVersionControl:
		return true
	}
	return false
}
