package service

import (
	"context"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"github.com/google/uuid"
)

type transitionCommand struct {
	OrderID     uuid.UUID
	Target      domain.OrderStatus
	Actor       domain.Actor
	Reason      string
	PrepMinutes int
	Authorize   func(order *domain.Order) error
}

type transitioner struct {
	tx       TxManager
	orders   OrderRepository
	menus    MenuRepository
	outbox   OutboxRepository
	notifier OutboxNotifier
	clock    Clock
}

func newTransitioner(
	tx TxManager,
	orders OrderRepository,
	menus MenuRepository,
	outbox OutboxRepository,
	notifier OutboxNotifier,
	clock Clock,
) *transitioner {
	return &transitioner{
		tx:       tx,
		orders:   orders,
		menus:    menus,
		outbox:   outbox,
		notifier: notifier,
		clock:    clock,
	}
}

func (t *transitioner) apply(ctx context.Context, cmd transitionCommand) (*domain.Order, error) {
	var result *domain.Order

	err := t.tx.WithinTx(ctx, func(ctx context.Context) error {
		order, err := t.orders.GetForUpdate(ctx, cmd.OrderID)
		if err != nil {
			return err
		}
		if cmd.Authorize != nil {
			if err := cmd.Authorize(order); err != nil {
				return err
			}
		}

		change, err := order.TransitionTo(cmd.Target, cmd.Actor, cmd.Reason, t.clock())
		if err != nil {
			return err
		}
		if cmd.PrepMinutes > 0 {
			order.PrepMinutes = cmd.PrepMinutes
		}

		if cmd.Target.ReleasesStock() {
			adjustments := make([]StockAdjustment, 0, len(order.Items))
			for _, item := range order.Items {
				adjustments = append(adjustments, StockAdjustment{
					MenuItemID: item.MenuItemID,
					Delta:      item.Quantity,
				})
			}
			if err := t.menus.AdjustStock(ctx, adjustments); err != nil {
				return err
			}
		}

		if err := t.orders.SaveStatus(ctx, order, change); err != nil {
			return err
		}
		if err := t.outbox.Append(ctx, domain.NewOrderStatusChangedEvent(order, change)); err != nil {
			return err
		}

		result = order
		return nil
	})
	if err != nil {
		return nil, err
	}

	t.notifier.Notify()
	return result, nil
}

func ownedByVenue(venueID uuid.UUID) func(*domain.Order) error {
	return func(order *domain.Order) error {
		if order.VenueID != venueID {
			return domain.NotFound("order_not_found", "order not found")
		}
		return nil
	}
}

func ownedByCustomer(customerID uuid.UUID) func(*domain.Order) error {
	return func(order *domain.Order) error {
		if order.CustomerID != customerID {
			return domain.NotFound("order_not_found", "order not found")
		}
		return nil
	}
}
