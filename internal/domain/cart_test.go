package domain_test

import (
	"testing"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"github.com/google/uuid"
)

func TestSetLineIsAbsoluteAndThereforeIdempotent(t *testing.T) {
	t.Parallel()

	itemID := uuid.New()
	cart := &domain.Cart{ID: uuid.New(), CustomerID: uuid.New()}

	for range 3 {
		if err := cart.SetLine(itemID, 2); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if len(cart.Lines) != 1 || cart.Lines[0].Quantity != 2 {
		t.Fatalf("lines = %+v, want a single line of 2", cart.Lines)
	}

	if err := cart.SetLine(itemID, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cart.Lines) != 0 {
		t.Fatalf("quantity 0 must remove the line, got %+v", cart.Lines)
	}

	if err := cart.SetLine(itemID, 0); err != nil {
		t.Fatalf("removing an absent line should be a no-op, got %v", err)
	}

	if err := cart.SetLine(itemID, domain.MaxCartLineQuantity+1); err == nil {
		t.Fatal("quantities above the cap must be rejected")
	}
}

func TestCartIsBoundToOneVenue(t *testing.T) {
	t.Parallel()

	first, second := uuid.New(), uuid.New()
	cart := &domain.Cart{ID: uuid.New(), VenueID: first}

	if err := cart.EnsureVenue(second); err != nil {
		t.Fatalf("an empty cart should accept any venue, got %v", err)
	}

	cart.Lines = []domain.CartLine{{MenuItemID: uuid.New(), Quantity: 1}}
	if err := cart.EnsureVenue(first); err != nil {
		t.Fatalf("the cart's own venue must be accepted, got %v", err)
	}

	err := cart.EnsureVenue(second)
	domainErr, ok := domain.AsError(err)
	if !ok || domainErr.Code != "cart_venue_conflict" {
		t.Fatalf("want cart_venue_conflict, got %v", err)
	}
}

func TestPriceCart(t *testing.T) {
	t.Parallel()

	venue := &domain.Venue{
		ID: uuid.New(), Name: "Pizza Avenue",
		Status: domain.VenueStatusActive, IsOpen: true,
		MinOrderAmount: 70000, DeliveryFee: 0,
	}
	available := domain.MenuItem{
		ID: uuid.New(), VenueID: venue.ID, Name: "Маргарита",
		Price: 59000, IsAvailable: true, StockQuantity: 20,
	}
	scarce := domain.MenuItem{
		ID: uuid.New(), VenueID: venue.ID, Name: "Четыре сыра",
		Price: 79000, IsAvailable: true, StockQuantity: 3,
	}

	cartWith := func(lines ...domain.CartLine) domain.Cart {
		return domain.Cart{ID: uuid.New(), CustomerID: uuid.New(), VenueID: venue.ID, Lines: lines}
	}
	menu := map[uuid.UUID]domain.MenuItem{available.ID: available, scarce.ID: scarce}

	t.Run("totals a healthy cart and clears it for checkout", func(t *testing.T) {
		t.Parallel()

		priced := domain.PriceCart(cartWith(
			domain.CartLine{MenuItemID: available.ID, Quantity: 2},
		), venue, menu)

		if priced.ItemsTotal != 118000 {
			t.Errorf("items total = %d, want 118000", priced.ItemsTotal)
		}
		if !priced.CheckoutReady {
			t.Errorf("cart should be checkout-ready, blockers: %v", priced.Blockers)
		}
	})

	t.Run("a line asking for more than the stock does not inflate the total", func(t *testing.T) {
		t.Parallel()

		priced := domain.PriceCart(cartWith(
			domain.CartLine{MenuItemID: available.ID, Quantity: 2},
			domain.CartLine{MenuItemID: scarce.ID, Quantity: 5},
		), venue, menu)

		if priced.CheckoutReady {
			t.Error("a cart with an unsatisfiable line must not be checkout-ready")
		}

		if priced.ItemsTotal != 118000 {
			t.Errorf("items total = %d, want 118000", priced.ItemsTotal)
		}

		unavailable := priced.UnavailableItems()
		if len(unavailable) != 1 {
			t.Fatalf("want one unavailable line, got %d", len(unavailable))
		}
		if unavailable[0].Reason != domain.ReasonInsufficientStock {
			t.Errorf("reason = %q, want insufficient_stock", unavailable[0].Reason)
		}
		if unavailable[0].Available != 3 || unavailable[0].Requested != 5 {
			t.Errorf("want requested 5 of 3 available, got %d of %d",
				unavailable[0].Requested, unavailable[0].Available)
		}
	})

	t.Run("an item that vanished from the menu is reported as withdrawn", func(t *testing.T) {
		t.Parallel()

		priced := domain.PriceCart(cartWith(
			domain.CartLine{MenuItemID: uuid.New(), Quantity: 1},
		), venue, menu)

		unavailable := priced.UnavailableItems()
		if len(unavailable) != 1 || unavailable[0].Reason != domain.ReasonWithdrawn {
			t.Fatalf("want a withdrawn line, got %+v", unavailable)
		}
	})

	t.Run("blocks checkout below the venue minimum", func(t *testing.T) {
		t.Parallel()

		priced := domain.PriceCart(cartWith(
			domain.CartLine{MenuItemID: available.ID, Quantity: 1},
		), venue, menu)

		if priced.CheckoutReady {
			t.Error("59000 is below the 70000 minimum; checkout must be blocked")
		}
		if len(priced.Blockers) == 0 {
			t.Error("the customer needs to be told why")
		}
	})

	t.Run("blocks checkout when the venue is closed", func(t *testing.T) {
		t.Parallel()

		closed := *venue
		closed.IsOpen = false

		priced := domain.PriceCart(cartWith(
			domain.CartLine{MenuItemID: available.ID, Quantity: 2},
		), &closed, menu)

		if priced.CheckoutReady {
			t.Error("a closed venue must block checkout")
		}
	})

	t.Run("an empty cart is not checkout-ready", func(t *testing.T) {
		t.Parallel()

		priced := domain.PriceCart(cartWith(), venue, menu)
		if priced.CheckoutReady {
			t.Error("an empty cart must not be checkout-ready")
		}
	})
}

func TestMenuItemCheckQuantity(t *testing.T) {
	t.Parallel()

	base := domain.MenuItem{Name: "dish", Price: 1000, IsAvailable: true, StockQuantity: 5}

	cases := []struct {
		name      string
		mutate    func(domain.MenuItem) domain.MenuItem
		requested int
		wantOK    bool
		wantWhy   domain.UnavailableReason
	}{
		{name: "within stock", requested: 5, wantOK: true},
		{name: "above stock", requested: 6, wantWhy: domain.ReasonInsufficientStock},
		{
			name:      "sold out",
			mutate:    func(i domain.MenuItem) domain.MenuItem { i.StockQuantity = 0; return i },
			requested: 1, wantWhy: domain.ReasonSoldOut,
		},
		{
			name:      "switched off by the kitchen",
			mutate:    func(i domain.MenuItem) domain.MenuItem { i.IsAvailable = false; return i },
			requested: 1, wantWhy: domain.ReasonDisabled,
		},
		{
			name: "withdrawn from the menu",
			mutate: func(i domain.MenuItem) domain.MenuItem {
				deleted := i.CreatedAt
				i.DeletedAt = &deleted
				return i
			},
			requested: 1, wantWhy: domain.ReasonWithdrawn,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			item := base
			if testCase.mutate != nil {
				item = testCase.mutate(item)
			}

			reason, ok := item.CheckQuantity(testCase.requested)
			if ok != testCase.wantOK {
				t.Fatalf("ok = %v, want %v (reason %q)", ok, testCase.wantOK, reason)
			}
			if !testCase.wantOK && reason != testCase.wantWhy {
				t.Fatalf("reason = %q, want %q", reason, testCase.wantWhy)
			}
		})
	}
}
