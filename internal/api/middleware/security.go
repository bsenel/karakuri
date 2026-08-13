package middleware

import "net/http"

// SecurityHeaders sets conservative security response headers on every response.
// The SPA is same-origin and self-contained (no external scripts, styles or
// fonts — see web/embed.go), so a strict default-src 'self' CSP holds without
// breaking the app; 'unsafe-inline' is allowed for styles only because the Vite
// build inlines a small critical stylesheet. See SECURITY_AUDIT.md F-11.
//
// HSTS is intentionally omitted here: the server is expected to sit behind a
// TLS-terminating proxy, and emitting HSTS on a plaintext hop would be
// misleading. Terminate it at the proxy, or add it there.
func SecurityHeaders(next http.Handler) http.Handler {
	const csp = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data:; connect-src 'self'; font-src 'self' data:; " +
		"object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", csp)
		next.ServeHTTP(w, r)
	})
}

// MaxBytes caps the size of a request body a handler will read, guarding the
// JSON decoders against a memory-pressure DoS from an oversized payload. It
// wraps r.Body in http.MaxBytesReader, which makes Read fail once the limit is
// exceeded; handlers surface that as a 400. See SECURITY_AUDIT.md F-05.
func MaxBytes(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			next.ServeHTTP(w, r)
		})
	}
}
