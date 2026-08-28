package domain_test

import (
	"testing"
	"time"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"github.com/google/uuid"
)

func TestOrderTransitions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		from    domain.OrderStatus
		to      domain.OrderStatus
		actor   domain.Actor
		wantErr string
	}{
		{name: "venue accepts a new order", from: domain.OrderStatusCreated, to: domain.OrderStatusAccepted, actor: domain.ActorVenue},
		{name: "venue rejects a new order", from: domain.OrderStatusCreated, to: domain.OrderStatusRejected, actor: domain.ActorVenue},
		{name: "customer cancels before acceptance", from: domain.OrderStatusCreated, to: domain.OrderStatusCancelled, actor: domain.ActorCustomer},
		{name: "customer cancels after acceptance", from: domain.OrderStatusAccepted, to: domain.OrderStatusCancelled, actor: domain.ActorCustomer},
		{name: "venue starts cooking", from: domain.OrderStatusAccepted, to: domain.OrderStatusCooking, actor: domain.ActorVenue},
		{name: "venue marks ready", from: domain.OrderStatusCooking, to: domain.OrderStatusReady, actor: domain.ActorVenue},
		{name: "venue hands over to delivery", from: domain.OrderStatusReady, to: domain.OrderStatusInDelivery, actor: domain.ActorVenue},
		{name: "delivery completes", from: domain.OrderStatusInDelivery, to: domain.OrderStatusDelivered, actor: domain.ActorVenue},

		{
			name: "a customer cannot accept their own order",
			from: domain.OrderStatusCreated, to: domain.OrderStatusAccepted, actor: domain.ActorCustomer,
			wantErr: "transition_not_allowed",
		},
		{
			name: "a customer cannot mark an order delivered",
			from: domain.OrderStatusInDelivery, to: domain.OrderStatusDelivered, actor: domain.ActorCustomer,
			wantErr: "transition_not_allowed",
		},
		{
			name: "a venue cannot cancel on the customer's behalf",
			from: domain.OrderStatusCreated, to: domain.OrderStatusCancelled, actor: domain.ActorVenue,
			wantErr: "transition_not_allowed",
		},
		{
			name: "cooking cannot be skipped",
			from: domain.OrderStatusAccepted, to: domain.OrderStatusReady, actor: domain.ActorVenue,
			wantErr: "invalid_status_transition",
		},
		{
			name: "an order being cooked can no longer be cancelled",
			from: domain.OrderStatusCooking, to: domain.OrderStatusCancelled, actor: domain.ActorCustomer,
			wantErr: "invalid_status_transition",
		},
		{
			name: "a delivered order is final",
			from: domain.OrderStatusDelivered, to: domain.OrderStatusCancelled, actor: domain.ActorCustomer,
			wantErr: "order_finished",
		},
		{
			name: "a cancelled order cannot be revived",
			from: domain.OrderStatusCancelled, to: domain.OrderStatusAccepted, actor: domain.ActorVenue,
			wantErr: "order_finished",
		},
		{
			name: "moving to the current status is a conflict, not a no-op",
			from: domain.OrderStatusAccepted, to: domain.OrderStatusAccepted, actor: domain.ActorVenue,
			wantErr: "invalid_status_transition",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			order := &domain.Order{ID: uuid.New(), Status: testCase.from}

			change, err := order.TransitionTo(testCase.to, testCase.actor, "reason", time.Now())

			if testCase.wantErr == "" {
				if err != nil {
					t.Fatalf("expected the transition to be allowed, got %v", err)
				}
				if order.Status != testCase.to {
					t.Fatalf("status = %q, want %q", order.Status, testCase.to)
				}
				if change.FromStatus != testCase.from || change.ToStatus != testCase.to {
					t.Fatalf("timeline entry = %q -> %q, want %q -> %q",
						change.FromStatus, change.ToStatus, testCase.from, testCase.to)
				}
				if len(order.Timeline) != 1 {
					t.Fatalf("timeline has %d entries, want 1", len(order.Timeline))
				}
				return
			}

			domainErr, ok := domain.AsError(err)
			if !ok {
				t.Fatalf("expected a domain error, got %v", err)
			}
			if domainErr.Code != testCase.wantErr {
				t.Fatalf("code = %q, want %q", domainErr.Code, testCase.wantErr)
			}
			if order.Status != testCase.from {
				t.Fatalf("a rejected transition must not change the status, got %q", order.Status)
			}
		})
	}
}

func TestRejectionRequiresAReason(t *testing.T) {
	t.Parallel()

	order := &domain.Order{ID: uuid.New(), Status: domain.OrderStatusCreated}
	_, err := order.TransitionTo(domain.OrderStatusRejected, domain.ActorVenue, "", time.Now())

	domainErr, ok := domain.AsError(err)
	if !ok || domainErr.Code != "reject_reason_required" {
		t.Fatalf("want reject_reason_required, got %v", err)
	}
}

func TestStockIsReleasedOnlyByTerminalFailures(t *testing.T) {
	t.Parallel()

	cases := map[domain.OrderStatus]bool{
		domain.OrderStatusRejected:   true,
		domain.OrderStatusCancelled:  true,
		domain.OrderStatusDelivered:  false,
		domain.OrderStatusCooking:    false,
		domain.OrderStatusInDelivery: false,
	}

	for status, wantRelease := range cases {
		if status.ReleasesStock() != wantRelease {
			t.Errorf("%s.ReleasesStock() = %v, want %v",
				status, status.ReleasesStock(), wantRelease)
		}
	}
}

func TestNewOrderFromCart(t *testing.T) {
	t.Parallel()

	venue := &domain.Venue{
		ID:             uuid.New(),
		Name:           "Хинкальная №1",
		Status:         domain.VenueStatusActive,
		IsOpen:         true,
		MinOrderAmount: 50000,
		DeliveryFee:    19900,
		AvgPrepMinutes: 25,
	}
	item := domain.MenuItem{
		ID: uuid.New(), VenueID: venue.ID, Name: "Хинкали",
		Price: 45000, IsAvailable: true, StockQuantity: 10,
	}
	delivery := domain.Delivery{RecipientName: "Алексей", Phone: "+79000000000", Address: "Пятницкая, 20"}

	newCart := func(quantity int) domain.Cart {
		return domain.Cart{
			ID: uuid.New(), CustomerID: uuid.New(), VenueID: venue.ID,
			Status: domain.CartStatusActive,
			Lines:  []domain.CartLine{{MenuItemID: item.ID, Quantity: quantity}},
		}
	}
	menu := map[uuid.UUID]domain.MenuItem{item.ID: item}

	t.Run("prices and snapshots the cart", func(t *testing.T) {
		t.Parallel()

		priced := domain.PriceCart(newCart(2), venue, menu)
		order, err := domain.NewOrderFromCart(priced, delivery, time.Now())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if order.ItemsTotal != 90000 {
			t.Errorf("items total = %d, want 90000", order.ItemsTotal)
		}
		if order.Total != 90000+19900 {
			t.Errorf("total = %d, want %d", order.Total, 90000+19900)
		}
		if order.Status != domain.OrderStatusCreated {
			t.Errorf("status = %q, want created", order.Status)
		}

		if len(order.Items) != 1 || order.Items[0].Name != "Хинкали" || order.Items[0].UnitPrice != 45000 {
			t.Errorf("order line was not snapshotted: %+v", order.Items)
		}
		if len(order.Timeline) != 1 {
			t.Errorf("a new order must open its timeline, got %d entries", len(order.Timeline))
		}
	})

	t.Run("refuses a cart below the venue minimum", func(t *testing.T) {
		t.Parallel()

		cheap := *venue
		cheap.MinOrderAmount = 100000
		priced := domain.PriceCart(newCart(1), &cheap, menu)

		_, err := domain.NewOrderFromCart(priced, delivery, time.Now())
		domainErr, ok := domain.AsError(err)
		if !ok || domainErr.Code != "below_min_order" {
			t.Fatalf("want below_min_order, got %v", err)
		}
	})

	t.Run("refuses a closed venue", func(t *testing.T) {
		t.Parallel()

		closed := *venue
		closed.IsOpen = false
		priced := domain.PriceCart(newCart(2), &closed, menu)

		_, err := domain.NewOrderFromCart(priced, delivery, time.Now())
		domainErr, ok := domain.AsError(err)
		if !ok || domainErr.Code != "venue_closed" {
			t.Fatalf("want venue_closed, got %v", err)
		}
	})

	t.Run("reports every unavailable position rather than just the first", func(t *testing.T) {
		t.Parallel()

		soldOut := item
		soldOut.StockQuantity = 0
		disabled := domain.MenuItem{
			ID: uuid.New(), VenueID: venue.ID, Name: "Эспрессо",
			Price: 19000, IsAvailable: false, StockQuantity: 50,
		}

		cart := domain.Cart{
			ID: uuid.New(), CustomerID: uuid.New(), VenueID: venue.ID,
			Lines: []domain.CartLine{
				{MenuItemID: soldOut.ID, Quantity: 2},
				{MenuItemID: disabled.ID, Quantity: 1},
			},
		}
		priced := domain.PriceCart(cart, venue, map[uuid.UUID]domain.MenuItem{
			soldOut.ID: soldOut, disabled.ID: disabled,
		})

		_, err := domain.NewOrderFromCart(priced, delivery, time.Now())
		domainErr, ok := domain.AsError(err)
		if !ok || domainErr.Code != "items_unavailable" {
			t.Fatalf("want items_unavailable, got %v", err)
		}
		if len(domainErr.Items) != 2 {
			t.Fatalf("want both positions reported, got %d", len(domainErr.Items))
		}

		reasons := map[uuid.UUID]domain.UnavailableReason{}
		for _, unavailable := range domainErr.Items {
			reasons[unavailable.MenuItemID] = unavailable.Reason
		}
		if reasons[soldOut.ID] != domain.ReasonSoldOut {
			t.Errorf("sold-out item reported as %q", reasons[soldOut.ID])
		}
		if reasons[disabled.ID] != domain.ReasonDisabled {
			t.Errorf("disabled item reported as %q", reasons[disabled.ID])
		}
	})

	t.Run("requires delivery details", func(t *testing.T) {
		t.Parallel()

		priced := domain.PriceCart(newCart(2), venue, menu)
		_, err := domain.NewOrderFromCart(priced, domain.Delivery{Phone: "+7900"}, time.Now())
		if domainErr, ok := domain.AsError(err); !ok || domainErr.Code != "delivery_name_required" {
			t.Fatalf("want delivery_name_required, got %v", err)
		}
	})
}

func TestOrderNumberIsReadableAloud(t *testing.T) {
	t.Parallel()

	seen := make(map[string]bool, 512)
	for range 512 {
		number := domain.NewOrderNumber()
		if len(number) != 9 || number[:3] != "AK-" {
			t.Fatalf("unexpected order number format: %q", number)
		}

		for _, char := range number[3:] {
			if char == 'I' || char == 'O' || char == '0' || char == '1' {
				t.Fatalf("ambiguous character %q in order number %q", char, number)
			}
		}
		seen[number] = true
	}
	if len(seen) < 500 {
		t.Fatalf("only %d distinct numbers out of 512 — entropy is too low", len(seen))
	}
}
