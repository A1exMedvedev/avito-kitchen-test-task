package postgres

import (
	"context"
	"fmt"
	"log/slog"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/repository/postgres/models"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/service"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type outboxRepository struct {
	base

	logger *slog.Logger
}

func NewOutboxRepository(pool *pgxpool.Pool, logger *slog.Logger) service.OutboxRepository {
	return &outboxRepository{base: base{pool: pool}, logger: logger}
}

func (r *outboxRepository) Append(ctx context.Context, events ...domain.OrderEvent) error {
	if len(events) == 0 {
		return nil
	}

	for _, event := range events {
		row, err := outboxRowFromDomain(event)
		if err != nil {
			return domain.Internal(fmt.Errorf("encode outbox payload: %w", err))
		}
		_, err = r.q(ctx).Exec(ctx, `
			INSERT INTO outbox_events (id, event_type, aggregate_type, aggregate_id,
			                           venue_id, payload, occurred_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			row.ID, row.EventType, row.AggregateType, row.AggregateID,
			row.VenueID, row.Payload, row.OccurredAt)
		if err != nil {
			return domain.Internal(fmt.Errorf("append outbox event: %w", err))
		}
	}
	return nil
}

func (r *outboxRepository) FetchUnpublished(ctx context.Context, limit int) ([]domain.OrderEvent, error) {
	rows, err := r.q(ctx).Query(ctx, `
		SELECT id, event_type, aggregate_type, aggregate_id, venue_id, payload,
		       occurred_at, published_at
		  FROM outbox_events
		 WHERE published_at IS NULL
		 ORDER BY occurred_at
		 LIMIT $1
		 FOR UPDATE SKIP LOCKED`, limit)
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("fetch outbox events: %w", err))
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.OutboxEvent])
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("scan outbox events: %w", err))
	}

	events := make([]domain.OrderEvent, 0, len(collected))
	for _, row := range collected {
		event, err := outboxToDomain(row)
		if err != nil {
			r.logger.Warn("outbox payload is not readable; delivering the event without statuses",
				slog.String("event_id", row.ID.String()),
				slog.String("event_type", row.EventType),
				slog.String("order_id", row.AggregateID.String()),
				slog.Any("error", err))
		}
		events = append(events, event)
	}
	return events, nil
}

func (r *outboxRepository) MarkPublished(ctx context.Context, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := r.q(ctx).Exec(ctx,
		`UPDATE outbox_events SET published_at = now() WHERE id = ANY($1)`, ids)
	if err != nil {
		return domain.Internal(fmt.Errorf("mark outbox events published: %w", err))
	}
	return nil
}
