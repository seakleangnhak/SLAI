package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/slai/slai/services/api/internal/auth"
	"github.com/slai/slai/services/api/internal/config"
	"github.com/slai/slai/services/api/internal/ledger"
	platformdb "github.com/slai/slai/services/api/internal/platform/db"
	httpserver "github.com/slai/slai/services/api/internal/platform/http"
	"github.com/slai/slai/services/api/internal/users"
)

func main() {
	if err := run(); err != nil {
		slog.Error("slai-api exited with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := newLogger(cfg)
	slog.SetDefault(logger)

	command := "serve"
	if len(os.Args) > 1 {
		command = strings.ToLower(os.Args[1])
	}

	switch command {
	case "serve", "server":
		return serve(cfg, logger)
	case "migrate":
		direction := "up"
		if len(os.Args) > 2 {
			direction = strings.ToLower(os.Args[2])
		}
		if direction != "up" {
			return fmt.Errorf("unsupported migration direction %q; only 'up' is implemented", direction)
		}
		return migrateUp(cfg, logger)
	case "seed-admin":
		return seedAdmin(cfg, logger)
	case "help", "-h", "--help":
		fmt.Println("Usage: slai-api [serve|migrate up|seed-admin]")
		return nil
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

func serve(cfg config.Config, logger *slog.Logger) error {
	ctx := context.Background()
	pool, err := platformdb.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer pool.Close()

	server := httpserver.NewServer(httpserver.ServerConfig{
		Addr:             cfg.HTTPAddr,
		ReadinessTimeout: cfg.ReadinessTimeout,
		SessionSecret:    cfg.SessionSecret,
		CookieSecure:     cfg.CookieSecure,
		SessionTTL:       cfg.SessionTTL,
		APIKeyPepper:     cfg.APIKeyPepper,
		APIKeyPrefix:     cfg.APIKeyPrefix,
		OmniRoute:        cfg.OmniRoute,
	}, pool, logger)

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server starting", "addr", cfg.HTTPAddr, "env", cfg.AppEnv)
		errCh <- server.ListenAndServe()
	}()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-signalCh:
		logger.Info("shutdown signal received", "signal", sig.String())
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown server: %w", err)
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("http server: %w", err)
	}
}

func seedAdmin(cfg config.Config, logger *slog.Logger) error {
	if cfg.AdminSeedEmail == "" || cfg.AdminSeedPassword == "" {
		return fmt.Errorf("ADMIN_SEED_EMAIL and ADMIN_SEED_PASSWORD are required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := platformdb.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer pool.Close()

	passwordHash, err := auth.HashPassword(cfg.AdminSeedPassword)
	if err != nil {
		return err
	}

	return platformdb.InTx(ctx, pool, func(tx pgx.Tx) error {
		repo := users.NewRepository(tx)
		existing, err := repo.GetByEmail(ctx, cfg.AdminSeedEmail)
		if err == nil {
			if existing.Role != users.RoleAdmin {
				return fmt.Errorf("user %s already exists and is not an admin", existing.Email)
			}
			logger.Info("admin seed already exists", "email", existing.Email)
			return nil
		}
		if !errors.Is(err, users.ErrNotFound) {
			return err
		}

		created, err := repo.Create(ctx, cfg.AdminSeedEmail, passwordHash, users.RoleAdmin)
		if err != nil {
			return err
		}
		if err := ledger.NewService(tx).EnsureBalance(ctx, created.ID); err != nil {
			return err
		}
		logger.Info("admin seed created", "email", created.Email)
		return nil
	})
}

func migrateUp(cfg config.Config, logger *slog.Logger) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := platformdb.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer pool.Close()

	runner := platformdb.NewMigrator(pool, cfg.MigrationsDir, logger)
	return runner.Up(ctx)
}

func newLogger(cfg config.Config) *slog.Logger {
	level := slog.LevelInfo
	switch strings.ToLower(cfg.LogLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
}
