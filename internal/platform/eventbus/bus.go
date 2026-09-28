package eventbus

import (
	"context"
	"log/slog"
	"sync"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"github.com/google/uuid"
)

const defaultBuffer = 64

type Bus interface {
	Publish(ctx context.Context, events ...domain.OrderEvent)
	Subscribe(venueID uuid.UUID) (<-chan domain.OrderEvent, func())
	SubscriberCount(venueID uuid.UUID) int
	Close()
}

type subscription struct {
	id      uint64
	venueID uuid.UUID
	channel chan domain.OrderEvent
}

type bus struct {
	logger *slog.Logger
	buffer int

	mu       sync.RWMutex
	nextID   uint64
	byVenue  map[uuid.UUID]map[uint64]*subscription
	shutdown bool
}

func New(logger *slog.Logger) Bus {
	return &bus{
		logger:  logger,
		buffer:  defaultBuffer,
		byVenue: make(map[uuid.UUID]map[uint64]*subscription),
	}
}

func (b *bus) Subscribe(venueID uuid.UUID) (<-chan domain.OrderEvent, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.nextID++
	sub := &subscription{
		id:      b.nextID,
		venueID: venueID,
		channel: make(chan domain.OrderEvent, b.buffer),
	}
	if b.shutdown {
		close(sub.channel)
		return sub.channel, func() {}
	}

	if b.byVenue[venueID] == nil {
		b.byVenue[venueID] = make(map[uint64]*subscription)
	}
	b.byVenue[venueID][sub.id] = sub

	var once sync.Once
	return sub.channel, func() {
		once.Do(func() { b.unsubscribe(sub) })
	}
}

func (b *bus) unsubscribe(sub *subscription) {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs, ok := b.byVenue[sub.venueID]
	if !ok {
		return
	}
	if _, present := subs[sub.id]; !present {
		return
	}
	delete(subs, sub.id)
	if len(subs) == 0 {
		delete(b.byVenue, sub.venueID)
	}
	close(sub.channel)
}

func (b *bus) Publish(_ context.Context, events ...domain.OrderEvent) {
	if len(events) == 0 {
		return
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, event := range events {
		for _, sub := range b.byVenue[event.VenueID] {
			select {
			case sub.channel <- event:
			default:
				b.logger.Warn("dropping event for a subscriber that is not keeping up",
					slog.String("venue_id", event.VenueID.String()),
					slog.String("order_id", event.OrderID.String()),
					slog.String("event_type", string(event.Type)))
			}
		}
	}
}

func (b *bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.shutdown = true
	for venueID, subs := range b.byVenue {
		for _, sub := range subs {
			close(sub.channel)
		}
		delete(b.byVenue, venueID)
	}
}

func (b *bus) SubscriberCount(venueID uuid.UUID) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.byVenue[venueID])
}
