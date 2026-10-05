package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/bsenel/karakuri/internal/app"
)

// shutdownGrace bounds how long a stopping server waits for requests in
// flight. The SSE streams never finish on their own, so without a bound a
// shutdown would wait on them forever.
const shutdownGrace = 10 * time.Second

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

	stop, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	served := make(chan error, 1)
	go func() { served <- srv.ListenAndServe() }()

	select {
	case err := <-served:
		slog.Error("server failed", "err", err)
		boot.Close()
		os.Exit(1)
	case <-stop.Done():
	}

	// Stop taking requests first, then release the MCP servers, so no tool
	// call is cut off halfway through by its subprocess going away.
	slog.Info("karakuri server stopping")
	ctx, done := context.WithTimeout(context.Background(), shutdownGrace)
	defer done()
	if err := srv.Shutdown(ctx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		slog.Warn("server shutdown", "err", err)
	}
	boot.Close()
	slog.Info("karakuri server stopped")
}
