package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/banshee86vr/omastx/backend/internal/config"
	"github.com/banshee86vr/omastx/backend/internal/seed"
	"github.com/banshee86vr/omastx/backend/internal/store"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("seed failed", "error", err)
		os.Exit(1)
	}
	logger.Info("seed applied", "clusters", 3)
}

func run(logger *slog.Logger) error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}
	if err := store.Migrate(cfg.DatabaseURL); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	ctx := context.Background()
	pool, err := store.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := seed.Apply(ctx, pool, cfg.MasterKey); err != nil {
		return err
	}
	logger.Info("demo fleet ready", "hint", "sign in via dev-login (OMASTX_DEV) or GitHub OAuth")
	return nil
}
