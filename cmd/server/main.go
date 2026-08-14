package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/bsenel/karakuri/internal/app"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	boot, err := app.BootstrapServer(app.ConfigPath())
	if err != nil {
		slog.Error("bootstrap failed", "err", err)
		os.Exit(1)
	}
	addr := boot.Config.Server.Addr
	slog.Info("karakuri server starting", "addr", addr)
	// Timeouts guard against slowloris and stuck connections. WriteTimeout is
	// deliberately left at zero: the SSE endpoints (GET /events and friends)
	// stream indefinitely, and a write deadline would truncate them. Read and
	// header deadlines bound how long a client may take to send a request, which
	// is where the slow-connection DoS lives. See SECURITY_AUDIT.md F-04.
	srv := &http.Server{
		Addr:              addr,
		Handler:           boot.App.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("server failed", "err", err)
		os.Exit(1)
	}
}
