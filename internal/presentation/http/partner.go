package http

import (
	"net/http"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/api/gen/openapi/kitchenapi"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/service"
)

func (s *server) GetPartnerVenue(w http.ResponseWriter, r *http.Request) {
	venue, err := VenueFrom(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}

	current, err := s.partners.GetVenue(r.Context(), venue.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, venueToAPI(*current))
}

func (s *server) UpdatePartnerVenue(w http.ResponseWriter, r *http.Request) {
	venue, err := VenueFrom(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}

	var body kitchenapi.UpdateVenueRequest
	if err := decodeBody(r, &body); err != nil {
		writeError(w, r, err)
		return
	}

	cmd := service.UpdateVenueCommand{
		IsOpen:         body.IsOpen,
		AvgPrepMinutes: body.AvgPrepMinutes,
		Description:    body.Description,
	}
	if body.MinOrderAmount != nil {
		cmd.MinOrderAmount = new(domain.Money(*body.MinOrderAmount))
	}
	if body.DeliveryFee != nil {
		cmd.DeliveryFee = new(domain.Money(*body.DeliveryFee))
	}

	updated, err := s.partners.UpdateVenue(r.Context(), venue.ID, cmd)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, venueToAPI(*updated))
}

func (s *server) GetPartnerMenu(w http.ResponseWriter, r *http.Request) {
	venue, err := VenueFrom(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}

	menu, err := s.partners.GetMenu(r.Context(), venue.ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, menuToAPI(menu))
}

func (s *server) CreateCategory(w http.ResponseWriter, r *http.Request) {
	venue, err := VenueFrom(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}

	var body kitchenapi.CategoryRequest
	if err := decodeBody(r, &body); err != nil {
		writeError(w, r, err)
		return
	}

	category, err := s.partners.SaveCategory(r.Context(), venue.ID, service.SaveCategoryCommand{
		Name:     body.Name,
		Position: intOrZero(body.Position),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, categoryToAPI(*category))
}

func (s *server) UpdateCategory(w http.ResponseWriter, r *http.Request, categoryID kitchenapi.CategoryId) {
	venue, err := VenueFrom(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}

	var body kitchenapi.CategoryRequest
	if err := decodeBody(r, &body); err != nil {
		writeError(w, r, err)
		return
	}

	category, err := s.partners.SaveCategory(r.Context(), venue.ID, service.SaveCategoryCommand{
		ID:       &categoryID,
		Name:     body.Name,
		Position: intOrZero(body.Position),
	})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, categoryToAPI(*category))
}

func (s *server) DeleteCategory(w http.ResponseWriter, r *http.Request, categoryID kitchenapi.CategoryId) {
	venue, err := VenueFrom(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	if err := s.partners.DeleteCategory(r.Context(), venue.ID, categoryID); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) CreateMenuItem(w http.ResponseWriter, r *http.Request) {
	venue, err := VenueFrom(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}

	var body kitchenapi.CreateMenuItemRequest
	if err := decodeBody(r, &body); err != nil {
		writeError(w, r, err)
		return
	}

	cmd := service.CreateItemCommand{
		CategoryID:  body.CategoryId,
		Name:        body.Name,
		Price:       domain.Money(body.PriceAmount),
		IsAvailable: true,
		Position:    intOrZero(body.Position),
		WeightGrams: intOrZero(body.WeightGrams),
	}
	if body.Description != nil {
		cmd.Description = *body.Description
	}
	if body.StockQuantity != nil {
		cmd.StockQuantity = *body.StockQuantity
	}
	if body.IsAvailable != nil {
		cmd.IsAvailable = *body.IsAvailable
	}

	item, err := s.partners.CreateItem(r.Context(), venue.ID, cmd)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, menuItemToAPI(*item))
}

func (s *server) UpdateMenuItem(w http.ResponseWriter, r *http.Request, itemID kitchenapi.ItemId) {
	venue, err := VenueFrom(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}

	var body kitchenapi.UpdateMenuItemRequest
	if err := decodeBody(r, &body); err != nil {
		writeError(w, r, err)
		return
	}

	cmd := service.UpdateItemCommand{
		CategoryID:  body.CategoryId,
		Name:        body.Name,
		Description: body.Description,
		IsAvailable: body.IsAvailable,
		WeightGrams: body.WeightGrams,
		Position:    body.Position,
	}
	if body.PriceAmount != nil {
		cmd.Price = new(domain.Money(*body.PriceAmount))
	}

	item, err := s.partners.UpdateItem(r.Context(), venue.ID, itemID, cmd)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, menuItemToAPI(*item))
}

func (s *server) SetMenuItemStock(w http.ResponseWriter, r *http.Request, itemID kitchenapi.ItemId) {
	venue, err := VenueFrom(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}

	var body kitchenapi.SetStockRequest
	if err := decodeBody(r, &body); err != nil {
		writeError(w, r, err)
		return
	}

	item, err := s.partners.SetStock(r.Context(), venue.ID, itemID, body.StockQuantity, body.IsAvailable)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, menuItemToAPI(*item))
}

func (s *server) DeleteMenuItem(w http.ResponseWriter, r *http.Request, itemID kitchenapi.ItemId) {
	venue, err := VenueFrom(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	if err := s.partners.DeleteItem(r.Context(), venue.ID, itemID); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) ListPartnerOrders(w http.ResponseWriter, r *http.Request, params kitchenapi.ListPartnerOrdersParams) {
	venue, err := VenueFrom(r.Context())
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
	orders, total, err := s.fulfillment.List(r.Context(), venue.ID, statuses, page)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, orderPageToAPI(orders, total, page.Limit, page.Offset))
}

func (s *server) GetPartnerOrder(w http.ResponseWriter, r *http.Request, orderID kitchenapi.OrderId) {
	venue, err := VenueFrom(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}

	order, err := s.fulfillment.Get(r.Context(), venue.ID, orderID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, orderDetailsToAPI(*order))
}

func (s *server) AcceptOrder(w http.ResponseWriter, r *http.Request, orderID kitchenapi.OrderId) {
	venue, err := VenueFrom(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}

	var body kitchenapi.AcceptOrderRequest
	if r.ContentLength > 0 {
		if err := decodeBody(r, &body); err != nil {
			writeError(w, r, err)
			return
		}
	}

	order, err := s.fulfillment.Accept(r.Context(), venue.ID, orderID, intOrZero(body.PrepMinutes))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, orderToAPI(*order))
}

func (s *server) RejectOrder(w http.ResponseWriter, r *http.Request, orderID kitchenapi.OrderId) {
	venue, err := VenueFrom(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}

	var body kitchenapi.RejectOrderRequest
	if err := decodeBody(r, &body); err != nil {
		writeError(w, r, err)
		return
	}

	order, err := s.fulfillment.Reject(r.Context(), venue.ID, orderID, body.Reason)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, orderToAPI(*order))
}

func (s *server) UpdateOrderStatus(w http.ResponseWriter, r *http.Request, orderID kitchenapi.OrderId) {
	venue, err := VenueFrom(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}

	var body kitchenapi.UpdateOrderStatusRequest
	if err := decodeBody(r, &body); err != nil {
		writeError(w, r, err)
		return
	}
	reason := ""
	if body.Reason != nil {
		reason = *body.Reason
	}

	order, err := s.fulfillment.UpdateStatus(
		r.Context(), venue.ID, orderID, domain.OrderStatus(body.Status), reason)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, orderToAPI(*order))
}

func intOrZero(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
