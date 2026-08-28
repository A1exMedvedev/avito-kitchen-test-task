package service

import (
	"context"
	"strings"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"github.com/google/uuid"
)

type partnerService struct {
	venues VenueRepository
	menus  MenuRepository
	clock  Clock
}

func NewPartnerService(venues VenueRepository, menus MenuRepository, clock Clock) PartnerService {
	return &partnerService{venues: venues, menus: menus, clock: clock}
}

func (s *partnerService) Authenticate(ctx context.Context, apiKey string) (*domain.Venue, error) {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return nil, domain.Unauthenticated("api_key_required", "X-Api-Key header is required")
	}
	venue, err := s.venues.ResolveAPIKey(ctx, apiKey)
	if err != nil {
		return nil, err
	}
	if venue.Status != domain.VenueStatusActive {
		return nil, domain.Forbidden("venue_suspended",
			"venue is suspended on the platform; contact support")
	}
	return venue, nil
}

func (s *partnerService) GetVenue(ctx context.Context, venueID uuid.UUID) (*domain.Venue, error) {
	return s.venues.Get(ctx, venueID)
}

func (s *partnerService) UpdateVenue(ctx context.Context, venueID uuid.UUID, cmd UpdateVenueCommand) (*domain.Venue, error) {
	venue, err := s.venues.Get(ctx, venueID)
	if err != nil {
		return nil, err
	}

	if cmd.IsOpen != nil {
		venue.IsOpen = *cmd.IsOpen
	}
	if cmd.AvgPrepMinutes != nil {
		venue.AvgPrepMinutes = *cmd.AvgPrepMinutes
	}
	if cmd.MinOrderAmount != nil {
		venue.MinOrderAmount = *cmd.MinOrderAmount
	}
	if cmd.DeliveryFee != nil {
		venue.DeliveryFee = *cmd.DeliveryFee
	}
	if cmd.Description != nil {
		venue.Description = *cmd.Description
	}
	if err := venue.Validate(); err != nil {
		return nil, err
	}
	venue.UpdatedAt = s.clock()

	if err := s.venues.Update(ctx, venue); err != nil {
		return nil, err
	}
	return venue, nil
}

func (s *partnerService) GetMenu(ctx context.Context, venueID uuid.UUID) (domain.Menu, error) {
	categories, err := s.menus.ListCategories(ctx, venueID)
	if err != nil {
		return domain.Menu{}, err
	}
	items, err := s.menus.ListItems(ctx, venueID, true)
	if err != nil {
		return domain.Menu{}, err
	}
	return domain.BuildMenu(venueID, categories, items), nil
}

func (s *partnerService) SaveCategory(ctx context.Context, venueID uuid.UUID, cmd SaveCategoryCommand) (*domain.MenuCategory, error) {
	name := strings.TrimSpace(cmd.Name)
	now := s.clock()
	category := &domain.MenuCategory{
		ID:        uuid.New(),
		VenueID:   venueID,
		Name:      name,
		Position:  cmd.Position,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if cmd.ID != nil {
		existing, err := s.menus.GetCategory(ctx, venueID, *cmd.ID)
		if err != nil {
			return nil, err
		}
		existing.Name = name
		existing.Position = cmd.Position
		existing.UpdatedAt = now
		category = existing
	}

	if err := category.Validate(); err != nil {
		return nil, err
	}
	if err := s.menus.SaveCategory(ctx, category); err != nil {
		return nil, err
	}
	return category, nil
}

func (s *partnerService) DeleteCategory(ctx context.Context, venueID, categoryID uuid.UUID) error {
	count, err := s.menus.CountCategoryItems(ctx, venueID, categoryID)
	if err != nil {
		return err
	}
	if count > 0 {
		return domain.Conflict("category_not_empty",
			"category still holds %d item(s); move or withdraw them first", count)
	}
	return s.menus.DeleteCategory(ctx, venueID, categoryID)
}

func (s *partnerService) CreateItem(ctx context.Context, venueID uuid.UUID, cmd CreateItemCommand) (*domain.MenuItem, error) {
	if _, err := s.menus.GetCategory(ctx, venueID, cmd.CategoryID); err != nil {
		return nil, err
	}

	now := s.clock()
	item := &domain.MenuItem{
		ID:            uuid.New(),
		VenueID:       venueID,
		CategoryID:    cmd.CategoryID,
		Name:          strings.TrimSpace(cmd.Name),
		Description:   cmd.Description,
		Price:         cmd.Price,
		IsAvailable:   cmd.IsAvailable,
		StockQuantity: cmd.StockQuantity,
		WeightGrams:   cmd.WeightGrams,
		Position:      cmd.Position,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := item.Validate(); err != nil {
		return nil, err
	}
	if err := s.menus.SaveItem(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *partnerService) UpdateItem(ctx context.Context, venueID, itemID uuid.UUID, cmd UpdateItemCommand) (*domain.MenuItem, error) {
	item, err := s.menus.GetItem(ctx, venueID, itemID)
	if err != nil {
		return nil, err
	}

	if cmd.CategoryID != nil {
		if _, err := s.menus.GetCategory(ctx, venueID, *cmd.CategoryID); err != nil {
			return nil, err
		}
		item.CategoryID = *cmd.CategoryID
	}
	if cmd.Name != nil {
		item.Name = strings.TrimSpace(*cmd.Name)
	}
	if cmd.Description != nil {
		item.Description = *cmd.Description
	}
	if cmd.Price != nil {
		item.Price = *cmd.Price
	}
	if cmd.IsAvailable != nil {
		item.IsAvailable = *cmd.IsAvailable
	}
	if cmd.WeightGrams != nil {
		item.WeightGrams = *cmd.WeightGrams
	}
	if cmd.Position != nil {
		item.Position = *cmd.Position
	}
	item.UpdatedAt = s.clock()

	if err := item.Validate(); err != nil {
		return nil, err
	}
	if err := s.menus.SaveItem(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *partnerService) SetStock(ctx context.Context, venueID, itemID uuid.UUID, quantity int, available *bool) (*domain.MenuItem, error) {
	if quantity < 0 || quantity > domain.MaxItemStockQuantity {
		return nil, domain.Invalid("item_stock_invalid",
			"stock quantity must be between 0 and %d", domain.MaxItemStockQuantity)
	}

	item, err := s.menus.GetItem(ctx, venueID, itemID)
	if err != nil {
		return nil, err
	}
	item.StockQuantity = quantity
	if available != nil {
		item.IsAvailable = *available
	}
	item.UpdatedAt = s.clock()

	if err := s.menus.SaveItem(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *partnerService) DeleteItem(ctx context.Context, venueID, itemID uuid.UUID) error {
	return s.menus.DeleteItem(ctx, venueID, itemID)
}
