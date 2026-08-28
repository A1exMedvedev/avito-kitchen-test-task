package service

import (
	"context"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"github.com/google/uuid"
)

type cartService struct {
	carts     CartRepository
	menus     MenuRepository
	venues    VenueRepository
	customers CustomerRepository
	clock     Clock
}

func NewCartService(
	carts CartRepository,
	menus MenuRepository,
	venues VenueRepository,
	customers CustomerRepository,
	clock Clock,
) CartService {
	return &cartService{carts: carts, menus: menus, venues: venues, customers: customers, clock: clock}
}

func (s *cartService) Get(ctx context.Context, customerID uuid.UUID) (domain.PricedCart, error) {
	cart, err := s.carts.GetActive(ctx, customerID)
	if err != nil {
		return domain.PricedCart{}, err
	}
	if cart == nil {
		return domain.PricedCart{
			Cart:  domain.Cart{CustomerID: customerID, Status: domain.CartStatusActive},
			Lines: []domain.PricedLine{},
		}, nil
	}
	return s.price(ctx, cart)
}

func (s *cartService) SetItem(
	ctx context.Context,
	customerID, venueID, menuItemID uuid.UUID,
	quantity int,
) (domain.PricedCart, error) {
	if _, err := s.customers.Ensure(ctx, customerID); err != nil {
		return domain.PricedCart{}, err
	}

	venue, err := s.venues.Get(ctx, venueID)
	if err != nil {
		return domain.PricedCart{}, err
	}
	item, err := s.menus.GetItem(ctx, venueID, menuItemID)
	if err != nil {
		return domain.PricedCart{}, err
	}
	if item.Withdrawn() {
		return domain.PricedCart{}, domain.NotFound("menu_item_not_found", "menu item not found")
	}

	cart, err := s.carts.GetActive(ctx, customerID)
	if err != nil {
		return domain.PricedCart{}, err
	}
	if cart == nil {
		now := s.clock()
		cart = &domain.Cart{
			ID:         uuid.New(),
			CustomerID: customerID,
			VenueID:    venue.ID,
			Status:     domain.CartStatusActive,
			CreatedAt:  now,
			UpdatedAt:  now,
		}
	}
	if err := cart.EnsureVenue(venue.ID); err != nil {
		return domain.PricedCart{}, err
	}
	cart.VenueID = venue.ID

	if err := cart.SetLine(menuItemID, quantity); err != nil {
		return domain.PricedCart{}, err
	}
	cart.UpdatedAt = s.clock()

	if err := s.carts.Save(ctx, cart); err != nil {
		return domain.PricedCart{}, err
	}
	return s.price(ctx, cart)
}

func (s *cartService) Clear(ctx context.Context, customerID uuid.UUID) error {
	return s.carts.DeleteActive(ctx, customerID)
}

func (s *cartService) price(ctx context.Context, cart *domain.Cart) (domain.PricedCart, error) {
	if len(cart.Lines) == 0 {
		return domain.PricedCart{Cart: *cart, Lines: []domain.PricedLine{}}, nil
	}

	venue, err := s.venues.Get(ctx, cart.VenueID)
	if err != nil {
		return domain.PricedCart{}, err
	}

	ids := make([]uuid.UUID, 0, len(cart.Lines))
	for _, line := range cart.Lines {
		ids = append(ids, line.MenuItemID)
	}
	items, err := s.menus.GetItemsByIDs(ctx, ids)
	if err != nil {
		return domain.PricedCart{}, err
	}
	return domain.PriceCart(*cart, venue, items), nil
}
