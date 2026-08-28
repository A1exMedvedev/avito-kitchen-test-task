package postgres

import (
	"encoding/json"
	"time"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/repository/postgres/models"
	"github.com/google/uuid"
)

func venueToDomain(row models.Venue) domain.Venue {
	return domain.Venue{
		ID:             row.ID,
		Slug:           row.Slug,
		Name:           row.Name,
		Description:    row.Description,
		Cuisines:       row.Cuisines,
		City:           row.City,
		Address:        row.Address,
		Status:         domain.VenueStatus(row.Status),
		IsOpen:         row.IsOpen,
		AvgPrepMinutes: int(row.AvgPrepMinutes),
		MinOrderAmount: domain.Money(row.MinOrderAmount),
		DeliveryFee:    domain.Money(row.DeliveryFee),
		Rating:         row.Rating,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
}

func venuesToDomain(rows []models.Venue) []domain.Venue {
	venues := make([]domain.Venue, 0, len(rows))
	for _, row := range rows {
		venues = append(venues, venueToDomain(row))
	}
	return venues
}

func categoryToDomain(row models.MenuCategory) domain.MenuCategory {
	return domain.MenuCategory{
		ID:        row.ID,
		VenueID:   row.VenueID,
		Name:      row.Name,
		Position:  int(row.Position),
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

func categoriesToDomain(rows []models.MenuCategory) []domain.MenuCategory {
	categories := make([]domain.MenuCategory, 0, len(rows))
	for _, row := range rows {
		categories = append(categories, categoryToDomain(row))
	}
	return categories
}

func menuItemToDomain(row models.MenuItem) domain.MenuItem {
	return domain.MenuItem{
		ID:            row.ID,
		VenueID:       row.VenueID,
		CategoryID:    row.CategoryID,
		Name:          row.Name,
		Description:   row.Description,
		Price:         domain.Money(row.Price),
		IsAvailable:   row.IsAvailable,
		StockQuantity: int(row.StockQuantity),
		WeightGrams:   int(row.WeightGrams),
		Position:      int(row.Position),
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
		DeletedAt:     row.DeletedAt,
	}
}

func menuItemsToDomain(rows []models.MenuItem) []domain.MenuItem {
	items := make([]domain.MenuItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, menuItemToDomain(row))
	}
	return items
}

func customerToDomain(row models.Customer) domain.Customer {
	return domain.Customer{
		ID:          row.ID,
		DisplayName: row.DisplayName,
		Phone:       row.Phone,
		CreatedAt:   row.CreatedAt,
	}
}

func cartToDomain(row models.Cart, items []models.CartItem) domain.Cart {
	lines := make([]domain.CartLine, 0, len(items))
	for _, item := range items {
		lines = append(lines, domain.CartLine{
			MenuItemID: item.MenuItemID,
			Quantity:   int(item.Quantity),
		})
	}
	return domain.Cart{
		ID:         row.ID,
		CustomerID: row.CustomerID,
		VenueID:    row.VenueID,
		Status:     domain.CartStatus(row.Status),
		Lines:      lines,
		CreatedAt:  row.CreatedAt,
		UpdatedAt:  row.UpdatedAt,
	}
}

func orderToDomain(row models.Order, items []models.OrderItem, history []models.OrderStatusHistory) domain.Order {
	order := domain.Order{
		ID:         row.ID,
		Number:     row.Number,
		CustomerID: row.CustomerID,
		VenueID:    row.VenueID,
		VenueName:  row.VenueName,
		Status:     domain.OrderStatus(row.Status),
		Items:      orderItemsToDomain(items),

		ItemsTotal:  domain.Money(row.ItemsTotal),
		DeliveryFee: domain.Money(row.DeliveryFee),
		Total:       domain.Money(row.Total),

		Delivery: domain.Delivery{
			RecipientName: row.RecipientName,
			Phone:         row.Phone,
			Address:       row.Address,
			Comment:       row.Comment,
		},
		StatusReason:   row.StatusReason,
		PrepMinutes:    int(row.PrepMinutes),
		IdempotencyKey: row.IdempotencyKey,
		Version:        int(row.Version),
		Timeline:       statusHistoryToDomain(history),

		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
	return order
}

func orderItemsToDomain(rows []models.OrderItem) []domain.OrderItem {
	items := make([]domain.OrderItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, domain.OrderItem{
			ID:         row.ID,
			OrderID:    row.OrderID,
			MenuItemID: row.MenuItemID,
			Name:       row.Name,
			UnitPrice:  domain.Money(row.UnitPrice),
			Quantity:   int(row.Quantity),
			LineTotal:  domain.Money(row.LineTotal),
		})
	}
	return items
}

func statusHistoryToDomain(rows []models.OrderStatusHistory) []domain.StatusChange {
	changes := make([]domain.StatusChange, 0, len(rows))
	for _, row := range rows {
		change := domain.StatusChange{
			ID:         row.ID,
			OrderID:    row.OrderID,
			ToStatus:   domain.OrderStatus(row.ToStatus),
			Actor:      domain.Actor(row.Actor),
			Reason:     row.Reason,
			OccurredAt: row.OccurredAt,
		}
		if row.FromStatus != nil {
			change.FromStatus = domain.OrderStatus(*row.FromStatus)
		}
		changes = append(changes, change)
	}
	return changes
}

type outboxPayload struct {
	PreviousStatus string `json:"previous_status,omitempty"`
	Status         string `json:"status"`
}

func outboxRowFromDomain(event domain.OrderEvent) (models.OutboxEvent, error) {
	payload, err := json.Marshal(outboxPayload{
		PreviousStatus: string(event.PreviousStatus),
		Status:         string(event.Status),
	})
	if err != nil {
		return models.OutboxEvent{}, err
	}
	return models.OutboxEvent{
		ID:            eventID(event),
		EventType:     string(event.Type),
		AggregateType: "order",
		AggregateID:   event.OrderID,
		VenueID:       event.VenueID,
		Payload:       payload,
		OccurredAt:    eventTime(event),
	}, nil
}

func outboxToDomain(row models.OutboxEvent) (domain.OrderEvent, error) {
	var payload outboxPayload
	err := json.Unmarshal(row.Payload, &payload)

	return domain.OrderEvent{
		ID:             row.ID,
		Type:           domain.EventType(row.EventType),
		VenueID:        row.VenueID,
		OrderID:        row.AggregateID,
		PreviousStatus: domain.OrderStatus(payload.PreviousStatus),
		Status:         domain.OrderStatus(payload.Status),
		OccurredAt:     row.OccurredAt,
	}, err
}

func eventID(event domain.OrderEvent) uuid.UUID {
	if event.ID == uuid.Nil {
		return uuid.New()
	}
	return event.ID
}

func eventTime(event domain.OrderEvent) time.Time {
	if event.OccurredAt.IsZero() {
		return time.Now().UTC()
	}
	return event.OccurredAt
}
