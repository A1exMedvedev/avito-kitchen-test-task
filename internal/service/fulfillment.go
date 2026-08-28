package service

import (
	"context"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"github.com/google/uuid"
)

type fulfillmentService struct {
	orders       OrderRepository
	transitioner *transitioner
}

func NewFulfillmentService(
	tx TxManager,
	orders OrderRepository,
	menus MenuRepository,
	outbox OutboxRepository,
	notifier OutboxNotifier,
	clock Clock,
) FulfillmentService {
	return &fulfillmentService{
		orders:       orders,
		transitioner: newTransitioner(tx, orders, menus, outbox, notifier, clock),
	}
}

func (s *fulfillmentService) List(
	ctx context.Context,
	venueID uuid.UUID,
	statuses []domain.OrderStatus,
	page Page,
) ([]domain.Order, int, error) {
	if err := validateStatusFilter(statuses); err != nil {
		return nil, 0, err
	}
	return s.orders.List(ctx, OrderFilter{
		VenueID:  &venueID,
		Statuses: statuses,
		Page:     page.Normalize(),
	})
}

func (s *fulfillmentService) ListActive(ctx context.Context, venueID uuid.UUID) ([]domain.Order, error) {
	orders, _, err := s.orders.List(ctx, OrderFilter{
		VenueID: &venueID,
		Statuses: []domain.OrderStatus{
			domain.OrderStatusCreated,
			domain.OrderStatusAccepted,
			domain.OrderStatusCooking,
			domain.OrderStatusReady,
			domain.OrderStatusInDelivery,
		},
		Page: Page{Limit: maxPageLimit},
	})
	return orders, err
}

func (s *fulfillmentService) Get(ctx context.Context, venueID, orderID uuid.UUID) (*domain.Order, error) {
	order, err := s.orders.Get(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order.VenueID != venueID {
		return nil, domain.NotFound("order_not_found", "order not found")
	}
	return order, nil
}

func (s *fulfillmentService) Accept(ctx context.Context, venueID, orderID uuid.UUID, prepMinutes int) (*domain.Order, error) {
	return s.transitioner.apply(ctx, transitionCommand{
		OrderID:     orderID,
		Target:      domain.OrderStatusAccepted,
		Actor:       domain.ActorVenue,
		PrepMinutes: prepMinutes,
		Authorize:   ownedByVenue(venueID),
	})
}

func (s *fulfillmentService) Reject(ctx context.Context, venueID, orderID uuid.UUID, reason string) (*domain.Order, error) {
	return s.transitioner.apply(ctx, transitionCommand{
		OrderID:   orderID,
		Target:    domain.OrderStatusRejected,
		Actor:     domain.ActorVenue,
		Reason:    reason,
		Authorize: ownedByVenue(venueID),
	})
}

func (s *fulfillmentService) UpdateStatus(
	ctx context.Context,
	venueID, orderID uuid.UUID,
	target domain.OrderStatus,
	reason string,
) (*domain.Order, error) {
	return s.transitioner.apply(ctx, transitionCommand{
		OrderID:   orderID,
		Target:    target,
		Actor:     domain.ActorVenue,
		Reason:    reason,
		Authorize: ownedByVenue(venueID),
	})
}
