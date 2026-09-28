package postgres

import (
	"context"
	"errors"
	"fmt"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/service"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type txContextKey struct{}

func withTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, txContextKey{}, tx)
}

func txFromContext(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txContextKey{}).(pgx.Tx)
	return tx, ok
}

type txManager struct {
	pool *pgxpool.Pool
}

func NewTxManager(pool *pgxpool.Pool) service.TxManager { return &txManager{pool: pool} }

func (m *txManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, alreadyInTx := txFromContext(ctx); alreadyInTx {
		return fn(ctx)
	}

	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	committed := false
	defer func() {
		if committed {
			return
		}

		_ = tx.Rollback(context.WithoutCancel(ctx)) //nolint:gosec
	}()

	if err := fn(withTx(ctx, tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	committed = true
	return nil
}

type base struct {
	pool *pgxpool.Pool
}

func (b base) q(ctx context.Context) querier {
	if tx, ok := txFromContext(ctx); ok {
		return tx
	}
	return b.pool
}

func pgErrorCode(err error) string {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code
	}
	return ""
}

func pgConstraint(err error) string {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.ConstraintName
	}
	return ""
}

const (
	sqlStateUniqueViolation     = "23505"
	sqlStateCheckViolation      = "23514"
	sqlStateForeignKeyViolation = "23503"
)

func isNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
