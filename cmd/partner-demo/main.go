package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/partner"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/platform/logger"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := partner.LoadConfig()
	if err != nil {
		return err
	}

	log := logger.New(cfg.LogLevel, cfg.LogFormat, "partner-demo")
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	venue, err := partner.New(cfg, log)
	if err != nil {
		return err
	}
	defer func() {
		if err := venue.Shutdown(); err != nil {
			log.Warn("closing the platform connection", slog.Any("error", err))
		}
	}()

	log.Info("waiting for the platform",
		slog.String("addr", cfg.KitchenGRPCAddr),
		slog.Duration("timeout", cfg.StartupTimeout))
	if err := venue.WaitReady(ctx, cfg.StartupTimeout); err != nil {
		return err
	}

	if err := venue.Start(ctx, cfg.SyncMenu); err != nil {
		return err
	}

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           partner.NewHTTPHandler(venue, log),
		ReadHeaderTimeout: 10 * time.Second,
	}

	var wg sync.WaitGroup
	failures := make(chan error, 2)

	wg.Go(func() {
		log.Info("kitchen display api listening", slog.String("addr", cfg.HTTPAddr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			failures <- err
		}
	})

	wg.Go(func() {
		failures <- venue.ConsumeEvents(ctx)
	})

	wg.Go(func() {
		venue.RunRestocking(ctx)
	})

	var runErr error
	select {
	case <-ctx.Done():
	case runErr = <-failures:
	}

	closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	venue.Close(closeCtx)
	cancel()

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Warn("http shutdown", slog.Any("error", err))
	}

	stop()
	wg.Wait()
	log.Info("partner-demo stopped")
	return runErr
}
