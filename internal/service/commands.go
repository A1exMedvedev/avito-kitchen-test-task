package service

import (
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"github.com/google/uuid"
)

type CheckoutCommand struct {
	CustomerID     uuid.UUID
	IdempotencyKey string
	Delivery       domain.Delivery
}

type UpdateVenueCommand struct {
	IsOpen         *bool
	AvgPrepMinutes *int
	MinOrderAmount *domain.Money
	DeliveryFee    *domain.Money
	Description    *string
}

type SaveCategoryCommand struct {
	ID       *uuid.UUID
	Name     string
	Position int
}

type CreateItemCommand struct {
	CategoryID    uuid.UUID
	Name          string
	Description   string
	Price         domain.Money
	StockQuantity int
	IsAvailable   bool
	WeightGrams   int
	Position      int
}

type UpdateItemCommand struct {
	CategoryID  *uuid.UUID
	Name        *string
	Description *string
	Price       *domain.Money
	IsAvailable *bool
	WeightGrams *int
	Position    *int
}
