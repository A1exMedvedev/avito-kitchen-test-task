package domain

import (
	"time"

	"github.com/google/uuid"
)

type EventType string

const (
	EventOrderCreated       EventType = "order.created"
	EventOrderStatusChanged EventType = "order.status_changed"
)

type OrderEvent struct {
	ID             uuid.UUID
	Type           EventType
	VenueID        uuid.UUID
	OrderID        uuid.UUID
	PreviousStatus OrderStatus
	Status         OrderStatus
	OccurredAt     time.Time

	Order *Order
}

func NewOrderCreatedEvent(order *Order, now time.Time) OrderEvent {
	return OrderEvent{
		ID:         uuid.New(),
		Type:       EventOrderCreated,
		VenueID:    order.VenueID,
		OrderID:    order.ID,
		Status:     order.Status,
		OccurredAt: now,
		Order:      order,
	}
}

func NewOrderStatusChangedEvent(order *Order, change StatusChange) OrderEvent {
	return OrderEvent{
		ID:             uuid.New(),
		Type:           EventOrderStatusChanged,
		VenueID:        order.VenueID,
		OrderID:        order.ID,
		PreviousStatus: change.FromStatus,
		Status:         change.ToStatus,
		OccurredAt:     change.OccurredAt,
		Order:          order,
	}
}
