package grpc

import (
	"context"
	"log/slog"

	kitchenv1 "backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/api/gen/kitchen/v1"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/service"
	"github.com/google/uuid"
)

type PartnerUseCase interface {
	Authenticate(ctx context.Context, apiKey string) (*domain.Venue, error)
	GetVenue(ctx context.Context, venueID uuid.UUID) (*domain.Venue, error)
	UpdateVenue(ctx context.Context, venueID uuid.UUID, cmd service.UpdateVenueCommand) (*domain.Venue, error)
	GetMenu(ctx context.Context, venueID uuid.UUID) (domain.Menu, error)
	SaveCategory(ctx context.Context, venueID uuid.UUID, cmd service.SaveCategoryCommand) (*domain.MenuCategory, error)
	CreateItem(ctx context.Context, venueID uuid.UUID, cmd service.CreateItemCommand) (*domain.MenuItem, error)
	UpdateItem(ctx context.Context, venueID, itemID uuid.UUID, cmd service.UpdateItemCommand) (*domain.MenuItem, error)
	SetStock(ctx context.Context, venueID, itemID uuid.UUID, quantity int, available *bool) (*domain.MenuItem, error)
}

type FulfillmentUseCase interface {
	List(ctx context.Context, venueID uuid.UUID, statuses []domain.OrderStatus, page service.Page) ([]domain.Order, int, error)
	ListActive(ctx context.Context, venueID uuid.UUID) ([]domain.Order, error)
	Get(ctx context.Context, venueID, orderID uuid.UUID) (*domain.Order, error)
	Accept(ctx context.Context, venueID, orderID uuid.UUID, prepMinutes int) (*domain.Order, error)
	Reject(ctx context.Context, venueID, orderID uuid.UUID, reason string) (*domain.Order, error)
	UpdateStatus(ctx context.Context, venueID, orderID uuid.UUID, target domain.OrderStatus, reason string) (*domain.Order, error)
}

type partnerServer struct {
	kitchenv1.UnimplementedPartnerServiceServer

	partners    PartnerUseCase
	fulfillment FulfillmentUseCase
	events      service.EventSubscriber
	logger      *slog.Logger
}

func NewPartnerServer(
	partners PartnerUseCase,
	fulfillment FulfillmentUseCase,
	events service.EventSubscriber,
	logger *slog.Logger,
) kitchenv1.PartnerServiceServer {
	return &partnerServer{
		partners:    partners,
		fulfillment: fulfillment,
		events:      events,
		logger:      logger,
	}
}

func (s *partnerServer) SubscribeOrderEvents(
	req *kitchenv1.SubscribeOrderEventsRequest,
	stream kitchenv1.PartnerService_SubscribeOrderEventsServer,
) error {
	ctx := stream.Context()
	venue, err := venueFrom(ctx)
	if err != nil {
		return err
	}

	events, cancel := s.events.Subscribe(venue.ID)
	defer cancel()

	if req.GetReplayActiveOrders() {
		active, err := s.fulfillment.ListActive(ctx, venue.ID)
		if err != nil {
			return err
		}
		for i := range active {
			order := active[i]
			replay := domain.OrderEvent{
				ID:         uuid.New(),
				Type:       domain.EventOrderCreated,
				VenueID:    venue.ID,
				OrderID:    order.ID,
				Status:     order.Status,
				OccurredAt: order.UpdatedAt,
				Order:      &order,
			}
			if err := stream.Send(orderEventToProto(replay)); err != nil {
				return err
			}
		}
		s.logger.Info("replayed active orders to partner",
			slog.String("venue_id", venue.ID.String()),
			slog.Int("count", len(active)))
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, open := <-events:
			if !open {

				return nil
			}
			if err := stream.Send(orderEventToProto(event)); err != nil {
				return err
			}
		}
	}
}

func (s *partnerServer) ListOrders(ctx context.Context, req *kitchenv1.ListOrdersRequest) (*kitchenv1.ListOrdersResponse, error) {
	venue, err := venueFrom(ctx)
	if err != nil {
		return nil, err
	}

	statuses := make([]domain.OrderStatus, 0, len(req.GetStatuses()))
	for _, protoStatus := range req.GetStatuses() {
		status, err := orderStatusFromProto(protoStatus)
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, status)
	}

	orders, total, err := s.fulfillment.List(ctx, venue.ID, statuses, service.Page{
		Limit:  int(req.GetLimit()),
		Offset: int(req.GetOffset()),
	})
	if err != nil {
		return nil, err
	}

	response := &kitchenv1.ListOrdersResponse{Total: int32(total)} //nolint:gosec
	for i := range orders {
		response.Orders = append(response.Orders, orderToProto(&orders[i]))
	}
	return response, nil
}

func (s *partnerServer) GetOrder(ctx context.Context, req *kitchenv1.GetOrderRequest) (*kitchenv1.Order, error) {
	venue, orderID, err := venueAndID(ctx, req.GetOrderId(), "order_id")
	if err != nil {
		return nil, err
	}
	order, err := s.fulfillment.Get(ctx, venue.ID, orderID)
	if err != nil {
		return nil, err
	}
	return orderToProto(order), nil
}

func (s *partnerServer) AcceptOrder(ctx context.Context, req *kitchenv1.AcceptOrderRequest) (*kitchenv1.Order, error) {
	venue, orderID, err := venueAndID(ctx, req.GetOrderId(), "order_id")
	if err != nil {
		return nil, err
	}
	order, err := s.fulfillment.Accept(ctx, venue.ID, orderID, int(req.GetPrepMinutes()))
	if err != nil {
		return nil, err
	}
	return orderToProto(order), nil
}

func (s *partnerServer) RejectOrder(ctx context.Context, req *kitchenv1.RejectOrderRequest) (*kitchenv1.Order, error) {
	venue, orderID, err := venueAndID(ctx, req.GetOrderId(), "order_id")
	if err != nil {
		return nil, err
	}
	order, err := s.fulfillment.Reject(ctx, venue.ID, orderID, req.GetReason())
	if err != nil {
		return nil, err
	}
	return orderToProto(order), nil
}

func (s *partnerServer) UpdateOrderStatus(ctx context.Context, req *kitchenv1.UpdateOrderStatusRequest) (*kitchenv1.Order, error) {
	venue, orderID, err := venueAndID(ctx, req.GetOrderId(), "order_id")
	if err != nil {
		return nil, err
	}
	target, err := orderStatusFromProto(req.GetStatus())
	if err != nil {
		return nil, err
	}

	order, err := s.fulfillment.UpdateStatus(ctx, venue.ID, orderID, target, req.GetReason())
	if err != nil {
		return nil, err
	}
	return orderToProto(order), nil
}

func (s *partnerServer) GetVenue(ctx context.Context, _ *kitchenv1.GetVenueRequest) (*kitchenv1.Venue, error) {
	venue, err := venueFrom(ctx)
	if err != nil {
		return nil, err
	}
	current, err := s.partners.GetVenue(ctx, venue.ID)
	if err != nil {
		return nil, err
	}
	return venueToProto(*current), nil
}

func (s *partnerServer) SetVenueOpen(ctx context.Context, req *kitchenv1.SetVenueOpenRequest) (*kitchenv1.Venue, error) {
	venue, err := venueFrom(ctx)
	if err != nil {
		return nil, err
	}
	isOpen := req.GetIsOpen()
	updated, err := s.partners.UpdateVenue(ctx, venue.ID, service.UpdateVenueCommand{IsOpen: &isOpen})
	if err != nil {
		return nil, err
	}
	return venueToProto(*updated), nil
}

func (s *partnerServer) GetMenu(ctx context.Context, _ *kitchenv1.GetMenuRequest) (*kitchenv1.Menu, error) {
	venue, err := venueFrom(ctx)
	if err != nil {
		return nil, err
	}
	menu, err := s.partners.GetMenu(ctx, venue.ID)
	if err != nil {
		return nil, err
	}
	return menuToProto(menu), nil
}

func (s *partnerServer) UpsertCategory(ctx context.Context, req *kitchenv1.UpsertCategoryRequest) (*kitchenv1.MenuCategory, error) {
	venue, err := venueFrom(ctx)
	if err != nil {
		return nil, err
	}

	cmd := service.SaveCategoryCommand{Name: req.GetName(), Position: int(req.GetPosition())}
	if req.GetId() != "" {
		id, err := parseUUID(req.GetId(), "id")
		if err != nil {
			return nil, err
		}
		cmd.ID = &id
	}

	category, err := s.partners.SaveCategory(ctx, venue.ID, cmd)
	if err != nil {
		return nil, err
	}
	return categoryToProto(*category), nil
}

func (s *partnerServer) UpsertMenuItem(ctx context.Context, req *kitchenv1.UpsertMenuItemRequest) (*kitchenv1.MenuItem, error) {
	venue, err := venueFrom(ctx)
	if err != nil {
		return nil, err
	}
	categoryID, err := parseUUID(req.GetCategoryId(), "category_id")
	if err != nil {
		return nil, err
	}

	if req.GetId() == "" {
		item, err := s.partners.CreateItem(ctx, venue.ID, service.CreateItemCommand{
			CategoryID:    categoryID,
			Name:          req.GetName(),
			Description:   req.GetDescription(),
			Price:         domain.Money(req.GetPrice().GetMinorUnits()),
			StockQuantity: int(req.GetStockQuantity()),
			IsAvailable:   req.GetIsAvailable(),
			WeightGrams:   int(req.GetWeightGrams()),
			Position:      int(req.GetPosition()),
		})
		if err != nil {
			return nil, err
		}
		return menuItemToProto(*item), nil
	}

	itemID, err := parseUUID(req.GetId(), "id")
	if err != nil {
		return nil, err
	}
	price := domain.Money(req.GetPrice().GetMinorUnits())
	name := req.GetName()
	description := req.GetDescription()
	isAvailable := req.GetIsAvailable()
	weight := int(req.GetWeightGrams())
	position := int(req.GetPosition())

	item, err := s.partners.UpdateItem(ctx, venue.ID, itemID, service.UpdateItemCommand{
		CategoryID:  &categoryID,
		Name:        &name,
		Description: &description,
		Price:       &price,
		IsAvailable: &isAvailable,
		WeightGrams: &weight,
		Position:    &position,
	})
	if err != nil {
		return nil, err
	}
	return menuItemToProto(*item), nil
}

func (s *partnerServer) SetItemStock(ctx context.Context, req *kitchenv1.SetItemStockRequest) (*kitchenv1.MenuItem, error) {
	venue, itemID, err := venueAndID(ctx, req.GetItemId(), "item_id")
	if err != nil {
		return nil, err
	}
	available := req.GetIsAvailable()

	item, err := s.partners.SetStock(ctx, venue.ID, itemID, int(req.GetStockQuantity()), &available)
	if err != nil {
		return nil, err
	}
	return menuItemToProto(*item), nil
}

func venueAndID(ctx context.Context, raw, field string) (*domain.Venue, uuid.UUID, error) {
	venue, err := venueFrom(ctx)
	if err != nil {
		return nil, uuid.Nil, err
	}
	id, err := parseUUID(raw, field)
	if err != nil {
		return nil, uuid.Nil, err
	}
	return venue, id, nil
}

func parseUUID(raw, field string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, domain.Invalid("invalid_"+field, "%s must be a UUID", field)
	}
	return id, nil
}
