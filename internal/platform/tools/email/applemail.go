package email

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// AppleMail is a macOS-only EmailAdapter that drives Mail.app via AppleScript.
// Send composes a new message in Mail.app and sends it through the default
// account (or a specific account if FromAddress matches one).
// List is not supported — Mail.app's stored mbox/EML files are accessible but
// outside the scope of v1; use IMAP via the SMTP adapter or an OAuth provider.
type AppleMail struct {
	fromAddress string
}

func NewAppleMail(fromAddress string) *AppleMail {
	return &AppleMail{fromAddress: fromAddress}
}

func (a *AppleMail) Name() string { return "apple_mail" }

func (a *AppleMail) Active() bool { return runtime.GOOS == "darwin" }

func (a *AppleMail) Send(ctx context.Context, msg Message) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("apple_mail: only supported on macOS (current GOOS=%s)", runtime.GOOS)
	}
	if len(msg.To) == 0 {
		return "", fmt.Errorf("apple_mail: at least one To address required")
	}

	from := msg.From
	if from == "" {
		from = a.fromAddress
	}

	script := buildAppleScriptSend(from, msg)
	cmd := exec.CommandContext(ctx, "osascript", "-")
	cmd.Stdin = strings.NewReader(script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("apple_mail: osascript: %w (output: %s)", err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

func (a *AppleMail) List(_ context.Context, _ string, _ int) ([]Message, error) {
	return nil, fmt.Errorf("apple_mail: List is not supported; use IMAP via SMTP or an OAuth provider")
}

// buildAppleScriptSend constructs an AppleScript that creates and sends a new
// outgoing message. AppleScript strings are quoted carefully to avoid injection.
func buildAppleScriptSend(from string, msg Message) string {
	var sb strings.Builder
	sb.WriteString("tell application \"Mail\"\n")
	sb.WriteString("\tset newMessage to make new outgoing message with properties {")
	fmt.Fprintf(&sb, "subject:%q, content:%q, visible:false", msg.Subject, msg.Body)
	if from != "" {
		fmt.Fprintf(&sb, ", sender:%q", from)
	}
	sb.WriteString("}\n")
	sb.WriteString("\ttell newMessage\n")
	for _, to := range msg.To {
		fmt.Fprintf(&sb, "\t\tmake new to recipient at end of to recipients with properties {address:%q}\n", to)
	}
	for _, cc := range msg.Cc {
		fmt.Fprintf(&sb, "\t\tmake new cc recipient at end of cc recipients with properties {address:%q}\n", cc)
	}
	sb.WriteString("\t\tsend\n")
	sb.WriteString("\tend tell\n")
	sb.WriteString("end tell\n")
	return sb.String()
}
