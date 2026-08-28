package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/app"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/config"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/platform/logger"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := logger.New(cfg.Log.Level, cfg.Log.Format, "kitchen")
	slog.SetDefault(log)
	log.Info("starting",
		slog.String("env", cfg.Env),
		slog.String("version", cfg.Version))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	application, err := app.New(ctx, cfg, log)
	if err != nil {
		return err
	}
	return application.Run(ctx)
}
