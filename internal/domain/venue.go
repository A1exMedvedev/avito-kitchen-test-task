package domain

import (
	"time"

	"github.com/google/uuid"
)

type VenueStatus string

const (
	VenueStatusActive    VenueStatus = "active"
	VenueStatusSuspended VenueStatus = "suspended"
)

type Venue struct {
	ID          uuid.UUID
	Slug        string
	Name        string
	Description string
	Cuisines    []string
	City        string
	Address     string

	Status VenueStatus
	IsOpen bool

	AvgPrepMinutes int
	MinOrderAmount Money
	DeliveryFee    Money
	Rating         float64

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (v Venue) AcceptsOrders() bool {
	return v.Status == VenueStatusActive && v.IsOpen
}

func (v Venue) EnsureAcceptsOrders() error {
	switch {
	case v.Status != VenueStatusActive:
		return Conflict("venue_unavailable", "venue %q is not available on the platform", v.Name)
	case !v.IsOpen:
		return Conflict("venue_closed", "venue %q is closed right now", v.Name)
	default:
		return nil
	}
}

func (v Venue) Validate() error {
	switch {
	case v.AvgPrepMinutes < MinPrepMinutes || v.AvgPrepMinutes > MaxPrepMinutes:
		return Invalid("prep_minutes_invalid",
			"preparation time must be between %d and %d minutes", MinPrepMinutes, MaxPrepMinutes)
	case v.MinOrderAmount.IsNegative():
		return Invalid("min_order_invalid", "minimum order amount must not be negative")
	case v.DeliveryFee.IsNegative():
		return Invalid("delivery_fee_invalid", "delivery fee must not be negative")
	case len([]rune(v.Description)) > MaxVenueDescriptionLength:
		return Invalid("venue_description_too_long",
			"description must be at most %d characters", MaxVenueDescriptionLength)
	default:
		return nil
	}
}
