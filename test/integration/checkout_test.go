//go:build integration

package integration

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/repository/postgres"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/service"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultTestDSN = "postgres://kitchen:kitchen@localhost:5433/kitchen?sslmode=disable"

type harness struct {
	pool        *pgxpool.Pool
	carts       service.CartService
	orders      service.OrderService
	fulfillment service.FulfillmentService

	venueID uuid.UUID
	itemID  uuid.UUID
}

type noopNotifier struct{}

func (noopNotifier) Notify() {}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()

	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		dsn = defaultTestDSN
	}

	pool, err := postgres.NewPool(ctx, postgres.PoolConfig{DSN: dsn, MaxConns: 20, ConnectTimeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("cannot reach PostgreSQL at %s: %v\nStart it with `make up` or `docker compose up -d postgres`.", dsn, err)
	}
	if err := postgres.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrations failed: %v", err)
	}

	txManager := postgres.NewTxManager(pool)
	venues := postgres.NewVenueRepository(pool)
	menus := postgres.NewMenuRepository(pool)
	carts := postgres.NewCartRepository(pool)
	orders := postgres.NewOrderRepository(pool)
	customers := postgres.NewCustomerRepository(pool)
	outbox := postgres.NewOutboxRepository(pool, slog.New(slog.NewTextHandler(io.Discard, nil)))

	clock := service.Clock(service.SystemClock)
	notifier := noopNotifier{}

	h := &harness{
		pool:  pool,
		carts: service.NewCartService(carts, menus, venues, customers, clock),
		orders: service.NewOrderService(
			txManager, orders, carts, menus, venues, customers, outbox, notifier, clock),
		fulfillment: service.NewFulfillmentService(txManager, orders, menus, outbox, notifier, clock),
	}
	h.seed(t)

	t.Cleanup(func() { h.cleanup(t); pool.Close() })
	return h
}

func (h *harness) seed(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	h.venueID = uuid.New()
	categoryID := uuid.New()
	h.itemID = uuid.New()

	_, err := h.pool.Exec(ctx, `
		INSERT INTO venues (id, slug, name, city, address, status, is_open,
		                    avg_prep_minutes, min_order_amount, delivery_fee)
		VALUES ($1, $2, 'Integration Test Venue', 'Москва', 'тестовая, 1',
		        'active', TRUE, 20, 0, 0)`,
		h.venueID, "integration-"+h.venueID.String()[:8])
	if err != nil {
		t.Fatalf("seeding the venue failed: %v", err)
	}

	if _, err := h.pool.Exec(ctx, `
		INSERT INTO menu_categories (id, venue_id, name, position)
		VALUES ($1, $2, 'Тест', 1)`, categoryID, h.venueID); err != nil {
		t.Fatalf("seeding the category failed: %v", err)
	}

	if _, err := h.pool.Exec(ctx, `
		INSERT INTO menu_items (id, venue_id, category_id, name, price,
		                        is_available, stock_quantity)
		VALUES ($1, $2, $3, 'Последняя порция', 10000, TRUE, 5)`,
		h.itemID, h.venueID, categoryID); err != nil {
		t.Fatalf("seeding the menu item failed: %v", err)
	}
}

func (h *harness) cleanup(t *testing.T) {
	t.Helper()
	ctx := context.Background()

	if _, err := h.pool.Exec(ctx,
		`DELETE FROM outbox_events WHERE venue_id = $1`, h.venueID); err != nil {
		t.Logf("cleanup (outbox): %v", err)
	}
	if _, err := h.pool.Exec(ctx,
		`DELETE FROM orders WHERE venue_id = $1`, h.venueID); err != nil {
		t.Logf("cleanup (orders): %v", err)
	}
	if _, err := h.pool.Exec(ctx,
		`DELETE FROM carts WHERE venue_id = $1`, h.venueID); err != nil {
		t.Logf("cleanup (carts): %v", err)
	}
	if _, err := h.pool.Exec(ctx,
		`DELETE FROM venues WHERE id = $1`, h.venueID); err != nil {
		t.Logf("cleanup (venue): %v", err)
	}
}

func (h *harness) stock(t *testing.T) int {
	t.Helper()

	var stock int
	if err := h.pool.QueryRow(context.Background(),
		`SELECT stock_quantity FROM menu_items WHERE id = $1`, h.itemID).Scan(&stock); err != nil {
		t.Fatalf("reading stock failed: %v", err)
	}
	return stock
}

func TestConcurrentCheckoutsCannotOversell(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	const (
		contenders = 20
		portions   = 5
	)

	customers := make([]uuid.UUID, contenders)
	for i := range customers {
		customers[i] = uuid.New()
		if _, err := h.carts.SetItem(ctx, customers[i], h.venueID, h.itemID, 1); err != nil {
			t.Fatalf("filling cart %d failed: %v", i, err)
		}
	}

	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		succeeded int
		conflicts int
		others    []error
	)

	start := make(chan struct{})
	for i, customerID := range customers {
		wg.Go(func() {
			<-start

			_, err := h.orders.Checkout(ctx, service.CheckoutCommand{
				CustomerID:     customerID,
				IdempotencyKey: fmt.Sprintf("race-%d-%s", i, customerID),
				Delivery: domain.Delivery{
					RecipientName: "Гонщик", Phone: "+79000000000", Address: "тестовая, 2",
				},
			})

			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				succeeded++
			case isCode(err, "items_unavailable"):
				conflicts++
			default:
				others = append(others, err)
			}
		})
	}
	close(start)
	wg.Wait()

	if len(others) > 0 {
		t.Fatalf("unexpected failures: %v", others)
	}
	if succeeded != portions {
		t.Errorf("%d checkouts succeeded, want exactly %d", succeeded, portions)
	}
	if conflicts != contenders-portions {
		t.Errorf("%d checkouts were told the item is gone, want %d",
			conflicts, contenders-portions)
	}
	if got := h.stock(t); got != 0 {
		t.Errorf("stock = %d, want 0 — the kitchen must never owe portions it does not have", got)
	}
}

func TestIdempotencyKeyHoldsUnderConcurrency(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	customerID := uuid.New()
	if _, err := h.carts.SetItem(ctx, customerID, h.venueID, h.itemID, 2); err != nil {
		t.Fatalf("filling the cart failed: %v", err)
	}

	const attempts = 8
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		ids   = map[uuid.UUID]struct{}{}
		fails []error
	)

	start := make(chan struct{})
	for range attempts {
		wg.Go(func() {
			<-start

			order, err := h.orders.Checkout(ctx, service.CheckoutCommand{
				CustomerID:     customerID,
				IdempotencyKey: "double-click",
				Delivery: domain.Delivery{
					RecipientName: "Нетерпеливый", Phone: "+79000000001", Address: "тестовая, 3",
				},
			})

			mu.Lock()
			defer mu.Unlock()
			if err != nil {

				if !isCode(err, "order_duplicate") && !isCode(err, "cart_empty") {
					fails = append(fails, err)
				}
				return
			}
			ids[order.ID] = struct{}{}
		})
	}
	close(start)
	wg.Wait()

	if len(fails) > 0 {
		t.Fatalf("unexpected failures: %v", fails)
	}
	if len(ids) != 1 {
		t.Fatalf("%d distinct orders were created, want exactly 1", len(ids))
	}

	var count int
	if err := h.pool.QueryRow(ctx,
		`SELECT count(*) FROM orders WHERE customer_id = $1`, customerID).Scan(&count); err != nil {
		t.Fatalf("counting orders failed: %v", err)
	}
	if count != 1 {
		t.Errorf("%d order rows in the database, want 1", count)
	}
	if got := h.stock(t); got != 3 {
		t.Errorf("stock = %d, want 3 — only one order may reserve portions", got)
	}
}

func TestRejectionReturnsStockToTheMenu(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	customerID := uuid.New()
	if _, err := h.carts.SetItem(ctx, customerID, h.venueID, h.itemID, 3); err != nil {
		t.Fatalf("filling the cart failed: %v", err)
	}

	order, err := h.orders.Checkout(ctx, service.CheckoutCommand{
		CustomerID:     customerID,
		IdempotencyKey: "reject-flow",
		Delivery: domain.Delivery{
			RecipientName: "Клиент", Phone: "+79000000002", Address: "тестовая, 4",
		},
	})
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}
	if got := h.stock(t); got != 2 {
		t.Fatalf("stock = %d, want 2 after the order", got)
	}

	if _, err := h.fulfillment.Reject(ctx, h.venueID, order.ID, "закончились продукты"); err != nil {
		t.Fatalf("reject failed: %v", err)
	}
	if got := h.stock(t); got != 5 {
		t.Errorf("stock = %d, want 5 — a rejection must return the portions", got)
	}

	var events int
	if err := h.pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox_events WHERE aggregate_id = $1`, order.ID).Scan(&events); err != nil {
		t.Fatalf("counting outbox events failed: %v", err)
	}
	if events != 2 {
		t.Errorf("%d outbox events, want 2 (created + status_changed)", events)
	}
}

func isCode(err error, code string) bool {
	domainErr, ok := domain.AsError(err)
	return ok && domainErr.Code == code
}
