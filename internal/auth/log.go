package auth

import "strings"

// maxLogValueLen bounds a single interpolated field. Long enough for any real
// identifier, short enough that one request cannot bury the surrounding records.
const maxLogValueLen = 128

// SanitizeLogValue makes a request-derived string safe to write into a log
// record.
//
// A principal identifier arrives from the client — it is carried in a token we
// issued, but it originated in whatever an administrator typed and reaches this
// process over the wire. A value containing a newline can forge an entire log
// entry: "authorization granted" lines that no code ever wrote, sitting in the
// middle of the audit trail. Control characters can do the same to a terminal
// tailing the log.
//
// So: line breaks and other control characters are dropped and the result is
// truncated. Callers should wrap anything attacker-influenced before logging it,
// and only then.
func SanitizeLogValue(s string) string {
	// Explicit newline removal first — this is the property that matters, and
	// stating it plainly beats hiding it inside the general filter below.
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
	if runes := []rune(s); len(runes) > maxLogValueLen {
		return string(runes[:maxLogValueLen]) + "…"
	}
	return s
}
