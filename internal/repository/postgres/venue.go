package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/repository/postgres/models"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/service"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type venueRepository struct {
	base
}

func NewVenueRepository(pool *pgxpool.Pool) service.VenueRepository {
	return &venueRepository{base{pool: pool}}
}

const venueColumns = `
	id, slug, name, description, cuisines, city, address, status, is_open,
	avg_prep_minutes, min_order_amount, delivery_fee, rating::float8 AS rating,
	created_at, updated_at`

func (r *venueRepository) Get(ctx context.Context, id uuid.UUID) (*domain.Venue, error) {
	rows, err := r.q(ctx).Query(ctx,
		`SELECT `+venueColumns+` FROM venues WHERE id = $1`, id)
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("query venue: %w", err))
	}
	row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[models.Venue])
	if err != nil {
		if isNoRows(err) {
			return nil, domain.NotFound("venue_not_found", "venue not found")
		}
		return nil, domain.Internal(fmt.Errorf("scan venue: %w", err))
	}
	venue := venueToDomain(row)
	return &venue, nil
}

func (r *venueRepository) List(ctx context.Context, filter service.VenueFilter) ([]domain.Venue, int, error) {
	conditions := []string{"status = 'active'"}
	args := []any{}

	if filter.City != "" {
		args = append(args, filter.City)
		conditions = append(conditions, fmt.Sprintf("city = $%d", len(args)))
	}
	if filter.Cuisine != "" {
		args = append(args, filter.Cuisine)
		conditions = append(conditions, fmt.Sprintf("$%d = ANY(cuisines)", len(args)))
	}
	if query := strings.TrimSpace(filter.Query); query != "" {
		args = append(args, "%"+query+"%")
		conditions = append(conditions,
			fmt.Sprintf("(name ILIKE $%d OR description ILIKE $%d)", len(args), len(args)))
	}
	if filter.OpenNow != nil && *filter.OpenNow {
		conditions = append(conditions, "is_open = TRUE")
	}
	where := "WHERE " + strings.Join(conditions, " AND ")

	var total int
	if err := r.q(ctx).QueryRow(ctx,
		`SELECT count(*) FROM venues `+where, args...,
	).Scan(&total); err != nil {
		return nil, 0, domain.Internal(fmt.Errorf("count venues: %w", err))
	}

	args = append(args, filter.Page.Limit, filter.Page.Offset)
	listSQL := fmt.Sprintf(
		`SELECT %s FROM venues %s
		 ORDER BY is_open DESC, rating DESC, name ASC
		 LIMIT $%d OFFSET $%d`,
		venueColumns, where, len(args)-1, len(args))

	rows, err := r.q(ctx).Query(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, domain.Internal(fmt.Errorf("list venues: %w", err))
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.Venue])
	if err != nil {
		return nil, 0, domain.Internal(fmt.Errorf("scan venues: %w", err))
	}
	return venuesToDomain(collected), total, nil
}

func (r *venueRepository) Update(ctx context.Context, venue *domain.Venue) error {
	tag, err := r.q(ctx).Exec(ctx, `
		UPDATE venues
		   SET description      = $2,
		       is_open          = $3,
		       avg_prep_minutes = $4,
		       min_order_amount = $5,
		       delivery_fee     = $6,
		       updated_at       = now()
		 WHERE id = $1`,
		venue.ID, venue.Description, venue.IsOpen, venue.AvgPrepMinutes,
		int64(venue.MinOrderAmount), int64(venue.DeliveryFee))
	if err != nil {
		return domain.Internal(fmt.Errorf("update venue: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return domain.NotFound("venue_not_found", "venue not found")
	}
	return nil
}

func (r *venueRepository) ResolveAPIKey(ctx context.Context, key string) (*domain.Venue, error) {
	digest := sha256.Sum256([]byte(key))
	hash := hex.EncodeToString(digest[:])

	rows, err := r.q(ctx).Query(ctx, `
		SELECT `+venueColumns+`
		  FROM venues
		 WHERE id = (SELECT venue_id
		               FROM venue_api_keys
		              WHERE key_hash = $1 AND revoked_at IS NULL)`, hash)
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("resolve api key: %w", err))
	}
	row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[models.Venue])
	if err != nil {
		if isNoRows(err) {
			return nil, domain.Unauthenticated("invalid_api_key", "unknown or revoked API key")
		}
		return nil, domain.Internal(fmt.Errorf("scan venue by api key: %w", err))
	}
	venue := venueToDomain(row)
	return &venue, nil
}
