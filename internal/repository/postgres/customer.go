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

type customerRepository struct {
	base
}

func NewCustomerRepository(pool *pgxpool.Pool) service.CustomerRepository {
	return &customerRepository{base{pool: pool}}
}

func (r *customerRepository) Ensure(ctx context.Context, id uuid.UUID) (*domain.Customer, error) {
	rows, err := r.q(ctx).Query(ctx, `
		INSERT INTO customers (id) VALUES ($1)
		ON CONFLICT (id) DO UPDATE SET id = EXCLUDED.id
		RETURNING id, display_name, phone, created_at, updated_at`, id)
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("ensure customer: %w", err))
	}
	row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[models.Customer])
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("scan customer: %w", err))
	}
	customer := customerToDomain(row)
	return &customer, nil
}

func (r *customerRepository) UpdateContacts(ctx context.Context, id uuid.UUID, displayName, phone string) error {
	_, err := r.q(ctx).Exec(ctx, `
		UPDATE customers
		   SET display_name = COALESCE(NULLIF($2, ''), display_name),
		       phone        = COALESCE(NULLIF($3, ''), phone),
		       updated_at   = now()
		 WHERE id = $1`, id, displayName, phone)
	if err != nil {
		return domain.Internal(fmt.Errorf("update customer contacts: %w", err))
	}
	return nil
}
