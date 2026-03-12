package main

import (
	"context"
	"os"

	"github.com/taylor/dbgold/internal/app"
	"github.com/taylor/dbgold/internal/core"
	"github.com/taylor/dbgold/internal/execx"
)

func main() {
	ctx := context.Background()
	cfg := core.LoadConfigFromEnv()
	runner := execx.NewRunner()
	logger := core.NewLogger(os.Stderr, cfg.Debug)
	svc := core.NewService(cfg, runner, logger)

	cmd := app.NewRootCommand(ctx, svc)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
