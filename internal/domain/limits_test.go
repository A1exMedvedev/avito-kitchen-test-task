package domain_test

import (
	"strings"
	"testing"
	"time"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"github.com/google/uuid"
)

func TestMenuItemValidateBounds(t *testing.T) {
	t.Parallel()

	valid := domain.MenuItem{
		CategoryID:    uuid.New(),
		Name:          "Хинкали",
		Price:         45000,
		StockQuantity: 10,
		WeightGrams:   500,
	}

	cases := []struct {
		name     string
		mutate   func(domain.MenuItem) domain.MenuItem
		wantCode string
	}{
		{name: "valid item passes"},
		{
			name:     "empty name",
			mutate:   func(i domain.MenuItem) domain.MenuItem { i.Name = ""; return i },
			wantCode: "item_name_required",
		},
		{
			name: "name over the limit",
			mutate: func(i domain.MenuItem) domain.MenuItem {
				i.Name = strings.Repeat("а", domain.MaxItemNameLength+1)
				return i
			},
			wantCode: "item_name_too_long",
		},
		{
			name: "name exactly at the limit is fine",
			mutate: func(i domain.MenuItem) domain.MenuItem {
				i.Name = strings.Repeat("а", domain.MaxItemNameLength)
				return i
			},
		},
		{
			name: "description over the limit",
			mutate: func(i domain.MenuItem) domain.MenuItem {
				i.Description = strings.Repeat("б", domain.MaxItemDescriptionLength+1)
				return i
			},
			wantCode: "item_description_too_long",
		},
		{
			name:     "zero price",
			mutate:   func(i domain.MenuItem) domain.MenuItem { i.Price = 0; return i },
			wantCode: "item_price_invalid",
		},
		{
			name:     "negative stock",
			mutate:   func(i domain.MenuItem) domain.MenuItem { i.StockQuantity = -1; return i },
			wantCode: "item_stock_invalid",
		},
		{
			name: "stock over the cap",
			mutate: func(i domain.MenuItem) domain.MenuItem {
				i.StockQuantity = domain.MaxItemStockQuantity + 1
				return i
			},
			wantCode: "item_stock_invalid",
		},
		{
			name:     "negative weight",
			mutate:   func(i domain.MenuItem) domain.MenuItem { i.WeightGrams = -1; return i },
			wantCode: "item_weight_invalid",
		},
		{
			name:     "negative position",
			mutate:   func(i domain.MenuItem) domain.MenuItem { i.Position = -1; return i },
			wantCode: "item_position_invalid",
		},
		{
			name:     "no category",
			mutate:   func(i domain.MenuItem) domain.MenuItem { i.CategoryID = uuid.Nil; return i },
			wantCode: "item_category_required",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			item := valid
			if testCase.mutate != nil {
				item = testCase.mutate(item)
			}
			assertCode(t, item.Validate(), testCase.wantCode)
		})
	}
}

func TestMenuCategoryValidate(t *testing.T) {
	t.Parallel()

	valid := domain.MenuCategory{Name: "Хинкали", Position: 1}

	assertCode(t, valid.Validate(), "")

	empty := valid
	empty.Name = ""
	assertCode(t, empty.Validate(), "category_name_required")

	long := valid
	long.Name = strings.Repeat("а", domain.MaxCategoryNameLength+1)
	assertCode(t, long.Validate(), "category_name_too_long")

	negative := valid
	negative.Position = -1
	assertCode(t, negative.Validate(), "category_position_invalid")
}

func TestVenueValidateBounds(t *testing.T) {
	t.Parallel()

	valid := domain.Venue{
		Name:           "Хинкальная №1",
		AvgPrepMinutes: 25,
		MinOrderAmount: 50000,
		DeliveryFee:    19900,
	}

	cases := []struct {
		name     string
		mutate   func(domain.Venue) domain.Venue
		wantCode string
	}{
		{name: "valid venue passes"},
		{
			name:     "prep time below the floor",
			mutate:   func(v domain.Venue) domain.Venue { v.AvgPrepMinutes = domain.MinPrepMinutes - 1; return v },
			wantCode: "prep_minutes_invalid",
		},
		{
			name:     "prep time above the ceiling",
			mutate:   func(v domain.Venue) domain.Venue { v.AvgPrepMinutes = domain.MaxPrepMinutes + 1; return v },
			wantCode: "prep_minutes_invalid",
		},
		{
			name:   "prep time exactly at the bounds is fine",
			mutate: func(v domain.Venue) domain.Venue { v.AvgPrepMinutes = domain.MaxPrepMinutes; return v },
		},
		{
			name:     "negative minimum order",
			mutate:   func(v domain.Venue) domain.Venue { v.MinOrderAmount = -1; return v },
			wantCode: "min_order_invalid",
		},
		{
			name:   "zero minimum order is fine",
			mutate: func(v domain.Venue) domain.Venue { v.MinOrderAmount = 0; return v },
		},
		{
			name:     "negative delivery fee",
			mutate:   func(v domain.Venue) domain.Venue { v.DeliveryFee = -1; return v },
			wantCode: "delivery_fee_invalid",
		},
		{
			name: "description over the limit",
			mutate: func(v domain.Venue) domain.Venue {
				v.Description = strings.Repeat("а", domain.MaxVenueDescriptionLength+1)
				return v
			},
			wantCode: "venue_description_too_long",
		},
		{
			name: "description exactly at the limit is fine",
			mutate: func(v domain.Venue) domain.Venue {
				v.Description = strings.Repeat("а", domain.MaxVenueDescriptionLength)
				return v
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			venue := valid
			if testCase.mutate != nil {
				venue = testCase.mutate(venue)
			}
			assertCode(t, venue.Validate(), testCase.wantCode)
		})
	}
}

func TestDeliveryValidateBounds(t *testing.T) {
	t.Parallel()

	valid := domain.Delivery{
		RecipientName: "Алексей",
		Phone:         "+7 900 000-00-00",
		Address:       "Москва, ул. Пятницкая, 20",
	}

	assertCode(t, valid.Validate(), "")

	cases := map[string]struct {
		mutate   func(domain.Delivery) domain.Delivery
		wantCode string
	}{
		"no name": {
			func(d domain.Delivery) domain.Delivery { d.RecipientName = ""; return d },
			"delivery_name_required",
		},
		"name too long": {
			func(d domain.Delivery) domain.Delivery {
				d.RecipientName = strings.Repeat("а", domain.MaxRecipientNameLength+1)
				return d
			},
			"delivery_name_too_long",
		},
		"no phone": {
			func(d domain.Delivery) domain.Delivery { d.Phone = ""; return d },
			"delivery_phone_required",
		},
		"phone too long": {
			func(d domain.Delivery) domain.Delivery {
				d.Phone = strings.Repeat("9", domain.MaxPhoneLength+1)
				return d
			},
			"delivery_phone_too_long",
		},
		"address too short": {
			func(d domain.Delivery) domain.Delivery { d.Address = "ул"; return d },
			"delivery_address_required",
		},
		"address too long": {
			func(d domain.Delivery) domain.Delivery {
				d.Address = strings.Repeat("а", domain.MaxAddressLength+1)
				return d
			},
			"delivery_address_too_long",
		},
		"comment too long": {
			func(d domain.Delivery) domain.Delivery {
				d.Comment = strings.Repeat("а", domain.MaxDeliveryCommentLength+1)
				return d
			},
			"delivery_comment_too_long",
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assertCode(t, testCase.mutate(valid).Validate(), testCase.wantCode)
		})
	}
}

func TestStatusReasonLength(t *testing.T) {
	t.Parallel()

	order := &domain.Order{ID: uuid.New(), Status: domain.OrderStatusCreated}
	_, err := order.TransitionTo(domain.OrderStatusRejected, domain.ActorVenue,
		strings.Repeat("а", domain.MaxStatusReasonLength+1), time.Now())

	assertCode(t, err, "status_reason_too_long")
	if order.Status != domain.OrderStatusCreated {
		t.Fatalf("status changed despite a rejected transition: %q", order.Status)
	}
}

func assertCode(t *testing.T, err error, wantCode string) {
	t.Helper()

	if wantCode == "" {
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		return
	}
	domainErr, ok := domain.AsError(err)
	if !ok {
		t.Fatalf("expected a domain error, got %v", err)
	}
	if domainErr.Code != wantCode {
		t.Fatalf("code = %q, want %q", domainErr.Code, wantCode)
	}
}
