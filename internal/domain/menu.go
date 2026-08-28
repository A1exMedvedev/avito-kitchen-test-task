package domain

import (
	"time"

	"github.com/google/uuid"
)

type MenuCategory struct {
	ID       uuid.UUID
	VenueID  uuid.UUID
	Name     string
	Position int

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (c MenuCategory) Validate() error {
	switch {
	case c.Name == "":
		return Invalid("category_name_required", "category name must not be empty")
	case len([]rune(c.Name)) > MaxCategoryNameLength:
		return Invalid("category_name_too_long",
			"category name must be at most %d characters", MaxCategoryNameLength)
	case c.Position < 0:
		return Invalid("category_position_invalid", "position must not be negative")
	default:
		return nil
	}
}

type MenuItem struct {
	ID          uuid.UUID
	VenueID     uuid.UUID
	CategoryID  uuid.UUID
	Name        string
	Description string
	Price       Money

	IsAvailable   bool
	StockQuantity int

	WeightGrams int
	Position    int

	CreatedAt time.Time
	UpdatedAt time.Time

	DeletedAt *time.Time
}

func (i MenuItem) Withdrawn() bool { return i.DeletedAt != nil }

func (i MenuItem) Orderable() bool {
	return !i.Withdrawn() && i.IsAvailable && i.StockQuantity > 0
}

func (i MenuItem) CheckQuantity(requested int) (UnavailableReason, bool) {
	switch {
	case i.Withdrawn():
		return ReasonWithdrawn, false
	case !i.IsAvailable:
		return ReasonDisabled, false
	case i.StockQuantity <= 0:
		return ReasonSoldOut, false
	case i.StockQuantity < requested:
		return ReasonInsufficientStock, false
	default:
		return "", true
	}
}

func (i MenuItem) Validate() error {
	switch {
	case i.Name == "":
		return Invalid("item_name_required", "menu item name must not be empty")
	case len([]rune(i.Name)) > MaxItemNameLength:
		return Invalid("item_name_too_long",
			"menu item name must be at most %d characters", MaxItemNameLength)
	case len([]rune(i.Description)) > MaxItemDescriptionLength:
		return Invalid("item_description_too_long",
			"menu item description must be at most %d characters", MaxItemDescriptionLength)
	case i.Price <= 0:
		return Invalid("item_price_invalid", "menu item price must be positive")
	case i.StockQuantity < 0:
		return Invalid("item_stock_invalid", "stock quantity must not be negative")
	case i.StockQuantity > MaxItemStockQuantity:
		return Invalid("item_stock_invalid",
			"stock quantity must be at most %d", MaxItemStockQuantity)
	case i.WeightGrams < 0 || i.WeightGrams > MaxItemWeightGrams:
		return Invalid("item_weight_invalid",
			"weight must be between 0 and %d grams", MaxItemWeightGrams)
	case i.Position < 0:
		return Invalid("item_position_invalid", "position must not be negative")
	case i.CategoryID == uuid.Nil:
		return Invalid("item_category_required", "menu item must belong to a category")
	default:
		return nil
	}
}

type Menu struct {
	VenueID  uuid.UUID
	Sections []MenuSection
}

type MenuSection struct {
	Category MenuCategory
	Items    []MenuItem
}

func BuildMenu(venueID uuid.UUID, categories []MenuCategory, items []MenuItem) Menu {
	byCategory := make(map[uuid.UUID][]MenuItem, len(categories))
	for _, item := range items {
		byCategory[item.CategoryID] = append(byCategory[item.CategoryID], item)
	}

	sections := make([]MenuSection, 0, len(categories))
	for _, category := range categories {
		sections = append(sections, MenuSection{
			Category: category,
			Items:    byCategory[category.ID],
		})
	}
	return Menu{VenueID: venueID, Sections: sections}
}
