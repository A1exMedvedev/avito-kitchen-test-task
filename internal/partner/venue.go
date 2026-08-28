package partner

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"time"

	kitchenv1 "backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/api/gen/kitchen/v1"
)

type Venue interface {
	WaitReady(ctx context.Context, timeout time.Duration) error
	Start(ctx context.Context, syncMenu bool) error
	ConsumeEvents(ctx context.Context) error
	RunRestocking(ctx context.Context)
	Restock(ctx context.Context) error
	Accept(ctx context.Context, orderID string, prepMinutes int)
	Reject(ctx context.Context, orderID, reason string)
	SetPolicy(policy Policy)
	Snapshot() Snapshot
	Close(ctx context.Context)
	Shutdown() error
}

type Snapshot struct {
	VenueID   string
	VenueName string
	Policy    Policy
	Orders    []Order
}

type Order struct {
	ID          string
	Number      string
	Status      string
	Total       int64
	Items       []OrderItem
	Delivery    Delivery
	ReceivedAt  time.Time
	UpdatedAt   time.Time
	AutoPiloted bool
}

type OrderItem struct {
	Name      string
	Quantity  int32
	UnitPrice int64
}

type Delivery struct {
	RecipientName string
	Phone         string
	Address       string
	Comment       string
}

type venue struct {
	client *client
	logger *slog.Logger

	mu       sync.RWMutex
	settings Policy
	orders   map[string]*Order
	piloted  map[string]bool

	venueName string
	venueID   string
}

func New(cfg Config, logger *slog.Logger) (Venue, error) {
	connection, err := dial(cfg.KitchenGRPCAddr, cfg.APIKey)
	if err != nil {
		return nil, err
	}
	return &venue{
		client:   connection,
		logger:   logger,
		settings: cfg.Policy,
		orders:   make(map[string]*Order),
		piloted:  make(map[string]bool),
	}, nil
}

func (v *venue) WaitReady(ctx context.Context, timeout time.Duration) error {
	return v.client.waitReady(ctx, timeout)
}

func (v *venue) Shutdown() error { return v.client.close() }

func (v *venue) policy() Policy {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.settings
}

func (v *venue) SetPolicy(policy Policy) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.settings = policy
}

func (v *venue) Start(ctx context.Context, syncMenu bool) error {
	profile, err := v.client.partner.GetVenue(ctx, &kitchenv1.GetVenueRequest{})
	if err != nil {
		return err
	}
	v.mu.Lock()
	v.venueName, v.venueID = profile.GetName(), profile.GetId()
	v.mu.Unlock()

	v.logger.Info("authenticated with the platform",
		slog.String("venue", profile.GetName()),
		slog.String("venue_id", profile.GetId()))

	if syncMenu {
		if err := v.syncMenu(ctx); err != nil {
			return err
		}
	}

	if _, err := v.client.partner.SetVenueOpen(ctx, &kitchenv1.SetVenueOpenRequest{IsOpen: true}); err != nil {
		return err
	}
	v.logger.Info("venue is open and accepting orders")
	return nil
}

func (v *venue) Close(ctx context.Context) {
	if _, err := v.client.partner.SetVenueOpen(ctx, &kitchenv1.SetVenueOpenRequest{IsOpen: false}); err != nil {
		v.logger.Warn("could not close the venue on shutdown", slog.Any("error", err))
		return
	}
	v.logger.Info("venue closed")
}

func (v *venue) ConsumeEvents(ctx context.Context) error {
	attempt := 0

	for {
		if ctx.Err() != nil {
			return nil //nolint:nilerr
		}
		attempt++

		err := v.consumeOnce(ctx)
		switch {
		case err == nil, errors.Is(err, io.EOF):
			v.logger.Info("event stream ended, reconnecting")
			attempt = 0
		case ctx.Err() != nil:
			return nil //nolint:nilerr
		default:
			v.logger.Warn("event stream failed, reconnecting",
				slog.Any("error", err),
				slog.Duration("in", backoff(attempt)))
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff(attempt)):
		}
	}
}

func (v *venue) consumeOnce(ctx context.Context) error {
	stream, err := v.client.partner.SubscribeOrderEvents(ctx,
		&kitchenv1.SubscribeOrderEventsRequest{ReplayActiveOrders: true})
	if err != nil {
		return err
	}
	v.logger.Info("subscribed to order events")

	for {
		event, err := stream.Recv()
		if err != nil {
			return err
		}
		v.handleEvent(ctx, event)
	}
}

func (v *venue) handleEvent(ctx context.Context, event *kitchenv1.OrderEvent) {
	received := event.GetOrder()
	if received == nil {
		return
	}

	tracked := v.track(received)
	v.logger.Info("order event",
		slog.String("order", received.GetNumber()),
		slog.String("status", received.GetStatus().String()),
		slog.String("event", event.GetType().String()))

	switch received.GetStatus() {
	case kitchenv1.OrderStatus_ORDER_STATUS_CREATED:
		v.decideOnNewOrder(ctx, tracked)
	case kitchenv1.OrderStatus_ORDER_STATUS_ACCEPTED:
		v.startFulfilment(ctx, tracked)
	case kitchenv1.OrderStatus_ORDER_STATUS_DELIVERED,
		kitchenv1.OrderStatus_ORDER_STATUS_REJECTED,
		kitchenv1.OrderStatus_ORDER_STATUS_CANCELLED:
		v.forget(received.GetId())
	default:
	}
}

func (v *venue) track(received *kitchenv1.Order) *Order {
	items := make([]OrderItem, 0, len(received.GetItems()))
	for _, item := range received.GetItems() {
		items = append(items, OrderItem{
			Name:      item.GetName(),
			Quantity:  item.GetQuantity(),
			UnitPrice: item.GetUnitPrice().GetMinorUnits(),
		})
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	tracked, known := v.orders[received.GetId()]
	if !known {
		tracked = &Order{ID: received.GetId(), ReceivedAt: time.Now().UTC()}
		v.orders[received.GetId()] = tracked
	}
	tracked.Number = received.GetNumber()
	tracked.Status = received.GetStatus().String()
	tracked.Total = received.GetTotal().GetMinorUnits()
	tracked.Items = items
	tracked.Delivery = Delivery{
		RecipientName: received.GetDelivery().GetRecipientName(),
		Phone:         received.GetDelivery().GetPhone(),
		Address:       received.GetDelivery().GetAddress(),
		Comment:       received.GetDelivery().GetComment(),
	}
	tracked.UpdatedAt = time.Now().UTC()
	return tracked
}

func (v *venue) forget(orderID string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.piloted, orderID)
}

func (v *venue) decideOnNewOrder(ctx context.Context, order *Order) {
	policy := v.policy()
	if !policy.AutoAccept {
		v.logger.Info("order waiting for a manual decision",
			slog.String("order", order.Number))
		return
	}

	if policy.AutoRejectOver > 0 && order.Total > policy.AutoRejectOver {
		go v.after(ctx, policy.AcceptDelay, func(ctx context.Context) {
			v.Reject(ctx, order.ID, "заказ слишком большой для текущей загрузки кухни")
		})
		return
	}

	v.mu.Lock()
	order.AutoPiloted = true
	v.mu.Unlock()

	go v.after(ctx, policy.AcceptDelay, func(ctx context.Context) {
		v.Accept(ctx, order.ID, policy.PrepMinutes)
	})
}

func (v *venue) startFulfilment(ctx context.Context, order *Order) {
	v.mu.Lock()
	if v.piloted[order.ID] {
		v.mu.Unlock()
		return
	}
	v.piloted[order.ID] = true
	v.mu.Unlock()

	policy := v.policy()
	steps := []struct {
		delay  time.Duration
		status kitchenv1.OrderStatus
	}{
		{policy.CookingDelay, kitchenv1.OrderStatus_ORDER_STATUS_COOKING},
		{policy.ReadyDelay, kitchenv1.OrderStatus_ORDER_STATUS_READY},
		{policy.DispatchDelay, kitchenv1.OrderStatus_ORDER_STATUS_IN_DELIVERY},
		{policy.DeliveryDelay, kitchenv1.OrderStatus_ORDER_STATUS_DELIVERED},
	}

	go func() {
		for _, step := range steps {
			select {
			case <-ctx.Done():
				return
			case <-time.After(step.delay):
			}

			_, err := v.client.partner.UpdateOrderStatus(ctx, &kitchenv1.UpdateOrderStatusRequest{
				OrderId: order.ID,
				Status:  step.status,
			})
			if err != nil {
				v.logger.Info("stopping fulfilment",
					slog.String("order", order.Number),
					slog.String("at", step.status.String()),
					slog.Any("reason", err))
				return
			}
			v.logger.Info("order advanced",
				slog.String("order", order.Number),
				slog.String("status", step.status.String()))
		}
	}()
}

func (v *venue) Accept(ctx context.Context, orderID string, prepMinutes int) {
	_, err := v.client.partner.AcceptOrder(ctx, &kitchenv1.AcceptOrderRequest{
		OrderId:     orderID,
		PrepMinutes: int32(prepMinutes), //nolint:gosec
	})
	if err != nil {
		v.logger.Warn("accept failed", slog.String("order_id", orderID), slog.Any("error", err))
		return
	}
	v.logger.Info("order accepted", slog.String("order_id", orderID))
}

func (v *venue) Reject(ctx context.Context, orderID, reason string) {
	_, err := v.client.partner.RejectOrder(ctx, &kitchenv1.RejectOrderRequest{
		OrderId: orderID,
		Reason:  reason,
	})
	if err != nil {
		v.logger.Warn("reject failed", slog.String("order_id", orderID), slog.Any("error", err))
		return
	}
	v.logger.Info("order rejected", slog.String("order_id", orderID), slog.String("reason", reason))
}

func (v *venue) RunRestocking(ctx context.Context) {
	interval := v.policy().RestockInterval
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := v.Restock(ctx); err != nil && ctx.Err() == nil {
				v.logger.Warn("restock failed", slog.Any("error", err))
			}
		}
	}
}

func (v *venue) after(ctx context.Context, delay time.Duration, fn func(context.Context)) {
	select {
	case <-ctx.Done():
	case <-time.After(delay):
		fn(ctx)
	}
}

func (v *venue) Snapshot() Snapshot {
	v.mu.RLock()
	defer v.mu.RUnlock()

	orders := make([]Order, 0, len(v.orders))
	for _, order := range v.orders {
		orders = append(orders, *order)
	}
	return Snapshot{
		VenueID:   v.venueID,
		VenueName: v.venueName,
		Policy:    v.settings,
		Orders:    orders,
	}
}
