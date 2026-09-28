package service_test

import (
	"context"
	"testing"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/service"
	"github.com/google/uuid"
)

func openVenue() domain.Venue {
	return domain.Venue{
		Slug:           "hinkalnaya-1",
		Name:           "Хинкальная №1",
		Status:         domain.VenueStatusActive,
		IsOpen:         true,
		MinOrderAmount: 50000,
		DeliveryFee:    19900,
		AvgPrepMinutes: 25,
	}
}

func delivery() domain.Delivery {
	return domain.Delivery{
		RecipientName: "Алексей",
		Phone:         "+7 900 000-00-00",
		Address:       "Москва, ул. Пятницкая, 20",
	}
}

func TestCheckout(t *testing.T) {
	t.Parallel()

	t.Run("moves stock, closes the cart and queues a notification", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		fix := newFixture()
		venue := fix.addVenue(openVenue())
		item := fix.addItem(domain.MenuItem{
			VenueID: venue.ID, Name: "Хинкали", Price: 45000,
			IsAvailable: true, StockQuantity: 10,
		})
		customerID := uuid.New()

		if _, err := fix.carts.SetItem(ctx, customerID, venue.ID, item.ID, 2); err != nil {
			t.Fatalf("adding to the cart failed: %v", err)
		}

		order, err := fix.orders.Checkout(ctx, service.CheckoutCommand{
			CustomerID:     customerID,
			IdempotencyKey: "checkout-1",
			Delivery:       delivery(),
		})
		if err != nil {
			t.Fatalf("checkout failed: %v", err)
		}

		if order.Status != domain.OrderStatusCreated {
			t.Errorf("status = %q, want created", order.Status)
		}
		if order.Total != 45000*2+19900 {
			t.Errorf("total = %d, want %d", order.Total, 45000*2+19900)
		}
		if got := fix.stockOf(item.ID); got != 8 {
			t.Errorf("stock = %d, want 8 — checkout must reserve the portions", got)
		}

		cart, err := fix.carts.Get(ctx, customerID)
		if err != nil {
			t.Fatalf("reading the cart failed: %v", err)
		}
		if len(cart.Lines) != 0 {
			t.Errorf("the cart should be closed after checkout, got %d lines", len(cart.Lines))
		}

		events := fix.outboxEvents()
		if len(events) != 1 || events[0].Type != domain.EventOrderCreated {
			t.Fatalf("want one order.created event in the outbox, got %+v", events)
		}
		if events[0].VenueID != venue.ID || events[0].OrderID != order.ID {
			t.Errorf("the event does not point at the order it describes: %+v", events[0])
		}
	})

	t.Run("a retry with the same key returns the original order", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		fix := newFixture()
		venue := fix.addVenue(openVenue())
		item := fix.addItem(domain.MenuItem{
			VenueID: venue.ID, Name: "Хинкали", Price: 45000,
			IsAvailable: true, StockQuantity: 10,
		})
		customerID := uuid.New()
		if _, err := fix.carts.SetItem(ctx, customerID, venue.ID, item.ID, 2); err != nil {
			t.Fatalf("adding to the cart failed: %v", err)
		}

		cmd := service.CheckoutCommand{
			CustomerID:     customerID,
			IdempotencyKey: "same-key",
			Delivery:       delivery(),
		}
		first, err := fix.orders.Checkout(ctx, cmd)
		if err != nil {
			t.Fatalf("first checkout failed: %v", err)
		}
		second, err := fix.orders.Checkout(ctx, cmd)
		if err != nil {
			t.Fatalf("retry failed: %v", err)
		}

		if first.ID != second.ID {
			t.Errorf("a retry created a second order: %s vs %s", first.ID, second.ID)
		}

		if got := fix.stockOf(item.ID); got != 8 {
			t.Errorf("stock = %d, want 8 — a retry must not reserve twice", got)
		}
	})

	t.Run("refuses a cart with a sold-out position and takes no stock", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		fix := newFixture()
		venue := fix.addVenue(openVenue())
		plenty := fix.addItem(domain.MenuItem{
			VenueID: venue.ID, Name: "Хинкали", Price: 45000,
			IsAvailable: true, StockQuantity: 10,
		})
		scarce := fix.addItem(domain.MenuItem{
			VenueID: venue.ID, Name: "Хачапури", Price: 59000,
			IsAvailable: true, StockQuantity: 1,
		})
		customerID := uuid.New()

		if _, err := fix.carts.SetItem(ctx, customerID, venue.ID, plenty.ID, 2); err != nil {
			t.Fatalf("adding to the cart failed: %v", err)
		}
		if _, err := fix.carts.SetItem(ctx, customerID, venue.ID, scarce.ID, 3); err != nil {
			t.Fatalf("adding to the cart failed: %v", err)
		}

		_, err := fix.orders.Checkout(ctx, service.CheckoutCommand{
			CustomerID:     customerID,
			IdempotencyKey: "unavailable-1",
			Delivery:       delivery(),
		})
		domainErr, ok := domain.AsError(err)
		if !ok || domainErr.Code != "items_unavailable" {
			t.Fatalf("want items_unavailable, got %v", err)
		}
		if len(domainErr.Items) != 1 || domainErr.Items[0].MenuItemID != scarce.ID {
			t.Errorf("the error must name the offending line, got %+v", domainErr.Items)
		}

		if got := fix.stockOf(plenty.ID); got != 10 {
			t.Errorf("stock = %d, want 10 — a failed checkout must not move stock", got)
		}
	})

	t.Run("refuses a closed venue", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()

		fix := newFixture()
		venue := openVenue()
		venue = fix.addVenue(venue)
		item := fix.addItem(domain.MenuItem{
			VenueID: venue.ID, Name: "Хинкали", Price: 45000,
			IsAvailable: true, StockQuantity: 10,
		})
		customerID := uuid.New()
		if _, err := fix.carts.SetItem(ctx, customerID, venue.ID, item.ID, 2); err != nil {
			t.Fatalf("adding to the cart failed: %v", err)
		}

		closed := venue
		closed.IsOpen = false
		fix.addVenue(closed)

		_, err := fix.orders.Checkout(ctx, service.CheckoutCommand{
			CustomerID:     customerID,
			IdempotencyKey: "closed-1",
			Delivery:       delivery(),
		})
		if domainErr, ok := domain.AsError(err); !ok || domainErr.Code != "venue_closed" {
			t.Fatalf("want venue_closed, got %v", err)
		}
	})

	t.Run("requires an idempotency key", func(t *testing.T) {
		t.Parallel()

		fix := newFixture()
		_, err := fix.orders.Checkout(context.Background(), service.CheckoutCommand{
			CustomerID: uuid.New(),
			Delivery:   delivery(),
		})
		if domainErr, ok := domain.AsError(err); !ok || domainErr.Code != "idempotency_key_required" {
			t.Fatalf("want idempotency_key_required, got %v", err)
		}
	})

	t.Run("refuses an empty cart", func(t *testing.T) {
		t.Parallel()

		fix := newFixture()
		_, err := fix.orders.Checkout(context.Background(), service.CheckoutCommand{
			CustomerID:     uuid.New(),
			IdempotencyKey: "empty-1",
			Delivery:       delivery(),
		})
		if domainErr, ok := domain.AsError(err); !ok || domainErr.Code != "cart_empty" {
			t.Fatalf("want cart_empty, got %v", err)
		}
	})
}

func TestCancellationReturnsStock(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	fix := newFixture()
	venue := fix.addVenue(openVenue())
	item := fix.addItem(domain.MenuItem{
		VenueID: venue.ID, Name: "Хинкали", Price: 45000,
		IsAvailable: true, StockQuantity: 10,
	})
	customerID := uuid.New()
	if _, err := fix.carts.SetItem(ctx, customerID, venue.ID, item.ID, 3); err != nil {
		t.Fatalf("adding to the cart failed: %v", err)
	}

	order, err := fix.orders.Checkout(ctx, service.CheckoutCommand{
		CustomerID:     customerID,
		IdempotencyKey: "cancel-1",
		Delivery:       delivery(),
	})
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}
	if got := fix.stockOf(item.ID); got != 7 {
		t.Fatalf("stock = %d, want 7 after the order", got)
	}

	cancelled, err := fix.orders.Cancel(ctx, customerID, order.ID, "передумал")
	if err != nil {
		t.Fatalf("cancel failed: %v", err)
	}
	if cancelled.Status != domain.OrderStatusCancelled {
		t.Errorf("status = %q, want cancelled", cancelled.Status)
	}
	if got := fix.stockOf(item.ID); got != 10 {
		t.Errorf("stock = %d, want 10 — cancelling must return the portions", got)
	}

	if _, err := fix.orders.Cancel(ctx, uuid.New(), order.ID, "не мой"); err != nil {
		if domainErr, ok := domain.AsError(err); !ok || domainErr.Code != "order_not_found" {
			t.Errorf("want order_not_found for another customer, got %v", err)
		}
	} else {
		t.Error("cancelling another customer's order must fail")
	}
}

func TestFulfilmentFlow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	fix := newFixture()
	venue := fix.addVenue(openVenue())
	other := fix.addVenue(domain.Venue{
		Slug: "pizza-avenue", Name: "Pizza Avenue",
		Status: domain.VenueStatusActive, IsOpen: true,
	})
	item := fix.addItem(domain.MenuItem{
		VenueID: venue.ID, Name: "Хинкали", Price: 45000,
		IsAvailable: true, StockQuantity: 10,
	})
	customerID := uuid.New()
	if _, err := fix.carts.SetItem(ctx, customerID, venue.ID, item.ID, 2); err != nil {
		t.Fatalf("adding to the cart failed: %v", err)
	}

	order, err := fix.orders.Checkout(ctx, service.CheckoutCommand{
		CustomerID:     customerID,
		IdempotencyKey: "flow-1",
		Delivery:       delivery(),
	})
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}

	if _, err := fix.fulfillment.Get(ctx, other.ID, order.ID); err != nil {
		if domainErr, ok := domain.AsError(err); !ok || domainErr.Code != "order_not_found" {
			t.Errorf("want order_not_found across venues, got %v", err)
		}
	} else {
		t.Error("a venue must not be able to read another venue's order")
	}

	accepted, err := fix.fulfillment.Accept(ctx, venue.ID, order.ID, 30)
	if err != nil {
		t.Fatalf("accept failed: %v", err)
	}
	if accepted.Status != domain.OrderStatusAccepted || accepted.PrepMinutes != 30 {
		t.Errorf("accepted order = %q / %d min, want accepted / 30",
			accepted.Status, accepted.PrepMinutes)
	}

	for _, next := range []domain.OrderStatus{
		domain.OrderStatusCooking,
		domain.OrderStatusReady,
		domain.OrderStatusInDelivery,
		domain.OrderStatusDelivered,
	} {
		updated, err := fix.fulfillment.UpdateStatus(ctx, venue.ID, order.ID, next, "")
		if err != nil {
			t.Fatalf("transition to %q failed: %v", next, err)
		}
		if updated.Status != next {
			t.Fatalf("status = %q, want %q", updated.Status, next)
		}
	}

	if got := fix.stockOf(item.ID); got != 8 {
		t.Errorf("stock = %d, want 8 — a delivered order does not return portions", got)
	}

	final, err := fix.orders.Get(ctx, customerID, order.ID)
	if err != nil {
		t.Fatalf("reading the order failed: %v", err)
	}

	if len(final.Timeline) != 6 {
		t.Errorf("timeline has %d entries, want 6: %+v", len(final.Timeline), final.Timeline)
	}

	if got := len(fix.outboxEvents()); got != 6 {
		t.Errorf("outbox holds %d events, want 6", got)
	}
}

func TestRejectionReturnsStockAndReachesTheCustomer(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	fix := newFixture()
	venue := fix.addVenue(openVenue())
	item := fix.addItem(domain.MenuItem{
		VenueID: venue.ID, Name: "Хинкали", Price: 45000,
		IsAvailable: true, StockQuantity: 10,
	})
	customerID := uuid.New()
	if _, err := fix.carts.SetItem(ctx, customerID, venue.ID, item.ID, 2); err != nil {
		t.Fatalf("adding to the cart failed: %v", err)
	}
	order, err := fix.orders.Checkout(ctx, service.CheckoutCommand{
		CustomerID:     customerID,
		IdempotencyKey: "reject-1",
		Delivery:       delivery(),
	})
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}

	rejected, err := fix.fulfillment.Reject(ctx, venue.ID, order.ID, "закончилось тесто")
	if err != nil {
		t.Fatalf("reject failed: %v", err)
	}
	if rejected.Status != domain.OrderStatusRejected {
		t.Errorf("status = %q, want rejected", rejected.Status)
	}
	if rejected.StatusReason != "закончилось тесто" {
		t.Errorf("the reason must reach the customer, got %q", rejected.StatusReason)
	}
	if got := fix.stockOf(item.ID); got != 10 {
		t.Errorf("stock = %d, want 10 — a rejection must return the portions", got)
	}

	if _, err := fix.fulfillment.Reject(ctx, venue.ID, order.ID, ""); err == nil {
		t.Error("rejecting without a reason must fail")
	}
}
