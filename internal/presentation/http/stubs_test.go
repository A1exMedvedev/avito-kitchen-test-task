package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	kitchenhttp "backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/presentation/http"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/service"
	"github.com/google/uuid"
)

var (
	testVenueID = uuid.MustParse("11111111-1111-4111-8111-111111111111")
	testItemID  = uuid.MustParse("22222222-2222-4222-8222-222222222222")
	testOrderID = uuid.MustParse("33333333-3333-4333-8333-333333333333")
	testAPIKey  = "test-partner-key"
)

func testVenue() domain.Venue {
	return domain.Venue{
		ID: testVenueID, Slug: "hinkalnaya-1", Name: "Хинкальная №1",
		Cuisines: []string{"georgian"}, City: "Москва", Address: "Пятницкая, 12",
		Status: domain.VenueStatusActive, IsOpen: true,
		AvgPrepMinutes: 25, MinOrderAmount: 50000, DeliveryFee: 19900, Rating: 4.8,
	}
}

type stubCatalog struct {
	venues []domain.Venue
	err    error
}

func (s stubCatalog) ListVenues(context.Context, service.VenueFilter) ([]domain.Venue, int, error) {
	return s.venues, len(s.venues), s.err
}

func (s stubCatalog) GetVenue(context.Context, uuid.UUID) (*domain.Venue, error) {
	if s.err != nil {
		return nil, s.err
	}
	venue := testVenue()
	return &venue, nil
}

func (s stubCatalog) GetMenu(context.Context, uuid.UUID) (domain.Menu, error) {
	return domain.Menu{VenueID: testVenueID}, s.err
}

type stubCart struct {
	cart domain.PricedCart
	err  error
}

func (s stubCart) Get(context.Context, uuid.UUID) (domain.PricedCart, error) {
	return s.cart, s.err
}

func (s stubCart) SetItem(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, int) (domain.PricedCart, error) {
	return s.cart, s.err
}

func (s stubCart) Clear(context.Context, uuid.UUID) error { return s.err }

type stubOrders struct {
	order *domain.Order
	err   error
}

func (s stubOrders) Checkout(context.Context, service.CheckoutCommand) (*domain.Order, error) {
	return s.order, s.err
}

func (s stubOrders) List(context.Context, uuid.UUID, []domain.OrderStatus, service.Page) ([]domain.Order, int, error) {
	if s.err != nil {
		return nil, 0, s.err
	}
	return []domain.Order{*s.order}, 1, nil
}

func (s stubOrders) Get(context.Context, uuid.UUID, uuid.UUID) (*domain.Order, error) {
	return s.order, s.err
}

func (s stubOrders) Cancel(context.Context, uuid.UUID, uuid.UUID, string) (*domain.Order, error) {
	return s.order, s.err
}

type stubFulfillment struct{ err error }

func (s stubFulfillment) List(context.Context, uuid.UUID, []domain.OrderStatus, service.Page) ([]domain.Order, int, error) {
	return nil, 0, s.err
}

func (s stubFulfillment) Get(context.Context, uuid.UUID, uuid.UUID) (*domain.Order, error) {
	return nil, s.err
}

func (s stubFulfillment) Accept(context.Context, uuid.UUID, uuid.UUID, int) (*domain.Order, error) {
	return nil, s.err
}

func (s stubFulfillment) Reject(context.Context, uuid.UUID, uuid.UUID, string) (*domain.Order, error) {
	return nil, s.err
}

func (s stubFulfillment) UpdateStatus(context.Context, uuid.UUID, uuid.UUID, domain.OrderStatus, string) (*domain.Order, error) {
	return nil, s.err
}

type stubPartners struct{}

func (stubPartners) Authenticate(_ context.Context, apiKey string) (*domain.Venue, error) {
	if apiKey == "" {
		return nil, domain.Unauthenticated("api_key_required", "X-Api-Key header is required")
	}
	if apiKey != testAPIKey {
		return nil, domain.Unauthenticated("invalid_api_key", "unknown or revoked API key")
	}
	venue := testVenue()
	return &venue, nil
}

func (stubPartners) GetVenue(context.Context, uuid.UUID) (*domain.Venue, error) {
	venue := testVenue()
	return &venue, nil
}

func (stubPartners) UpdateVenue(context.Context, uuid.UUID, service.UpdateVenueCommand) (*domain.Venue, error) {
	venue := testVenue()
	return &venue, nil
}

func (stubPartners) GetMenu(context.Context, uuid.UUID) (domain.Menu, error) {
	return domain.Menu{VenueID: testVenueID}, nil
}

func (stubPartners) SaveCategory(context.Context, uuid.UUID, service.SaveCategoryCommand) (*domain.MenuCategory, error) {
	return &domain.MenuCategory{ID: uuid.New(), Name: "Хинкали"}, nil
}

func (stubPartners) DeleteCategory(context.Context, uuid.UUID, uuid.UUID) error { return nil }

func (stubPartners) CreateItem(context.Context, uuid.UUID, service.CreateItemCommand) (*domain.MenuItem, error) {
	return &domain.MenuItem{ID: testItemID, Price: 45000}, nil
}

func (stubPartners) UpdateItem(context.Context, uuid.UUID, uuid.UUID, service.UpdateItemCommand) (*domain.MenuItem, error) {
	return &domain.MenuItem{ID: testItemID, Price: 45000}, nil
}

func (stubPartners) SetStock(context.Context, uuid.UUID, uuid.UUID, int, *bool) (*domain.MenuItem, error) {
	return &domain.MenuItem{ID: testItemID, Price: 45000, StockQuantity: 12, IsAvailable: true}, nil
}

func (stubPartners) DeleteItem(context.Context, uuid.UUID, uuid.UUID) error { return nil }

type stubs struct {
	catalog     kitchenhttp.CatalogUseCase
	carts       kitchenhttp.CartUseCase
	orders      kitchenhttp.OrderUseCase
	fulfillment kitchenhttp.FulfillmentUseCase
}

func newHandler(t *testing.T, override stubs) http.Handler {
	t.Helper()

	if override.catalog == nil {
		override.catalog = stubCatalog{venues: []domain.Venue{testVenue()}}
	}
	if override.carts == nil {
		override.carts = stubCart{}
	}
	if override.orders == nil {
		override.orders = stubOrders{order: &domain.Order{
			ID: testOrderID, Number: "AK-TEST12", VenueID: testVenueID,
			Status: domain.OrderStatusCreated, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}}
	}
	if override.fulfillment == nil {
		override.fulfillment = stubFulfillment{}
	}

	partners := stubPartners{}
	handler, err := kitchenhttp.NewRouter(
		kitchenhttp.NewServer(override.catalog, override.carts, override.orders,
			override.fulfillment, partners),
		partners,
		kitchenhttp.RouterOptions{Logger: discardLogger(), Version: "test"},
	)
	if err != nil {
		t.Fatalf("building the router failed: %v", err)
	}
	return handler
}

func do(t *testing.T, handler http.Handler, method, path, body string, headers map[string]string) (int, string) {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	request := httptest.NewRequest(method, path, reader)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder.Code, recorder.Body.String()
}

func errorCode(t *testing.T, body string) string {
	t.Helper()

	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("response is not an error envelope: %s", body)
	}
	return payload.Code
}

var errSecret = errors.New("connection refused to postgres at 10.0.0.5:5432")

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
