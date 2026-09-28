package service

import (
	"context"
	"errors"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"github.com/google/uuid"
)

const checkoutRetries = 3

type orderService struct {
	tx           TxManager
	orders       OrderRepository
	carts        CartRepository
	menus        MenuRepository
	venues       VenueRepository
	customers    CustomerRepository
	outbox       OutboxRepository
	notifier     OutboxNotifier
	clock        Clock
	transitioner *transitioner
}

func NewOrderService(
	tx TxManager,
	orders OrderRepository,
	carts CartRepository,
	menus MenuRepository,
	venues VenueRepository,
	customers CustomerRepository,
	outbox OutboxRepository,
	notifier OutboxNotifier,
	clock Clock,
) OrderService {
	return &orderService{
		tx:           tx,
		orders:       orders,
		carts:        carts,
		menus:        menus,
		venues:       venues,
		customers:    customers,
		outbox:       outbox,
		notifier:     notifier,
		clock:        clock,
		transitioner: newTransitioner(tx, orders, menus, outbox, notifier, clock),
	}
}

func (s *orderService) Checkout(ctx context.Context, cmd CheckoutCommand) (*domain.Order, error) {
	if cmd.IdempotencyKey == "" {
		return nil, domain.Invalid("idempotency_key_required",
			"Idempotency-Key header is required for checkout")
	}
	if err := cmd.Delivery.Validate(); err != nil {
		return nil, err
	}

	var lastErr error
	for attempt := 0; attempt < checkoutRetries; attempt++ {
		order, err := s.checkoutOnce(ctx, cmd)
		if err == nil {
			s.notifier.Notify()
			return order, nil
		}
		lastErr = err

		if domainErr, ok := errors.AsType[*domain.Error](err); !ok || domainErr.Code != "order_number_conflict" {
			return nil, err
		}
	}
	return nil, lastErr
}

func (s *orderService) checkoutOnce(ctx context.Context, cmd CheckoutCommand) (*domain.Order, error) {
	var placed *domain.Order

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := s.customers.Ensure(ctx, cmd.CustomerID); err != nil {
			return err
		}

		existing, err := s.orders.GetByIdempotencyKey(ctx, cmd.CustomerID, cmd.IdempotencyKey)
		if err != nil {
			return err
		}
		if existing != nil {
			placed = existing
			return nil
		}

		cart, err := s.carts.GetActive(ctx, cmd.CustomerID)
		if err != nil {
			return err
		}
		if cart == nil || len(cart.Lines) == 0 {
			return domain.Unprocessable("cart_empty", "cannot place an order with an empty cart")
		}

		venue, err := s.venues.Get(ctx, cart.VenueID)
		if err != nil {
			return err
		}

		ids := make([]uuid.UUID, 0, len(cart.Lines))
		for _, line := range cart.Lines {
			ids = append(ids, line.MenuItemID)
		}
		items, err := s.menus.LockItemsByIDs(ctx, ids)
		if err != nil {
			return err
		}
		for id, item := range items {
			if item.VenueID != venue.ID {
				delete(items, id)
			}
		}

		priced := domain.PriceCart(*cart, venue, items)
		order, err := domain.NewOrderFromCart(priced, cmd.Delivery, s.clock())
		if err != nil {
			return err
		}
		order.IdempotencyKey = cmd.IdempotencyKey

		adjustments := make([]StockAdjustment, 0, len(order.Items))
		for _, item := range order.Items {
			adjustments = append(adjustments, StockAdjustment{
				MenuItemID: item.MenuItemID,
				Delta:      -item.Quantity,
			})
		}
		if err := s.menus.AdjustStock(ctx, adjustments); err != nil {
			return err
		}
		if err := s.orders.Create(ctx, order); err != nil {
			return err
		}
		if err := s.carts.MarkOrdered(ctx, cart.ID); err != nil {
			return err
		}
		if err := s.customers.UpdateContacts(
			ctx, cmd.CustomerID, cmd.Delivery.RecipientName, cmd.Delivery.Phone,
		); err != nil {
			return err
		}
		if err := s.outbox.Append(ctx, domain.NewOrderCreatedEvent(order, order.CreatedAt)); err != nil {
			return err
		}

		placed = order
		return nil
	})
	if err != nil {
		return nil, err
	}
	return placed, nil
}

func (s *orderService) List(
	ctx context.Context,
	customerID uuid.UUID,
	statuses []domain.OrderStatus,
	page Page,
) ([]domain.Order, int, error) {
	return s.orders.List(ctx, OrderFilter{
		CustomerID: &customerID,
		Statuses:   statuses,
		Page:       page.Normalize(),
	})
}

func (s *orderService) Get(ctx context.Context, customerID, orderID uuid.UUID) (*domain.Order, error) {
	order, err := s.orders.Get(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order.CustomerID != customerID {
		return nil, domain.NotFound("order_not_found", "order not found")
	}
	return order, nil
}

func (s *orderService) Cancel(ctx context.Context, customerID, orderID uuid.UUID, reason string) (*domain.Order, error) {
	return s.transitioner.apply(ctx, transitionCommand{
		OrderID:   orderID,
		Target:    domain.OrderStatusCancelled,
		Actor:     domain.ActorCustomer,
		Reason:    reason,
		Authorize: ownedByCustomer(customerID),
	})
}
