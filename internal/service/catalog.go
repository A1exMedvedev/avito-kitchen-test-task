package service

import (
	"context"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"github.com/google/uuid"
)

type catalogService struct {
	venues VenueRepository
	menus  MenuRepository
}

func NewCatalogService(venues VenueRepository, menus MenuRepository) CatalogService {
	return &catalogService{venues: venues, menus: menus}
}

func (s *catalogService) ListVenues(ctx context.Context, filter VenueFilter) ([]domain.Venue, int, error) {
	filter.Page = filter.Page.Normalize()
	return s.venues.List(ctx, filter)
}

func (s *catalogService) GetVenue(ctx context.Context, id uuid.UUID) (*domain.Venue, error) {
	venue, err := s.venues.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if venue.Status != domain.VenueStatusActive {
		return nil, domain.NotFound("venue_not_found", "venue not found")
	}
	return venue, nil
}

func (s *catalogService) GetMenu(ctx context.Context, venueID uuid.UUID) (domain.Menu, error) {
	if _, err := s.GetVenue(ctx, venueID); err != nil {
		return domain.Menu{}, err
	}

	categories, err := s.menus.ListCategories(ctx, venueID)
	if err != nil {
		return domain.Menu{}, err
	}
	items, err := s.menus.ListItems(ctx, venueID, false)
	if err != nil {
		return domain.Menu{}, err
	}
	return domain.BuildMenu(venueID, categories, items), nil
}
