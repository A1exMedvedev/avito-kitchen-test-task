package models

import (
	"time"

	"github.com/google/uuid"
)

type Venue struct {
	ID             uuid.UUID `db:"id"`
	Slug           string    `db:"slug"`
	Name           string    `db:"name"`
	Description    string    `db:"description"`
	Cuisines       []string  `db:"cuisines"`
	City           string    `db:"city"`
	Address        string    `db:"address"`
	Status         string    `db:"status"`
	IsOpen         bool      `db:"is_open"`
	AvgPrepMinutes int32     `db:"avg_prep_minutes"`
	MinOrderAmount int64     `db:"min_order_amount"`
	DeliveryFee    int64     `db:"delivery_fee"`
	Rating         float64   `db:"rating"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
}

type MenuCategory struct {
	ID        uuid.UUID `db:"id"`
	VenueID   uuid.UUID `db:"venue_id"`
	Name      string    `db:"name"`
	Position  int32     `db:"position"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

type MenuItem struct {
	ID            uuid.UUID  `db:"id"`
	VenueID       uuid.UUID  `db:"venue_id"`
	CategoryID    uuid.UUID  `db:"category_id"`
	Name          string     `db:"name"`
	Description   string     `db:"description"`
	Price         int64      `db:"price"`
	IsAvailable   bool       `db:"is_available"`
	StockQuantity int32      `db:"stock_quantity"`
	WeightGrams   int32      `db:"weight_grams"`
	Position      int32      `db:"position"`
	CreatedAt     time.Time  `db:"created_at"`
	UpdatedAt     time.Time  `db:"updated_at"`
	DeletedAt     *time.Time `db:"deleted_at"`
}

type Customer struct {
	ID          uuid.UUID `db:"id"`
	DisplayName string    `db:"display_name"`
	Phone       string    `db:"phone"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

type Cart struct {
	ID         uuid.UUID `db:"id"`
	CustomerID uuid.UUID `db:"customer_id"`
	VenueID    uuid.UUID `db:"venue_id"`
	Status     string    `db:"status"`
	CreatedAt  time.Time `db:"created_at"`
	UpdatedAt  time.Time `db:"updated_at"`
}

type CartItem struct {
	ID         uuid.UUID `db:"id"`
	CartID     uuid.UUID `db:"cart_id"`
	MenuItemID uuid.UUID `db:"menu_item_id"`
	Quantity   int32     `db:"quantity"`
	CreatedAt  time.Time `db:"created_at"`
}

type Order struct {
	ID             uuid.UUID `db:"id"`
	Number         string    `db:"number"`
	CustomerID     uuid.UUID `db:"customer_id"`
	VenueID        uuid.UUID `db:"venue_id"`
	VenueName      string    `db:"venue_name"`
	Status         string    `db:"status"`
	ItemsTotal     int64     `db:"items_total"`
	DeliveryFee    int64     `db:"delivery_fee"`
	Total          int64     `db:"total"`
	RecipientName  string    `db:"recipient_name"`
	Phone          string    `db:"phone"`
	Address        string    `db:"address"`
	Comment        string    `db:"comment"`
	StatusReason   string    `db:"status_reason"`
	PrepMinutes    int32     `db:"prep_minutes"`
	IdempotencyKey string    `db:"idempotency_key"`
	Version        int32     `db:"version"`
	CreatedAt      time.Time `db:"created_at"`
	UpdatedAt      time.Time `db:"updated_at"`
}

type OrderItem struct {
	ID         uuid.UUID `db:"id"`
	OrderID    uuid.UUID `db:"order_id"`
	MenuItemID uuid.UUID `db:"menu_item_id"`
	Name       string    `db:"name"`
	UnitPrice  int64     `db:"unit_price"`
	Quantity   int32     `db:"quantity"`
	LineTotal  int64     `db:"line_total"`
}

type OrderStatusHistory struct {
	ID         uuid.UUID `db:"id"`
	OrderID    uuid.UUID `db:"order_id"`
	FromStatus *string   `db:"from_status"`
	ToStatus   string    `db:"to_status"`
	Actor      string    `db:"actor"`
	Reason     string    `db:"reason"`
	OccurredAt time.Time `db:"occurred_at"`
}

type OutboxEvent struct {
	ID            uuid.UUID  `db:"id"`
	EventType     string     `db:"event_type"`
	AggregateType string     `db:"aggregate_type"`
	AggregateID   uuid.UUID  `db:"aggregate_id"`
	VenueID       uuid.UUID  `db:"venue_id"`
	Payload       []byte     `db:"payload"`
	OccurredAt    time.Time  `db:"occurred_at"`
	PublishedAt   *time.Time `db:"published_at"`
}
