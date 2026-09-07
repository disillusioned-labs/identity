// Command grpc is the gRPC server entry point: load configuration, hand it to
// the app, and turn a failure into an exit code. Wiring lives in internal/app.
package main

import (
	"log/slog"
	"os"

	"github.com/disillusioned-labs/identity/internal/app"
	"github.com/disillusioned-labs/identity/internal/config"
)

func main() {
	// Loaded here, not inside RunGRPC, so RunGRPC stays callable with a
	// config built in code.
	cfg, err := config.Load()
	if err != nil {
		// The real logger is built from cfg, so this failure has to report
		// through the default one.
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	cfg.Service.Name += "-grpc"

	if err := app.RunGRPC(cfg); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}
