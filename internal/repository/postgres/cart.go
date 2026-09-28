package postgres

import (
	"context"
	"fmt"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/repository/postgres/models"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/service"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type cartRepository struct {
	base
}

func NewCartRepository(pool *pgxpool.Pool) service.CartRepository {
	return &cartRepository{base{pool: pool}}
}

func (r *cartRepository) GetActive(ctx context.Context, customerID uuid.UUID) (*domain.Cart, error) {
	rows, err := r.q(ctx).Query(ctx, `
		SELECT id, customer_id, venue_id, status, created_at, updated_at
		  FROM carts
		 WHERE customer_id = $1 AND status = 'active'`, customerID)
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("query cart: %w", err))
	}
	cartRow, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[models.Cart])
	if err != nil {
		if isNoRows(err) {
			return nil, nil //nolint:nilnil
		}
		return nil, domain.Internal(fmt.Errorf("scan cart: %w", err))
	}

	itemRows, err := r.q(ctx).Query(ctx, `
		SELECT id, cart_id, menu_item_id, quantity, created_at
		  FROM cart_items
		 WHERE cart_id = $1
		 ORDER BY created_at, id`, cartRow.ID)
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("query cart items: %w", err))
	}
	itemRowsCollected, err := pgx.CollectRows(itemRows, pgx.RowToStructByName[models.CartItem])
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("scan cart items: %w", err))
	}

	cart := cartToDomain(cartRow, itemRowsCollected)
	return &cart, nil
}

func (r *cartRepository) Save(ctx context.Context, cart *domain.Cart) error {
	querier := r.q(ctx)

	_, err := querier.Exec(ctx, `
		INSERT INTO carts (id, customer_id, venue_id, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE
		   SET venue_id   = EXCLUDED.venue_id,
		       updated_at = EXCLUDED.updated_at`,
		cart.ID, cart.CustomerID, cart.VenueID, string(cart.Status),
		cart.CreatedAt, cart.UpdatedAt)
	if err != nil {
		return domain.Internal(fmt.Errorf("save cart: %w", err))
	}

	if _, err := querier.Exec(ctx, `DELETE FROM cart_items WHERE cart_id = $1`, cart.ID); err != nil {
		return domain.Internal(fmt.Errorf("clear cart items: %w", err))
	}
	if len(cart.Lines) == 0 {
		return nil
	}

	ids := make([]uuid.UUID, 0, len(cart.Lines))
	quantities := make([]int32, 0, len(cart.Lines))
	for _, line := range cart.Lines {
		ids = append(ids, line.MenuItemID)
		quantities = append(quantities, int32(line.Quantity)) //nolint:gosec
	}

	_, err = querier.Exec(ctx, `
		INSERT INTO cart_items (cart_id, menu_item_id, quantity)
		SELECT $1, item_id, quantity
		  FROM unnest($2::uuid[], $3::int[]) AS t(item_id, quantity)`,
		cart.ID, ids, quantities)
	if err != nil {
		return domain.Internal(fmt.Errorf("insert cart items: %w", err))
	}
	return nil
}

func (r *cartRepository) MarkOrdered(ctx context.Context, cartID uuid.UUID) error {
	_, err := r.q(ctx).Exec(ctx,
		`UPDATE carts SET status = 'ordered', updated_at = now() WHERE id = $1`, cartID)
	if err != nil {
		return domain.Internal(fmt.Errorf("mark cart ordered: %w", err))
	}
	return nil
}

func (r *cartRepository) DeleteActive(ctx context.Context, customerID uuid.UUID) error {
	_, err := r.q(ctx).Exec(ctx,
		`DELETE FROM carts WHERE customer_id = $1 AND status = 'active'`, customerID)
	if err != nil {
		return domain.Internal(fmt.Errorf("delete cart: %w", err))
	}
	return nil
}
