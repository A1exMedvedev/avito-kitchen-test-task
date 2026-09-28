package eventbus_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/platform/eventbus"
	"github.com/google/uuid"
)

func newBus() eventbus.Bus {
	return eventbus.New(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func eventFor(venueID uuid.UUID) domain.OrderEvent {
	return domain.OrderEvent{
		ID: uuid.New(), Type: domain.EventOrderCreated,
		VenueID: venueID, OrderID: uuid.New(),
		Status: domain.OrderStatusCreated, OccurredAt: time.Now(),
	}
}

func TestEventsReachOnlyTheirOwnVenue(t *testing.T) {
	t.Parallel()

	bus := newBus()
	first, second := uuid.New(), uuid.New()

	firstEvents, cancelFirst := bus.Subscribe(first)
	defer cancelFirst()
	secondEvents, cancelSecond := bus.Subscribe(second)
	defer cancelSecond()

	bus.Publish(context.Background(), eventFor(first))

	select {
	case event := <-firstEvents:
		if event.VenueID != first {
			t.Fatalf("venue = %s, want %s", event.VenueID, first)
		}
	case <-time.After(time.Second):
		t.Fatal("the subscribed venue never received its event")
	}

	select {
	case event := <-secondEvents:
		t.Fatalf("an unrelated venue received an event: %+v", event)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSeveralSubscribersOfOneVenueAllGetTheEvent(t *testing.T) {
	t.Parallel()

	bus := newBus()
	venueID := uuid.New()

	firstEvents, cancelFirst := bus.Subscribe(venueID)
	defer cancelFirst()
	secondEvents, cancelSecond := bus.Subscribe(venueID)
	defer cancelSecond()

	if got := bus.SubscriberCount(venueID); got != 2 {
		t.Fatalf("subscriber count = %d, want 2", got)
	}
	bus.Publish(context.Background(), eventFor(venueID))

	for i, events := range []<-chan domain.OrderEvent{firstEvents, secondEvents} {
		select {
		case <-events:
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d never received the event", i)
		}
	}
}

func TestUnsubscribeReleasesTheSubscription(t *testing.T) {
	t.Parallel()

	bus := newBus()
	venueID := uuid.New()

	events, cancel := bus.Subscribe(venueID)
	cancel()

	if got := bus.SubscriberCount(venueID); got != 0 {
		t.Fatalf("subscriber count = %d, want 0 after cancel", got)
	}
	if _, open := <-events; open {
		t.Fatal("the channel should be closed so the stream handler can return")
	}

	cancel()

	bus.Publish(context.Background(), eventFor(venueID))
}

func TestASlowSubscriberDoesNotBlockThePublisher(t *testing.T) {
	t.Parallel()

	bus := newBus()
	venueID := uuid.New()

	_, cancel := bus.Subscribe(venueID)
	defer cancel()

	done := make(chan struct{})
	go func() {
		for range 500 {
			bus.Publish(context.Background(), eventFor(venueID))
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("publishing blocked on a subscriber that is not keeping up")
	}
}

func TestCloseEndsEveryStream(t *testing.T) {
	t.Parallel()

	bus := newBus()
	venueID := uuid.New()
	events, cancel := bus.Subscribe(venueID)
	defer cancel()

	bus.Close()

	select {
	case _, open := <-events:
		if open {
			t.Fatal("the channel should be closed after Close")
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not end the subscription")
	}

	late, cancelLate := bus.Subscribe(venueID)
	defer cancelLate()
	if _, open := <-late; open {
		t.Fatal("subscribing after Close should hand back a closed channel")
	}
}
