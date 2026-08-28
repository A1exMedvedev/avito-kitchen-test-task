package http

import (
	"context"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/service"
	"github.com/google/uuid"
)

type CatalogUseCase interface {
	ListVenues(ctx context.Context, filter service.VenueFilter) ([]domain.Venue, int, error)
	GetVenue(ctx context.Context, id uuid.UUID) (*domain.Venue, error)
	GetMenu(ctx context.Context, venueID uuid.UUID) (domain.Menu, error)
}

type CartUseCase interface {
	Get(ctx context.Context, customerID uuid.UUID) (domain.PricedCart, error)
	SetItem(ctx context.Context, customerID, venueID, menuItemID uuid.UUID, quantity int) (domain.PricedCart, error)
	Clear(ctx context.Context, customerID uuid.UUID) error
}

type OrderUseCase interface {
	Checkout(ctx context.Context, cmd service.CheckoutCommand) (*domain.Order, error)
	List(ctx context.Context, customerID uuid.UUID, statuses []domain.OrderStatus, page service.Page) ([]domain.Order, int, error)
	Get(ctx context.Context, customerID, orderID uuid.UUID) (*domain.Order, error)
	Cancel(ctx context.Context, customerID, orderID uuid.UUID, reason string) (*domain.Order, error)
}

type FulfillmentUseCase interface {
	List(ctx context.Context, venueID uuid.UUID, statuses []domain.OrderStatus, page service.Page) ([]domain.Order, int, error)
	Get(ctx context.Context, venueID, orderID uuid.UUID) (*domain.Order, error)
	Accept(ctx context.Context, venueID, orderID uuid.UUID, prepMinutes int) (*domain.Order, error)
	Reject(ctx context.Context, venueID, orderID uuid.UUID, reason string) (*domain.Order, error)
	UpdateStatus(ctx context.Context, venueID, orderID uuid.UUID, target domain.OrderStatus, reason string) (*domain.Order, error)
}

type PartnerUseCase interface {
	Authenticate(ctx context.Context, apiKey string) (*domain.Venue, error)
	GetVenue(ctx context.Context, venueID uuid.UUID) (*domain.Venue, error)
	UpdateVenue(ctx context.Context, venueID uuid.UUID, cmd service.UpdateVenueCommand) (*domain.Venue, error)
	GetMenu(ctx context.Context, venueID uuid.UUID) (domain.Menu, error)
	SaveCategory(ctx context.Context, venueID uuid.UUID, cmd service.SaveCategoryCommand) (*domain.MenuCategory, error)
	DeleteCategory(ctx context.Context, venueID, categoryID uuid.UUID) error
	CreateItem(ctx context.Context, venueID uuid.UUID, cmd service.CreateItemCommand) (*domain.MenuItem, error)
	UpdateItem(ctx context.Context, venueID, itemID uuid.UUID, cmd service.UpdateItemCommand) (*domain.MenuItem, error)
	SetStock(ctx context.Context, venueID, itemID uuid.UUID, quantity int, available *bool) (*domain.MenuItem, error)
	DeleteItem(ctx context.Context, venueID, itemID uuid.UUID) error
}
