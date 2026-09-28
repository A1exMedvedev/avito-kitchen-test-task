package service

import (
	"context"
	"time"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"github.com/google/uuid"
)

type Clock func() time.Time

func SystemClock() time.Time { return time.Now().UTC() }

type Page struct {
	Limit  int
	Offset int
}

const (
	defaultPageLimit = 20
	maxPageLimit     = 100
)

func (p Page) Normalize() Page {
	if p.Limit <= 0 {
		p.Limit = defaultPageLimit
	}
	if p.Limit > maxPageLimit {
		p.Limit = maxPageLimit
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	return p
}

type VenueFilter struct {
	City    string
	Cuisine string
	Query   string

	OpenNow *bool
	Page    Page
}

type OrderFilter struct {
	CustomerID *uuid.UUID
	VenueID    *uuid.UUID
	Statuses   []domain.OrderStatus
	Page       Page
}

func validateStatusFilter(statuses []domain.OrderStatus) error {
	if len(statuses) > domain.MaxOrderStatusFilterItems {
		return domain.Invalid("status_filter_too_long",
			"at most %d statuses may be requested at once", domain.MaxOrderStatusFilterItems)
	}
	for _, status := range statuses {
		if !status.Valid() {
			return domain.Invalid("invalid_status", "unknown order status %q", status)
		}
	}
	return nil
}

type StockAdjustment struct {
	MenuItemID uuid.UUID
	Delta      int
}

type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type VenueRepository interface {
	Get(ctx context.Context, id uuid.UUID) (*domain.Venue, error)
	List(ctx context.Context, filter VenueFilter) ([]domain.Venue, int, error)
	Update(ctx context.Context, venue *domain.Venue) error

	ResolveAPIKey(ctx context.Context, key string) (*domain.Venue, error)
}

type MenuRepository interface {
	ListCategories(ctx context.Context, venueID uuid.UUID) ([]domain.MenuCategory, error)
	GetCategory(ctx context.Context, venueID, categoryID uuid.UUID) (*domain.MenuCategory, error)
	SaveCategory(ctx context.Context, category *domain.MenuCategory) error
	DeleteCategory(ctx context.Context, venueID, categoryID uuid.UUID) error
	CountCategoryItems(ctx context.Context, venueID, categoryID uuid.UUID) (int, error)

	ListItems(ctx context.Context, venueID uuid.UUID, includeWithdrawn bool) ([]domain.MenuItem, error)
	GetItem(ctx context.Context, venueID, itemID uuid.UUID) (*domain.MenuItem, error)
	SaveItem(ctx context.Context, item *domain.MenuItem) error
	DeleteItem(ctx context.Context, venueID, itemID uuid.UUID) error

	GetItemsByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.MenuItem, error)

	LockItemsByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.MenuItem, error)

	AdjustStock(ctx context.Context, adjustments []StockAdjustment) error
}

type CartRepository interface {
	GetActive(ctx context.Context, customerID uuid.UUID) (*domain.Cart, error)
	Save(ctx context.Context, cart *domain.Cart) error
	MarkOrdered(ctx context.Context, cartID uuid.UUID) error
	DeleteActive(ctx context.Context, customerID uuid.UUID) error
}

type OrderRepository interface {
	Create(ctx context.Context, order *domain.Order) error
	Get(ctx context.Context, id uuid.UUID) (*domain.Order, error)

	GetForUpdate(ctx context.Context, id uuid.UUID) (*domain.Order, error)
	GetByIdempotencyKey(ctx context.Context, customerID uuid.UUID, key string) (*domain.Order, error)
	List(ctx context.Context, filter OrderFilter) ([]domain.Order, int, error)

	SaveStatus(ctx context.Context, order *domain.Order, change domain.StatusChange) error
}

type CustomerRepository interface {
	Ensure(ctx context.Context, id uuid.UUID) (*domain.Customer, error)
	UpdateContacts(ctx context.Context, id uuid.UUID, displayName, phone string) error
}

type OutboxRepository interface {
	Append(ctx context.Context, events ...domain.OrderEvent) error

	FetchUnpublished(ctx context.Context, limit int) ([]domain.OrderEvent, error)
	MarkPublished(ctx context.Context, ids []uuid.UUID) error
}

type OutboxNotifier interface {
	Notify()
}

type EventPublisher interface {
	Publish(ctx context.Context, events ...domain.OrderEvent)
}

type EventSubscriber interface {
	Subscribe(venueID uuid.UUID) (<-chan domain.OrderEvent, func())
}
