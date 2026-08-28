package partner

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"time"
)

type PolicyPayload struct {
	AutoAccept      *bool   `json:"auto_accept,omitempty"`
	AutoRejectOver  *int64  `json:"auto_reject_over,omitempty"`
	PrepMinutes     *int    `json:"prep_minutes,omitempty"`
	AcceptDelay     *string `json:"accept_delay,omitempty"`
	CookingDelay    *string `json:"cooking_delay,omitempty"`
	ReadyDelay      *string `json:"ready_delay,omitempty"`
	DispatchDelay   *string `json:"dispatch_delay,omitempty"`
	DeliveryDelay   *string `json:"delivery_delay,omitempty"`
	RestockInterval *string `json:"restock_interval,omitempty"`
	RestockTo       *int    `json:"restock_to,omitempty"`
}

type policyView struct {
	AutoAccept      bool   `json:"auto_accept"`
	AutoRejectOver  int64  `json:"auto_reject_over"`
	PrepMinutes     int    `json:"prep_minutes"`
	AcceptDelay     string `json:"accept_delay"`
	CookingDelay    string `json:"cooking_delay"`
	ReadyDelay      string `json:"ready_delay"`
	DispatchDelay   string `json:"dispatch_delay"`
	DeliveryDelay   string `json:"delivery_delay"`
	RestockInterval string `json:"restock_interval"`
	RestockTo       int    `json:"restock_to"`
}

type orderView struct {
	ID         string       `json:"id"`
	Number     string       `json:"number"`
	Status     string       `json:"status"`
	Total      int64        `json:"total"`
	Items      []itemView   `json:"items"`
	Delivery   deliveryView `json:"delivery"`
	ReceivedAt time.Time    `json:"received_at"`
	UpdatedAt  time.Time    `json:"updated_at"`
	AutoPilot  bool         `json:"auto_pilot"`
}

type itemView struct {
	Name      string `json:"name"`
	Quantity  int32  `json:"quantity"`
	UnitPrice int64  `json:"unit_price"`
}

type deliveryView struct {
	Recipient string `json:"recipient_name"`
	Phone     string `json:"phone"`
	Address   string `json:"address"`
	Comment   string `json:"comment,omitempty"`
}

type manualActionRequest struct {
	Reason      string `json:"reason,omitempty"`
	PrepMinutes int    `json:"prep_minutes,omitempty"`
}

func NewHTTPHandler(kitchen Venue, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		respond(w, logger, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /kds", func(w http.ResponseWriter, _ *http.Request) {
		snapshot := kitchen.Snapshot()
		respond(w, logger, http.StatusOK, map[string]any{
			"venue_id":   snapshot.VenueID,
			"venue_name": snapshot.VenueName,
			"policy":     toPolicyView(snapshot.Policy),
			"orders":     toOrderViews(snapshot.Orders),
		})
	})

	mux.HandleFunc("GET /kds/orders", func(w http.ResponseWriter, _ *http.Request) {
		respond(w, logger, http.StatusOK,
			map[string]any{"orders": toOrderViews(kitchen.Snapshot().Orders)})
	})

	mux.HandleFunc("GET /kds/policy", func(w http.ResponseWriter, _ *http.Request) {
		respond(w, logger, http.StatusOK, toPolicyView(kitchen.Snapshot().Policy))
	})

	mux.HandleFunc("PUT /kds/policy", func(w http.ResponseWriter, r *http.Request) {
		var payload PolicyPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			respond(w, logger, http.StatusBadRequest,
				map[string]string{"error": "invalid JSON body"})
			return
		}

		updated, err := applyPolicyPatch(kitchen.Snapshot().Policy, payload)
		if err != nil {
			respond(w, logger, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		kitchen.SetPolicy(updated)
		logger.Info("fulfilment policy updated", slog.Bool("auto_accept", updated.AutoAccept))
		respond(w, logger, http.StatusOK, toPolicyView(updated))
	})

	mux.HandleFunc("POST /kds/orders/{orderId}/accept", func(w http.ResponseWriter, r *http.Request) {
		payload, err := decodeOptional[manualActionRequest](r)
		if err != nil {
			respond(w, logger, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if payload.PrepMinutes <= 0 {
			payload.PrepMinutes = kitchen.Snapshot().Policy.PrepMinutes
		}
		kitchen.Accept(r.Context(), r.PathValue("orderId"), payload.PrepMinutes)
		respond(w, logger, http.StatusAccepted, map[string]string{"status": "accept requested"})
	})

	mux.HandleFunc("POST /kds/orders/{orderId}/reject", func(w http.ResponseWriter, r *http.Request) {
		payload, err := decodeOptional[manualActionRequest](r)
		if err != nil {
			respond(w, logger, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if payload.Reason == "" {
			respond(w, logger, http.StatusBadRequest,
				map[string]string{"error": "reason is required"})
			return
		}
		kitchen.Reject(r.Context(), r.PathValue("orderId"), payload.Reason)
		respond(w, logger, http.StatusAccepted, map[string]string{"status": "reject requested"})
	})

	mux.HandleFunc("POST /kds/restock", func(w http.ResponseWriter, r *http.Request) {
		if err := kitchen.Restock(r.Context()); err != nil {
			respond(w, logger, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		respond(w, logger, http.StatusOK, map[string]string{"status": "restocked"})
	})

	return mux
}

func toPolicyView(policy Policy) policyView {
	return policyView{
		AutoAccept:      policy.AutoAccept,
		AutoRejectOver:  policy.AutoRejectOver,
		PrepMinutes:     policy.PrepMinutes,
		AcceptDelay:     policy.AcceptDelay.String(),
		CookingDelay:    policy.CookingDelay.String(),
		ReadyDelay:      policy.ReadyDelay.String(),
		DispatchDelay:   policy.DispatchDelay.String(),
		DeliveryDelay:   policy.DeliveryDelay.String(),
		RestockInterval: policy.RestockInterval.String(),
		RestockTo:       policy.RestockTo,
	}
}

func toOrderViews(orders []Order) []orderView {
	views := make([]orderView, 0, len(orders))
	for _, order := range orders {
		items := make([]itemView, 0, len(order.Items))
		for _, item := range order.Items {
			items = append(items, itemView(item))
		}
		views = append(views, orderView{
			ID:     order.ID,
			Number: order.Number,
			Status: order.Status,
			Total:  order.Total,
			Items:  items,
			Delivery: deliveryView{
				Recipient: order.Delivery.RecipientName,
				Phone:     order.Delivery.Phone,
				Address:   order.Delivery.Address,
				Comment:   order.Delivery.Comment,
			},
			ReceivedAt: order.ReceivedAt,
			UpdatedAt:  order.UpdatedAt,
			AutoPilot:  order.AutoPiloted,
		})
	}

	sort.Slice(views, func(i, j int) bool {
		return views[i].ReceivedAt.After(views[j].ReceivedAt)
	})
	return views
}

func applyPolicyPatch(policy Policy, payload PolicyPayload) (Policy, error) {
	if payload.AutoAccept != nil {
		policy.AutoAccept = *payload.AutoAccept
	}
	if payload.AutoRejectOver != nil {
		policy.AutoRejectOver = *payload.AutoRejectOver
	}
	if payload.PrepMinutes != nil {
		policy.PrepMinutes = *payload.PrepMinutes
	}
	if payload.RestockTo != nil {
		policy.RestockTo = *payload.RestockTo
	}

	durations := []struct {
		raw    *string
		target *time.Duration
	}{
		{payload.AcceptDelay, &policy.AcceptDelay},
		{payload.CookingDelay, &policy.CookingDelay},
		{payload.ReadyDelay, &policy.ReadyDelay},
		{payload.DispatchDelay, &policy.DispatchDelay},
		{payload.DeliveryDelay, &policy.DeliveryDelay},
		{payload.RestockInterval, &policy.RestockInterval},
	}
	for _, item := range durations {
		if item.raw == nil {
			continue
		}
		parsed, err := time.ParseDuration(*item.raw)
		if err != nil {
			return policy, err
		}
		*item.target = parsed
	}
	return policy, nil
}

func decodeOptional[T any](r *http.Request) (T, error) {
	var payload T
	if r.ContentLength == 0 {
		return payload, nil
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return payload, fmt.Errorf("invalid JSON body: %w", err)
	}
	return payload, nil
}

func respond(w http.ResponseWriter, logger *slog.Logger, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		logger.Error("write response", slog.Any("error", err))
	}
}
