package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/banshee86vr/omastx/backend/internal/api"
	"github.com/banshee86vr/omastx/backend/internal/cluster"
	"github.com/banshee86vr/omastx/backend/internal/config"
	"github.com/banshee86vr/omastx/backend/internal/registryauth"
	"github.com/banshee86vr/omastx/backend/internal/scan"
	"github.com/banshee86vr/omastx/backend/internal/seed"
	"github.com/banshee86vr/omastx/backend/internal/settings"
	"github.com/banshee86vr/omastx/backend/internal/store"
	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

func main() {
	migrateOnly := flag.Bool("migrate-only", false, "apply migrations and exit")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger, *migrateOnly); err != nil {
		logger.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger, migrateOnly bool) error {
	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}

	if err := store.Migrate(cfg.DatabaseURL); err != nil {
		return err
	}
	logger.Info("migrations applied")
	if migrateOnly {
		return nil
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := store.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	queries := db.New(pool)

	settingsLoader := settings.NewLoader(queries)
	if err := settingsLoader.Refresh(ctx); err != nil {
		logger.Warn("load app settings failed; using defaults", "error", err)
	}

	if cfg.DevMode {
		if err := seed.RepairKubeconfigs(ctx, pool, cfg.MasterKey); err != nil {
			logger.Warn("seed kubeconfig repair failed", "error", err)
		}
	}

	scanner := scan.NewManager(scan.Config{
		Store:        queries,
		RegistryAuth: registryauth.DBStore{Q: queries, MasterKey: cfg.MasterKey},
		Providers:    scan.DefaultProviders(),
		Resolvers:    scan.DefaultResolvers(queries, settingsLoader),
		Logger:       logger,
		MasterKey:    cfg.MasterKey,
	})
	scheduler := scan.NewScheduler(scanner, queries, logger)
	if err := scheduler.Reload(ctx); err != nil {
		logger.Warn("initial schedule load failed", "error", err)
	}
	defer scheduler.Stop()

	if cfg.DevMode {
		logger.Warn("OMASTX_DEV is enabled — using dev defaults and passwordless sign-in; never set this in production")
	}
	apiServer := api.NewServer(queries, logger, api.Options{
		SecureCookies:      cfg.SecureCookies,
		MasterKey:          cfg.MasterKey,
		DevMode:            cfg.DevMode,
		GitHubClientID:     cfg.GitHubClientID,
		GitHubClientSecret: cfg.GitHubClientSecret,
		GitHubOrg:          cfg.GitHubOrg,
		BaseURL:            cfg.BaseURL,
		Connector:          &cluster.KubeConnector{},
		Scanner:            scanner,
		Scheduler:          scheduler,
		SettingsLoader:     settingsLoader,
	})
	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           apiServer.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
