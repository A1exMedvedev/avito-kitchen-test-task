package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	nethttp "net/http"
	"time"

	kitchenv1 "backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/api/gen/kitchen/v1"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/config"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/platform/eventbus"
	grpcpresentation "backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/presentation/grpc"
	httppresentation "backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/presentation/http"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/repository/postgres"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
)

type App interface {
	Run(ctx context.Context) error
}

type app struct {
	cfg    config.Config
	logger *slog.Logger

	pool       *pgxpool.Pool
	bus        eventbus.Bus
	dispatcher service.OutboxDispatcher

	httpServer *nethttp.Server
	grpcServer *grpc.Server
}

func New(ctx context.Context, cfg config.Config, logger *slog.Logger) (App, error) {
	pool, err := postgres.NewPool(ctx, postgres.PoolConfig{
		DSN:             cfg.Database.DSN,
		MaxConns:        cfg.Database.MaxConns,
		MinConns:        cfg.Database.MinConns,
		MaxConnLifetime: cfg.Database.MaxConnLifetime,
		ConnectTimeout:  cfg.Database.ConnectTimeout,
	})
	if err != nil {
		return nil, err
	}

	if cfg.Database.AutoMigrate {
		if err := postgres.Migrate(ctx, pool); err != nil {
			pool.Close()
			return nil, err
		}
		logger.Info("database migrations applied")
	}

	txManager := postgres.NewTxManager(pool)
	venues := postgres.NewVenueRepository(pool)
	menus := postgres.NewMenuRepository(pool)
	carts := postgres.NewCartRepository(pool)
	orders := postgres.NewOrderRepository(pool)
	customers := postgres.NewCustomerRepository(pool)
	outbox := postgres.NewOutboxRepository(pool, logger)

	bus := eventbus.New(logger)

	clock := service.Clock(service.SystemClock)
	dispatcher := service.NewOutboxDispatcher(outbox, orders, bus, logger, service.DispatcherOptions{
		Interval:  cfg.Outbox.PollInterval,
		BatchSize: cfg.Outbox.BatchSize,
	})

	catalogService := service.NewCatalogService(venues, menus)
	cartService := service.NewCartService(carts, menus, venues, customers, clock)
	orderService := service.NewOrderService(
		txManager, orders, carts, menus, venues, customers, outbox, dispatcher, clock)
	fulfillmentService := service.NewFulfillmentService(
		txManager, orders, menus, outbox, dispatcher, service.Clock(service.SystemClock))
	partnerService := service.NewPartnerService(venues, menus, clock)

	httpHandler, err := httppresentation.NewRouter(
		httppresentation.NewServer(
			catalogService, cartService, orderService, fulfillmentService, partnerService),
		partnerService,
		httppresentation.RouterOptions{
			Logger:  logger,
			Version: cfg.Version,
			ReadinessCheck: func(r *nethttp.Request) error {
				return pool.Ping(r.Context())
			},
		})
	if err != nil {
		pool.Close()
		return nil, err
	}

	interceptors := grpcpresentation.NewInterceptors(partnerService, logger)
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(interceptors.Unary()),
		grpc.StreamInterceptor(interceptors.Stream()),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: cfg.GRPC.MaxConnectionIdle,

			Time:    2 * time.Minute,
			Timeout: 20 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             30 * time.Second,
			PermitWithoutStream: true,
		}),
	)
	kitchenv1.RegisterPartnerServiceServer(grpcServer,
		grpcpresentation.NewPartnerServer(partnerService, fulfillmentService, bus, logger))

	reflection.Register(grpcServer)

	return &app{
		cfg:        cfg,
		logger:     logger,
		pool:       pool,
		bus:        bus,
		dispatcher: dispatcher,
		httpServer: &nethttp.Server{
			Addr:              cfg.HTTP.Addr,
			Handler:           httpHandler,
			ReadHeaderTimeout: cfg.HTTP.ReadTimeout,
			ReadTimeout:       cfg.HTTP.ReadTimeout,
			WriteTimeout:      cfg.HTTP.WriteTimeout,
			IdleTimeout:       cfg.HTTP.IdleTimeout,
		},
		grpcServer: grpcServer,
	}, nil
}

func (a *app) Run(ctx context.Context) error {
	errs := make(chan error, 3)

	go func() {
		a.logger.Info("http server listening", slog.String("addr", a.cfg.HTTP.Addr))
		if err := a.httpServer.ListenAndServe(); err != nil &&
			!errors.Is(err, nethttp.ErrServerClosed) {
			errs <- fmt.Errorf("http server: %w", err)
			return
		}
		errs <- nil
	}()

	go func() {
		listener, err := listen(ctx, a.cfg.GRPC.Addr)
		if err != nil {
			errs <- fmt.Errorf("grpc listen: %w", err)
			return
		}
		a.logger.Info("grpc server listening", slog.String("addr", a.cfg.GRPC.Addr))
		if err := a.grpcServer.Serve(listener); err != nil {
			errs <- fmt.Errorf("grpc server: %w", err)
			return
		}
		errs <- nil
	}()

	go func() {
		a.logger.Info("outbox dispatcher started",
			slog.Duration("interval", a.cfg.Outbox.PollInterval))
		errs <- a.dispatcher.Run(ctx)
	}()

	select {
	case <-ctx.Done():
		return a.shutdown() //nolint:contextcheck
	case err := <-errs:
		if err == nil {
			return a.shutdown() //nolint:contextcheck
		}
		a.logger.Error("component failed, shutting down", slog.Any("error", err))
		if shutdownErr := a.shutdown(); shutdownErr != nil { //nolint:contextcheck
			a.logger.Error("shutdown after failure was not clean",
				slog.Any("error", shutdownErr))
		}
		return err
	}
}

func (a *app) shutdown() error {
	a.logger.Info("shutting down")

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(), a.cfg.HTTP.ShutdownTimeout)
	defer cancel()

	httpErr := a.httpServer.Shutdown(shutdownCtx)

	a.bus.Close()

	stopped := make(chan struct{})
	go func() {
		a.grpcServer.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-shutdownCtx.Done():
		a.logger.Warn("grpc graceful stop timed out, forcing")
		a.grpcServer.Stop()
	}

	a.pool.Close()

	if httpErr != nil {
		return fmt.Errorf("http shutdown: %w", httpErr)
	}
	a.logger.Info("shutdown complete")
	return nil
}
