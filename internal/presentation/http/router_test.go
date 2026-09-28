package http_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"github.com/google/uuid"
)

func TestPublicEndpointsNeedNoCredentials(t *testing.T) {
	t.Parallel()

	handler := newHandler(t, stubs{})
	status, body := do(t, handler, http.MethodGet, "/api/v1/venues", "", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", status, body)
	}
	if !strings.Contains(body, "Хинкальная") {
		t.Errorf("venue name missing from the response: %s", body)
	}
	if !strings.Contains(body, `"accepts_orders":true`) {
		t.Errorf("the derived accepts_orders flag should be exposed: %s", body)
	}
}

func TestAuthenticationFollowsTheSpec(t *testing.T) {
	t.Parallel()

	handler := newHandler(t, stubs{})
	customerID := uuid.NewString()

	cases := []struct {
		name       string
		method     string
		path       string
		headers    map[string]string
		wantStatus int
		wantCode   string
	}{
		{
			name: "customer endpoint without a header", method: http.MethodGet, path: "/api/v1/cart",
			wantStatus: http.StatusUnauthorized, wantCode: "customer_required",
		},
		{
			name: "customer endpoint with a non-UUID header", method: http.MethodGet, path: "/api/v1/cart",
			headers:    map[string]string{"X-Customer-Id": "not-a-uuid"},
			wantStatus: http.StatusUnauthorized, wantCode: "customer_invalid",
		},
		{
			name: "customer endpoint with a valid header", method: http.MethodGet, path: "/api/v1/cart",
			headers:    map[string]string{"X-Customer-Id": customerID},
			wantStatus: http.StatusOK,
		},
		{
			name: "partner endpoint without a key", method: http.MethodGet, path: "/api/v1/partner/venue",
			wantStatus: http.StatusUnauthorized, wantCode: "api_key_required",
		},
		{
			name: "partner endpoint with an unknown key", method: http.MethodGet, path: "/api/v1/partner/venue",
			headers:    map[string]string{"X-Api-Key": "wrong"},
			wantStatus: http.StatusUnauthorized, wantCode: "invalid_api_key",
		},
		{
			name: "partner endpoint with a valid key", method: http.MethodGet, path: "/api/v1/partner/venue",
			headers:    map[string]string{"X-Api-Key": testAPIKey},
			wantStatus: http.StatusOK,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			status, body := do(t, handler, testCase.method, testCase.path, "", testCase.headers)
			if status != testCase.wantStatus {
				t.Fatalf("status = %d, want %d: %s", status, testCase.wantStatus, body)
			}
			if testCase.wantCode != "" && errorCode(t, body) != testCase.wantCode {
				t.Errorf("code = %q, want %q", errorCode(t, body), testCase.wantCode)
			}
		})
	}
}

func TestSchemaValidationRejectsBadRequests(t *testing.T) {
	t.Parallel()

	handler := newHandler(t, stubs{})
	customer := map[string]string{"X-Customer-Id": uuid.NewString()}

	cases := []struct {
		name       string
		method     string
		path       string
		body       string
		headers    map[string]string
		wantStatus int
	}{
		{
			name: "quantity above the documented maximum", method: http.MethodPut, path: "/api/v1/cart/items",
			body: `{"venue_id":"` + testVenueID.String() + `","menu_item_id":"` +
				testItemID.String() + `","quantity":500}`,
			headers: customer, wantStatus: http.StatusBadRequest,
		},
		{
			name: "missing required field", method: http.MethodPut, path: "/api/v1/cart/items",
			body:    `{"venue_id":"` + testVenueID.String() + `"}`,
			headers: customer, wantStatus: http.StatusBadRequest,
		},
		{
			name: "checkout without an Idempotency-Key", method: http.MethodPost, path: "/api/v1/orders",
			body:    `{"delivery":{"recipient_name":"a","phone":"+7900","address":"адрес"}}`,
			headers: customer, wantStatus: http.StatusBadRequest,
		},
		{
			name: "malformed UUID in the path", method: http.MethodGet, path: "/api/v1/venues/not-a-uuid",
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "unknown endpoint", method: http.MethodGet, path: "/api/v1/does-not-exist",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			status, body := do(t, handler, testCase.method, testCase.path, testCase.body, testCase.headers)
			if status != testCase.wantStatus {
				t.Fatalf("status = %d, want %d: %s", status, testCase.wantStatus, body)
			}
		})
	}
}

func TestDomainErrorsBecomeHTTPStatuses(t *testing.T) {
	t.Parallel()

	unavailable := &domain.Error{
		Kind:    domain.KindConflict,
		Code:    "items_unavailable",
		Message: "1 of 2 positions are no longer available",
		Items: []domain.UnavailableItem{{
			MenuItemID: testItemID, Name: "Качо-э-пепе",
			Requested: 2, Available: 0, Reason: domain.ReasonSoldOut,
		}},
	}

	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"not found", domain.NotFound("order_not_found", "order not found"), http.StatusNotFound, "order_not_found"},
		{"conflict", unavailable, http.StatusConflict, "items_unavailable"},
		{"unprocessable", domain.Unprocessable("cart_empty", "cart is empty"), http.StatusUnprocessableEntity, "cart_empty"},
		{"invalid", domain.Invalid("bad", "bad"), http.StatusBadRequest, "bad"},
		{"forbidden", domain.Forbidden("nope", "nope"), http.StatusForbidden, "nope"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			handler := newHandler(t, stubs{orders: stubOrders{err: testCase.err}})
			status, body := do(t, handler, http.MethodPost, "/api/v1/orders",
				`{"delivery":{"recipient_name":"Алексей","phone":"+79000000000","address":"Пятницкая, 20"}}`,
				map[string]string{
					"X-Customer-Id":   uuid.NewString(),
					"Idempotency-Key": "test-idempotency-key",
				})

			if status != testCase.wantStatus {
				t.Fatalf("status = %d, want %d: %s", status, testCase.wantStatus, body)
			}
			if got := errorCode(t, body); got != testCase.wantCode {
				t.Errorf("code = %q, want %q", got, testCase.wantCode)
			}
		})
	}

	t.Run("unavailable positions travel with the error", func(t *testing.T) {
		t.Parallel()

		handler := newHandler(t, stubs{orders: stubOrders{err: unavailable}})
		_, body := do(t, handler, http.MethodPost, "/api/v1/orders",
			`{"delivery":{"recipient_name":"Алексей","phone":"+79000000000","address":"Пятницкая, 20"}}`,
			map[string]string{
				"X-Customer-Id":   uuid.NewString(),
				"Idempotency-Key": "test-idempotency-key",
			})

		var payload struct {
			UnavailableItems []struct {
				Name   string `json:"name"`
				Reason string `json:"reason"`
			} `json:"unavailable_items"`
		}
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			t.Fatalf("decoding failed: %v", err)
		}
		if len(payload.UnavailableItems) != 1 || payload.UnavailableItems[0].Reason != "sold_out" {
			t.Fatalf("the client cannot repair the cart from this: %s", body)
		}
	})
}

func TestInternalErrorsAreNotLeaked(t *testing.T) {
	t.Parallel()

	handler := newHandler(t, stubs{
		catalog: stubCatalog{err: domain.Internal(errSecret)},
	})
	status, body := do(t, handler, http.MethodGet, "/api/v1/venues", "", nil)

	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", status)
	}
	if strings.Contains(body, "connection refused to postgres") {
		t.Fatalf("the internal cause leaked to the client: %s", body)
	}
	if errorCode(t, body) != "internal_error" {
		t.Errorf("code = %q, want internal_error", errorCode(t, body))
	}
}

func TestOpsEndpoints(t *testing.T) {
	t.Parallel()

	handler := newHandler(t, stubs{})

	if status, _ := do(t, handler, http.MethodGet, "/healthz", "", nil); status != http.StatusOK {
		t.Errorf("/healthz = %d, want 200", status)
	}
	if status, _ := do(t, handler, http.MethodGet, "/readyz", "", nil); status != http.StatusOK {
		t.Errorf("/readyz = %d, want 200", status)
	}

	status, body := do(t, handler, http.MethodGet, "/openapi.json", "", nil)
	if status != http.StatusOK || !strings.Contains(body, "API Кухни") {
		t.Errorf("/openapi.json = %d: %s", status, body[:min(200, len(body))])
	}
}
