package auth

import (
	"strings"
	"testing"
)

func TestSanitizeLogValue(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"ordinary id passes through", "alice", "alice"},
		{"non-ascii is not mangled", "アリス-ünïcode", "アリス-ünïcode"},
		{"empty", "", ""},
		// The attack this exists for: a forged record in the middle of the log.
		{"lf forges a record", "alice\nlevel=INFO msg=\"authorization granted\"", "alice level=INFO msg=\"authorization granted\""},
		{"crlf forges a record", "alice\r\nlevel=INFO", "alice level=INFO"},
		{"bare cr overwrites a line", "alice\rroot", "alice root"},
		{"control characters are dropped", "al\x00i\x07c\x1be\x7f", "alice"},
		{"tab is a control character too", "a\tb", "ab"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := SanitizeLogValue(tc.in); got != tc.want {
				t.Errorf("SanitizeLogValue(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSanitizeLogValueTruncates(t *testing.T) {
	// One oversized field must not be able to bury the records around it.
	got := SanitizeLogValue(strings.Repeat("a", maxLogValueLen*3))
	if want := strings.Repeat("a", maxLogValueLen) + "…"; got != want {
		t.Errorf("long value not truncated: got %d chars", len([]rune(got)))
	}

	// Truncation counts runes, so it can never split one in half and emit
	// invalid UTF-8 into the log.
	multibyte := SanitizeLogValue(strings.Repeat("あ", maxLogValueLen+10))
	if runes := []rune(multibyte); len(runes) != maxLogValueLen+1 {
		t.Errorf("multibyte truncation = %d runes, want %d", len(runes), maxLogValueLen+1)
	}
	if !strings.HasSuffix(multibyte, "…") || strings.Contains(multibyte, "�") {
		t.Errorf("multibyte value damaged by truncation: %q", multibyte)
	}
}
