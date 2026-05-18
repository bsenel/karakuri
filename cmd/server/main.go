package main

import (
	"log/slog"
	"net/http"
	"os"

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
	if err := http.ListenAndServe(addr, boot.App.Handler()); err != nil {
		slog.Error("server failed", "err", err)
		os.Exit(1)
	}
}
