package http

import (
	"net/http"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/api/gen/openapi/kitchenapi"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/service"
)

func (s *server) ListVenues(w http.ResponseWriter, r *http.Request, params kitchenapi.ListVenuesParams) {
	filter := service.VenueFilter{OpenNow: params.OpenNow, Page: pageFrom(params.Limit, params.Offset)}
	if params.City != nil {
		filter.City = *params.City
	}
	if params.Cuisine != nil {
		filter.Cuisine = *params.Cuisine
	}
	if params.Q != nil {
		filter.Query = *params.Q
	}

	venues, total, err := s.catalog.ListVenues(r.Context(), filter)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK,
		venuePageToAPI(venues, total, filter.Page.Limit, filter.Page.Offset))
}

func (s *server) GetVenue(w http.ResponseWriter, r *http.Request, venueID kitchenapi.VenueId) {
	venue, err := s.catalog.GetVenue(r.Context(), venueID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, venueToAPI(*venue))
}

func (s *server) GetVenueMenu(w http.ResponseWriter, r *http.Request, venueID kitchenapi.VenueId) {
	menu, err := s.catalog.GetMenu(r.Context(), venueID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, menuToAPI(menu))
}

func (s *server) GetCart(w http.ResponseWriter, r *http.Request) {
	customerID, err := CustomerFrom(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}

	cart, err := s.carts.Get(r.Context(), customerID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, cartToAPI(cart))
}

func (s *server) SetCartItem(w http.ResponseWriter, r *http.Request) {
	customerID, err := CustomerFrom(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}

	var body kitchenapi.SetCartItemRequest
	if err := decodeBody(r, &body); err != nil {
		writeError(w, r, err)
		return
	}

	cart, err := s.carts.SetItem(r.Context(), customerID, body.VenueId, body.MenuItemId, body.Quantity)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, cartToAPI(cart))
}

func (s *server) ClearCart(w http.ResponseWriter, r *http.Request) {
	customerID, err := CustomerFrom(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	if err := s.carts.Clear(r.Context(), customerID); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) CreateOrder(w http.ResponseWriter, r *http.Request, params kitchenapi.CreateOrderParams) {
	customerID, err := CustomerFrom(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}

	var body kitchenapi.CreateOrderRequest
	if err := decodeBody(r, &body); err != nil {
		writeError(w, r, err)
		return
	}

	order, err := s.orders.Checkout(r.Context(), service.CheckoutCommand{
		CustomerID:     customerID,
		IdempotencyKey: params.IdempotencyKey,
		Delivery:       deliveryFrom(body.Delivery),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, orderToAPI(*order))
}

func (s *server) ListOrders(w http.ResponseWriter, r *http.Request, params kitchenapi.ListOrdersParams) {
	customerID, err := CustomerFrom(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	statuses, err := statusesFrom(params.Status)
	if err != nil {
		writeError(w, r, err)
		return
	}

	page := pageFrom(params.Limit, params.Offset)
	orders, total, err := s.orders.List(r.Context(), customerID, statuses, page)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, orderPageToAPI(orders, total, page.Limit, page.Offset))
}

func (s *server) GetOrder(w http.ResponseWriter, r *http.Request, orderID kitchenapi.OrderId) {
	customerID, err := CustomerFrom(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}

	order, err := s.orders.Get(r.Context(), customerID, orderID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, orderDetailsToAPI(*order))
}

func (s *server) CancelOrder(w http.ResponseWriter, r *http.Request, orderID kitchenapi.OrderId) {
	customerID, err := CustomerFrom(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}

	var body kitchenapi.CancelOrderRequest
	if r.ContentLength > 0 {
		if err := decodeBody(r, &body); err != nil {
			writeError(w, r, err)
			return
		}
	}
	reason := ""
	if body.Reason != nil {
		reason = *body.Reason
	}

	order, err := s.orders.Cancel(r.Context(), customerID, orderID, reason)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, orderToAPI(*order))
}
