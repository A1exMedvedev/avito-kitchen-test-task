package service_test

import (
	"context"
	"sync"
	"time"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/service"
	"github.com/google/uuid"
)

type fakeStore struct {
	mu sync.Mutex

	venues    map[uuid.UUID]domain.Venue
	items     map[uuid.UUID]domain.MenuItem
	carts     map[uuid.UUID]*domain.Cart
	orders    map[uuid.UUID]*domain.Order
	customers map[uuid.UUID]domain.Customer
	outbox    []domain.OrderEvent

	notified int
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		venues:    make(map[uuid.UUID]domain.Venue),
		items:     make(map[uuid.UUID]domain.MenuItem),
		carts:     make(map[uuid.UUID]*domain.Cart),
		orders:    make(map[uuid.UUID]*domain.Order),
		customers: make(map[uuid.UUID]domain.Customer),
	}
}

type fakeTx struct{ store *fakeStore }

func (f fakeTx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

type fakeVenues struct{ store *fakeStore }

func (f fakeVenues) Get(_ context.Context, id uuid.UUID) (*domain.Venue, error) {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()

	venue, ok := f.store.venues[id]
	if !ok {
		return nil, domain.NotFound("venue_not_found", "venue not found")
	}
	return &venue, nil
}

func (f fakeVenues) List(_ context.Context, _ service.VenueFilter) ([]domain.Venue, int, error) {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()

	venues := make([]domain.Venue, 0, len(f.store.venues))
	for _, venue := range f.store.venues {
		venues = append(venues, venue)
	}
	return venues, len(venues), nil
}

func (f fakeVenues) Update(_ context.Context, venue *domain.Venue) error {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()

	f.store.venues[venue.ID] = *venue
	return nil
}

func (f fakeVenues) ResolveAPIKey(_ context.Context, key string) (*domain.Venue, error) {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()

	for _, venue := range f.store.venues {
		if venue.Slug == key {
			return &venue, nil
		}
	}
	return nil, domain.Unauthenticated("invalid_api_key", "unknown or revoked API key")
}

type fakeMenus struct{ store *fakeStore }

func (f fakeMenus) ListCategories(context.Context, uuid.UUID) ([]domain.MenuCategory, error) {
	return nil, nil
}

func (f fakeMenus) GetCategory(_ context.Context, _, categoryID uuid.UUID) (*domain.MenuCategory, error) {
	return &domain.MenuCategory{ID: categoryID}, nil
}

func (f fakeMenus) SaveCategory(context.Context, *domain.MenuCategory) error { return nil }

func (f fakeMenus) DeleteCategory(context.Context, uuid.UUID, uuid.UUID) error { return nil }

func (f fakeMenus) CountCategoryItems(context.Context, uuid.UUID, uuid.UUID) (int, error) {
	return 0, nil
}

func (f fakeMenus) ListItems(_ context.Context, venueID uuid.UUID, includeWithdrawn bool) ([]domain.MenuItem, error) {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()

	items := make([]domain.MenuItem, 0, len(f.store.items))
	for _, item := range f.store.items {
		if item.VenueID != venueID || (!includeWithdrawn && item.Withdrawn()) {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func (f fakeMenus) GetItem(_ context.Context, venueID, itemID uuid.UUID) (*domain.MenuItem, error) {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()

	item, ok := f.store.items[itemID]
	if !ok || item.VenueID != venueID {
		return nil, domain.NotFound("menu_item_not_found", "menu item not found")
	}
	return &item, nil
}

func (f fakeMenus) SaveItem(_ context.Context, item *domain.MenuItem) error {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()

	f.store.items[item.ID] = *item
	return nil
}

func (f fakeMenus) DeleteItem(_ context.Context, _, itemID uuid.UUID) error {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()

	item, ok := f.store.items[itemID]
	if !ok {
		return domain.NotFound("menu_item_not_found", "menu item not found")
	}
	now := time.Now()
	item.DeletedAt = &now
	f.store.items[itemID] = item
	return nil
}

func (f fakeMenus) GetItemsByIDs(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.MenuItem, error) {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()

	items := make(map[uuid.UUID]domain.MenuItem, len(ids))
	for _, id := range ids {
		if item, ok := f.store.items[id]; ok {
			items[id] = item
		}
	}
	return items, nil
}

func (f fakeMenus) LockItemsByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.MenuItem, error) {
	return f.GetItemsByIDs(ctx, ids)
}

func (f fakeMenus) AdjustStock(_ context.Context, adjustments []service.StockAdjustment) error {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()

	for _, adjustment := range adjustments {
		item, ok := f.store.items[adjustment.MenuItemID]
		if !ok {
			continue
		}
		if item.StockQuantity+adjustment.Delta < 0 {
			return domain.Conflict("items_unavailable",
				"another order took the last portions while you were checking out")
		}
		item.StockQuantity += adjustment.Delta
		f.store.items[adjustment.MenuItemID] = item
	}
	return nil
}

type fakeCarts struct{ store *fakeStore }

func (f fakeCarts) GetActive(_ context.Context, customerID uuid.UUID) (*domain.Cart, error) {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()

	cart, ok := f.store.carts[customerID]
	if !ok || cart.Status != domain.CartStatusActive {
		return nil, nil //nolint:nilnil
	}
	clone := *cart
	clone.Lines = append([]domain.CartLine(nil), cart.Lines...)
	return &clone, nil
}

func (f fakeCarts) Save(_ context.Context, cart *domain.Cart) error {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()

	clone := *cart
	clone.Lines = append([]domain.CartLine(nil), cart.Lines...)
	f.store.carts[cart.CustomerID] = &clone
	return nil
}

func (f fakeCarts) MarkOrdered(_ context.Context, cartID uuid.UUID) error {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()

	for _, cart := range f.store.carts {
		if cart.ID == cartID {
			cart.Status = domain.CartStatusOrdered
		}
	}
	return nil
}

func (f fakeCarts) DeleteActive(_ context.Context, customerID uuid.UUID) error {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()

	delete(f.store.carts, customerID)
	return nil
}

type fakeOrders struct{ store *fakeStore }

func (f fakeOrders) Create(_ context.Context, order *domain.Order) error {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()

	for _, existing := range f.store.orders {
		if existing.CustomerID == order.CustomerID &&
			existing.IdempotencyKey == order.IdempotencyKey {
			return domain.Conflict("order_duplicate", "duplicate idempotency key")
		}
	}
	clone := *order
	f.store.orders[order.ID] = &clone
	return nil
}

func (f fakeOrders) Get(_ context.Context, id uuid.UUID) (*domain.Order, error) {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()

	order, ok := f.store.orders[id]
	if !ok {
		return nil, domain.NotFound("order_not_found", "order not found")
	}
	clone := *order
	return &clone, nil
}

func (f fakeOrders) GetForUpdate(ctx context.Context, id uuid.UUID) (*domain.Order, error) {
	return f.Get(ctx, id)
}

func (f fakeOrders) GetByIdempotencyKey(_ context.Context, customerID uuid.UUID, key string) (*domain.Order, error) {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()

	for _, order := range f.store.orders {
		if order.CustomerID == customerID && order.IdempotencyKey == key {
			clone := *order
			return &clone, nil
		}
	}
	return nil, nil //nolint:nilnil
}

func (f fakeOrders) List(_ context.Context, filter service.OrderFilter) ([]domain.Order, int, error) {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()

	var matched []domain.Order
	for _, order := range f.store.orders {
		if filter.CustomerID != nil && order.CustomerID != *filter.CustomerID {
			continue
		}
		if filter.VenueID != nil && order.VenueID != *filter.VenueID {
			continue
		}
		if len(filter.Statuses) > 0 && !containsStatus(filter.Statuses, order.Status) {
			continue
		}
		matched = append(matched, *order)
	}
	return matched, len(matched), nil
}

func containsStatus(statuses []domain.OrderStatus, status domain.OrderStatus) bool {
	for _, candidate := range statuses {
		if candidate == status {
			return true
		}
	}
	return false
}

func (f fakeOrders) SaveStatus(_ context.Context, order *domain.Order, change domain.StatusChange) error {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()

	stored, ok := f.store.orders[order.ID]
	if !ok {
		return domain.NotFound("order_not_found", "order not found")
	}
	if stored.Version != order.Version {
		return domain.Conflict("order_modified", "the order changed while this request was in flight")
	}

	clone := *order
	clone.Version++
	clone.Timeline = append(append([]domain.StatusChange(nil), stored.Timeline...), change)
	f.store.orders[order.ID] = &clone
	order.Version = clone.Version
	return nil
}

type fakeCustomers struct{ store *fakeStore }

func (f fakeCustomers) Ensure(_ context.Context, id uuid.UUID) (*domain.Customer, error) {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()

	customer, ok := f.store.customers[id]
	if !ok {
		customer = domain.Customer{ID: id, CreatedAt: time.Now()}
		f.store.customers[id] = customer
	}
	return &customer, nil
}

func (f fakeCustomers) UpdateContacts(_ context.Context, id uuid.UUID, displayName, phone string) error {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()

	customer := f.store.customers[id]
	customer.ID = id
	if displayName != "" {
		customer.DisplayName = displayName
	}
	if phone != "" {
		customer.Phone = phone
	}
	f.store.customers[id] = customer
	return nil
}

type fakeOutbox struct{ store *fakeStore }

func (f fakeOutbox) Append(_ context.Context, events ...domain.OrderEvent) error {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()

	f.store.outbox = append(f.store.outbox, events...)
	return nil
}

func (f fakeOutbox) FetchUnpublished(context.Context, int) ([]domain.OrderEvent, error) {
	return nil, nil
}

func (f fakeOutbox) MarkPublished(context.Context, []uuid.UUID) error { return nil }

type fakeNotifier struct{ store *fakeStore }

func (f fakeNotifier) Notify() {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()

	f.store.notified++
}

type fixture struct {
	store       *fakeStore
	carts       service.CartService
	orders      service.OrderService
	fulfillment service.FulfillmentService
	partners    service.PartnerService
	now         time.Time
}

func newFixture() *fixture {
	store := newFakeStore()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	clock := service.Clock(func() time.Time { return now })

	venues := fakeVenues{store: store}
	menus := fakeMenus{store: store}
	carts := fakeCarts{store: store}
	orders := fakeOrders{store: store}
	customers := fakeCustomers{store: store}
	outbox := fakeOutbox{store: store}

	tx := fakeTx{store: store}
	notifier := fakeNotifier{store: store}

	return &fixture{
		store: store,
		carts: service.NewCartService(carts, menus, venues, customers, clock),
		orders: service.NewOrderService(
			tx, orders, carts, menus, venues, customers, outbox, notifier, clock),
		fulfillment: service.NewFulfillmentService(tx, orders, menus, outbox, notifier, clock),
		partners:    service.NewPartnerService(venues, menus, clock),
		now:         now,
	}
}

func (f *fixture) addVenue(venue domain.Venue) domain.Venue {
	if venue.ID == uuid.Nil {
		venue.ID = uuid.New()
	}
	f.store.mu.Lock()
	defer f.store.mu.Unlock()

	f.store.venues[venue.ID] = venue
	return venue
}

func (f *fixture) addItem(item domain.MenuItem) domain.MenuItem {
	if item.ID == uuid.Nil {
		item.ID = uuid.New()
	}
	f.store.mu.Lock()
	defer f.store.mu.Unlock()

	f.store.items[item.ID] = item
	return item
}

func (f *fixture) stockOf(itemID uuid.UUID) int {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()

	return f.store.items[itemID].StockQuantity
}

func (f *fixture) outboxEvents() []domain.OrderEvent {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()

	return append([]domain.OrderEvent(nil), f.store.outbox...)
}
