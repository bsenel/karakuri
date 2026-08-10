package middleware

import (
	"log/slog"
	"net/http"
	"time"

	karakuriauth "github.com/bsenel/karakuri/internal/auth"
)

func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		// The path is whatever the client asked for, so it is sanitized before
		// it reaches a log record: a request for "/x%0alevel=INFO msg=..." would
		// otherwise write a second, fabricated line into the log stream.
		slog.Info("request",
			"method", r.Method,
			"path", karakuriauth.SanitizeLogValue(r.URL.Path),
			"duration", time.Since(start))
	})
}
