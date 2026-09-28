package domain

import (
	"crypto/rand"
	"math/big"
	"strconv"
	"time"

	"github.com/google/uuid"
)

type OrderStatus string

const (
	OrderStatusCreated    OrderStatus = "created"
	OrderStatusAccepted   OrderStatus = "accepted"
	OrderStatusCooking    OrderStatus = "cooking"
	OrderStatusReady      OrderStatus = "ready"
	OrderStatusInDelivery OrderStatus = "in_delivery"
	OrderStatusDelivered  OrderStatus = "delivered"
	OrderStatusRejected   OrderStatus = "rejected"
	OrderStatusCancelled  OrderStatus = "cancelled"
)

type Actor string

const (
	ActorCustomer Actor = "customer"
	ActorVenue    Actor = "venue"
	ActorSystem   Actor = "system"
)

var orderTransitions = map[OrderStatus]map[OrderStatus][]Actor{
	OrderStatusCreated: {
		OrderStatusAccepted:  {ActorVenue},
		OrderStatusRejected:  {ActorVenue, ActorSystem},
		OrderStatusCancelled: {ActorCustomer, ActorSystem},
	},
	OrderStatusAccepted: {
		OrderStatusCooking: {ActorVenue},

		OrderStatusRejected:  {ActorVenue},
		OrderStatusCancelled: {ActorCustomer, ActorSystem},
	},
	OrderStatusCooking: {
		OrderStatusReady: {ActorVenue},
	},
	OrderStatusReady: {
		OrderStatusInDelivery: {ActorVenue, ActorSystem},
	},
	OrderStatusInDelivery: {
		OrderStatusDelivered: {ActorVenue, ActorSystem},
	},
}

func (s OrderStatus) IsTerminal() bool {
	switch s {
	case OrderStatusDelivered, OrderStatusRejected, OrderStatusCancelled:
		return true
	default:
		return false
	}
}

func (s OrderStatus) ReleasesStock() bool {
	return s == OrderStatusRejected || s == OrderStatusCancelled
}

func (s OrderStatus) Valid() bool {
	switch s {
	case OrderStatusCreated, OrderStatusAccepted, OrderStatusCooking, OrderStatusReady,
		OrderStatusInDelivery, OrderStatusDelivered, OrderStatusRejected, OrderStatusCancelled:
		return true
	default:
		return false
	}
}

type OrderItem struct {
	ID         uuid.UUID
	OrderID    uuid.UUID
	MenuItemID uuid.UUID
	Name       string
	UnitPrice  Money
	Quantity   int
	LineTotal  Money
}

type Delivery struct {
	RecipientName string
	Phone         string
	Address       string
	Comment       string
}

func (d Delivery) Validate() error {
	switch {
	case d.RecipientName == "":
		return Invalid("delivery_name_required", "recipient name is required")
	case len([]rune(d.RecipientName)) > MaxRecipientNameLength:
		return Invalid("delivery_name_too_long",
			"recipient name must be at most %d characters", MaxRecipientNameLength)
	case d.Phone == "":
		return Invalid("delivery_phone_required", "phone is required")
	case len([]rune(d.Phone)) > MaxPhoneLength:
		return Invalid("delivery_phone_too_long",
			"phone must be at most %d characters", MaxPhoneLength)
	case len([]rune(d.Address)) < MinAddressLength:
		return Invalid("delivery_address_required", "delivery address is required")
	case len([]rune(d.Address)) > MaxAddressLength:
		return Invalid("delivery_address_too_long",
			"delivery address must be at most %d characters", MaxAddressLength)
	case len([]rune(d.Comment)) > MaxDeliveryCommentLength:
		return Invalid("delivery_comment_too_long",
			"comment must be at most %d characters", MaxDeliveryCommentLength)
	default:
		return nil
	}
}

type StatusChange struct {
	ID         uuid.UUID
	OrderID    uuid.UUID
	FromStatus OrderStatus
	ToStatus   OrderStatus
	Actor      Actor
	Reason     string
	OccurredAt time.Time
}

type Order struct {
	ID         uuid.UUID
	Number     string
	CustomerID uuid.UUID
	VenueID    uuid.UUID
	VenueName  string
	Status     OrderStatus

	Items       []OrderItem
	ItemsTotal  Money
	DeliveryFee Money
	Total       Money

	Delivery     Delivery
	StatusReason string
	PrepMinutes  int

	IdempotencyKey string

	Version int

	Timeline []StatusChange

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (o *Order) IsActive() bool { return !o.Status.IsTerminal() }

func (o *Order) CanTransitionTo(next OrderStatus, by Actor) error {
	if !next.Valid() {
		return Invalid("invalid_status", "unknown order status %q", next)
	}
	if o.Status == next {
		return Conflict("invalid_status_transition", "order is already %s", next)
	}
	if o.Status.IsTerminal() {
		return Conflict("order_finished", "order is already %s and cannot change", o.Status)
	}

	allowedActors, reachable := orderTransitions[o.Status][next]
	if !reachable {
		return Conflict("invalid_status_transition",
			"cannot move order from %s to %s", o.Status, next)
	}
	for _, actor := range allowedActors {
		if actor == by {
			return nil
		}
	}
	return Forbidden("transition_not_allowed",
		"%s is not allowed to move an order from %s to %s", by, o.Status, next)
}

func (o *Order) TransitionTo(next OrderStatus, by Actor, reason string, now time.Time) (StatusChange, error) {
	if err := o.CanTransitionTo(next, by); err != nil {
		return StatusChange{}, err
	}
	if next == OrderStatusRejected && reason == "" {
		return StatusChange{}, Invalid("reject_reason_required",
			"a rejection must carry a reason the customer can read")
	}
	if len([]rune(reason)) > MaxStatusReasonLength {
		return StatusChange{}, Invalid("status_reason_too_long",
			"reason must be at most %d characters", MaxStatusReasonLength)
	}

	change := StatusChange{
		ID:         uuid.New(),
		OrderID:    o.ID,
		FromStatus: o.Status,
		ToStatus:   next,
		Actor:      by,
		Reason:     reason,
		OccurredAt: now,
	}

	o.Status = next
	o.UpdatedAt = now
	o.Timeline = append(o.Timeline, change)
	if reason != "" {
		o.StatusReason = reason
	}
	return change, nil
}

func NewOrderFromCart(priced PricedCart, delivery Delivery, now time.Time) (*Order, error) {
	if err := delivery.Validate(); err != nil {
		return nil, err
	}
	if priced.Venue == nil {
		return nil, NotFound("venue_not_found", "venue is unknown")
	}
	if err := priced.Venue.EnsureAcceptsOrders(); err != nil {
		return nil, err
	}
	if len(priced.Lines) == 0 {
		return nil, Unprocessable("cart_empty", "cannot place an order with an empty cart")
	}

	if unavailable := priced.UnavailableItems(); len(unavailable) > 0 {
		return nil, &Error{
			Kind:    KindConflict,
			Code:    "items_unavailable",
			Message: unavailableMessage(unavailable, len(priced.Lines)),
			Items:   unavailable,
		}
	}
	if priced.ItemsTotal < priced.Venue.MinOrderAmount {
		return nil, Unprocessable("below_min_order",
			"order total is below the venue's minimum of %d.%02d %s",
			priced.Venue.MinOrderAmount/100, priced.Venue.MinOrderAmount%100, Currency)
	}

	orderID := uuid.New()
	items := make([]OrderItem, 0, len(priced.Lines))
	for _, line := range priced.Lines {
		items = append(items, OrderItem{
			ID:         uuid.New(),
			OrderID:    orderID,
			MenuItemID: line.MenuItemID,
			Name:       line.Name,
			UnitPrice:  line.UnitPrice,
			Quantity:   line.Quantity,
			LineTotal:  line.LineTotal,
		})
	}

	order := &Order{
		ID:          orderID,
		Number:      NewOrderNumber(),
		CustomerID:  priced.Cart.CustomerID,
		VenueID:     priced.Venue.ID,
		VenueName:   priced.Venue.Name,
		Status:      OrderStatusCreated,
		Items:       items,
		ItemsTotal:  priced.ItemsTotal,
		DeliveryFee: priced.DeliveryFee,
		Total:       priced.Total,
		Delivery:    delivery,
		PrepMinutes: priced.Venue.AvgPrepMinutes,
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	order.Timeline = []StatusChange{{
		ID:         uuid.New(),
		OrderID:    orderID,
		ToStatus:   OrderStatusCreated,
		Actor:      ActorCustomer,
		OccurredAt: now,
	}}
	return order, nil
}

func unavailableMessage(unavailable []UnavailableItem, total int) string {
	if len(unavailable) == 1 {
		return "\"" + unavailable[0].Name + "\" is no longer available"
	}
	return strconv.Itoa(len(unavailable)) + " of " + strconv.Itoa(total) +
		" positions are no longer available"
}

const orderNumberAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func NewOrderNumber() string {
	suffix := make([]byte, 6)
	for i := range suffix {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(orderNumberAlphabet))))
		if err != nil {

			suffix[i] = orderNumberAlphabet[i%len(orderNumberAlphabet)]
			continue
		}
		suffix[i] = orderNumberAlphabet[n.Int64()]
	}
	return "AK-" + string(suffix)
}
