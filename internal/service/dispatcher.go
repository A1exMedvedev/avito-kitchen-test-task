package service

import (
	"context"
	"log/slog"
	"time"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"github.com/google/uuid"
)

type outboxDispatcher struct {
	outbox    OutboxRepository
	orders    OrderRepository
	publisher EventPublisher
	logger    *slog.Logger

	interval  time.Duration
	batchSize int

	wakeup chan struct{}
}

type DispatcherOptions struct {
	Interval  time.Duration
	BatchSize int
}

func NewOutboxDispatcher(
	outbox OutboxRepository,
	orders OrderRepository,
	publisher EventPublisher,
	logger *slog.Logger,
	opts DispatcherOptions,
) OutboxDispatcher {
	if opts.Interval <= 0 {
		opts.Interval = time.Second
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = 100
	}
	return &outboxDispatcher{
		outbox:    outbox,
		orders:    orders,
		publisher: publisher,
		logger:    logger,
		interval:  opts.Interval,
		batchSize: opts.BatchSize,
		wakeup:    make(chan struct{}, 1),
	}
}

func (d *outboxDispatcher) Notify() {
	select {
	case d.wakeup <- struct{}{}:
	default:
	}
}

func (d *outboxDispatcher) Run(ctx context.Context) error {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		case <-d.wakeup:
		}

		if err := d.drain(ctx); err != nil && ctx.Err() == nil {

			d.logger.Error("outbox drain failed", slog.Any("error", err))
		}
	}
}

func (d *outboxDispatcher) drain(ctx context.Context) error {
	for {
		events, err := d.outbox.FetchUnpublished(ctx, d.batchSize)
		if err != nil {
			return err
		}
		if len(events) == 0 {
			return nil
		}

		published := make([]uuid.UUID, 0, len(events))
		hydrated := make([]domain.OrderEvent, 0, len(events))
		for _, event := range events {

			order, err := d.orders.Get(ctx, event.OrderID)
			if err != nil {
				d.logger.Warn("skipping outbox event for unknown order",
					slog.String("order_id", event.OrderID.String()),
					slog.Any("error", err))
				published = append(published, event.ID)
				continue
			}
			event.Order = order
			hydrated = append(hydrated, event)
			published = append(published, event.ID)
		}

		d.publisher.Publish(ctx, hydrated...)
		if err := d.outbox.MarkPublished(ctx, published); err != nil {
			return err
		}
		if len(events) < d.batchSize {
			return nil
		}
	}
}
