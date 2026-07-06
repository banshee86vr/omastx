package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/banshee86vr/omastx/backend/internal/api"
	"github.com/banshee86vr/omastx/backend/internal/cluster"
	"github.com/banshee86vr/omastx/backend/internal/config"
	"github.com/banshee86vr/omastx/backend/internal/registryauth"
	"github.com/banshee86vr/omastx/backend/internal/scan"
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

	if err := bootstrapAdmin(ctx, queries, cfg, logger); err != nil {
		return err
	}

	scanner := scan.NewManager(scan.Config{
		Store:        queries,
		RegistryAuth: registryauth.DBStore{Q: queries, MasterKey: cfg.MasterKey},
		Providers:    scan.DefaultProviders(),
		Resolvers:    scan.DefaultResolvers(queries, 0),
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
		SecureCookies: cfg.SecureCookies,
		MasterKey:     cfg.MasterKey,
		DevMode:       cfg.DevMode,
		DevLoginEmail: cfg.AdminEmail,
		Connector:     &cluster.KubeConnector{},
		Scanner:       scanner,
		Scheduler:     scheduler,
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

// bootstrapAdmin creates the initial admin user from env on first run (idempotent).
func bootstrapAdmin(ctx context.Context, queries *db.Queries, cfg config.Config, logger *slog.Logger) error {
	count, err := queries.CountUsers(ctx)
	if err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if count > 0 {
		return nil
	}
	if cfg.AdminEmail == "" || cfg.AdminPassword == "" {
		return fmt.Errorf("no users exist and OMASTX_ADMIN_EMAIL / OMASTX_ADMIN_PASSWORD are not set; set both so the first admin can be created")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(cfg.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = queries.CreateUser(ctx, db.CreateUserParams{
		Email:        strings.ToLower(strings.TrimSpace(cfg.AdminEmail)),
		PasswordHash: string(hash),
		Role:         "admin",
	})
	if err != nil {
		return fmt.Errorf("create admin user: %w", err)
	}
	logger.Info("admin user created", "email", cfg.AdminEmail)
	return nil
}
