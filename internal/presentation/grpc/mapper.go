package grpc

import (
	kitchenv1 "backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/api/gen/kitchen/v1"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var statusToProto = map[domain.OrderStatus]kitchenv1.OrderStatus{
	domain.OrderStatusCreated:    kitchenv1.OrderStatus_ORDER_STATUS_CREATED,
	domain.OrderStatusAccepted:   kitchenv1.OrderStatus_ORDER_STATUS_ACCEPTED,
	domain.OrderStatusCooking:    kitchenv1.OrderStatus_ORDER_STATUS_COOKING,
	domain.OrderStatusReady:      kitchenv1.OrderStatus_ORDER_STATUS_READY,
	domain.OrderStatusInDelivery: kitchenv1.OrderStatus_ORDER_STATUS_IN_DELIVERY,
	domain.OrderStatusDelivered:  kitchenv1.OrderStatus_ORDER_STATUS_DELIVERED,
	domain.OrderStatusRejected:   kitchenv1.OrderStatus_ORDER_STATUS_REJECTED,
	domain.OrderStatusCancelled:  kitchenv1.OrderStatus_ORDER_STATUS_CANCELLED,
}

var statusFromProto = func() map[kitchenv1.OrderStatus]domain.OrderStatus {
	reversed := make(map[kitchenv1.OrderStatus]domain.OrderStatus, len(statusToProto))
	for status, protoStatus := range statusToProto {
		reversed[protoStatus] = status
	}
	return reversed
}()

func orderStatusToProto(status domain.OrderStatus) kitchenv1.OrderStatus {
	if protoStatus, ok := statusToProto[status]; ok {
		return protoStatus
	}
	return kitchenv1.OrderStatus_ORDER_STATUS_UNSPECIFIED
}

func orderStatusFromProto(protoStatus kitchenv1.OrderStatus) (domain.OrderStatus, error) {
	status, ok := statusFromProto[protoStatus]
	if !ok || protoStatus == kitchenv1.OrderStatus_ORDER_STATUS_UNSPECIFIED {
		return "", domain.Invalid("invalid_status", "order status must be specified")
	}
	return status, nil
}

func moneyToProto(amount domain.Money) *kitchenv1.Money {
	return &kitchenv1.Money{MinorUnits: int64(amount), Currency: domain.Currency}
}

func venueToProto(venue domain.Venue) *kitchenv1.Venue {
	return &kitchenv1.Venue{
		Id:             venue.ID.String(),
		Slug:           venue.Slug,
		Name:           venue.Name,
		Description:    venue.Description,
		Cuisines:       venue.Cuisines,
		City:           venue.City,
		Address:        venue.Address,
		Status:         string(venue.Status),
		IsOpen:         venue.IsOpen,
		AvgPrepMinutes: int32(venue.AvgPrepMinutes), //nolint:gosec
		MinOrderAmount: moneyToProto(venue.MinOrderAmount),
		DeliveryFee:    moneyToProto(venue.DeliveryFee),
		Rating:         venue.Rating,
	}
}

func categoryToProto(category domain.MenuCategory) *kitchenv1.MenuCategory {
	return &kitchenv1.MenuCategory{
		Id:       category.ID.String(),
		Name:     category.Name,
		Position: int32(category.Position), //nolint:gosec
	}
}

func menuItemToProto(item domain.MenuItem) *kitchenv1.MenuItem {
	return &kitchenv1.MenuItem{
		Id:            item.ID.String(),
		CategoryId:    item.CategoryID.String(),
		Name:          item.Name,
		Description:   item.Description,
		Price:         moneyToProto(item.Price),
		StockQuantity: int32(item.StockQuantity), //nolint:gosec
		IsAvailable:   item.IsAvailable,
		WeightGrams:   int32(item.WeightGrams), //nolint:gosec
		Position:      int32(item.Position),    //nolint:gosec
	}
}

func menuToProto(menu domain.Menu) *kitchenv1.Menu {
	protoMenu := &kitchenv1.Menu{}
	for _, section := range menu.Sections {
		protoMenu.Categories = append(protoMenu.Categories, categoryToProto(section.Category))
		for _, item := range section.Items {
			protoMenu.Items = append(protoMenu.Items, menuItemToProto(item))
		}
	}
	return protoMenu
}

func orderToProto(order *domain.Order) *kitchenv1.Order {
	if order == nil {
		return nil
	}

	items := make([]*kitchenv1.OrderItem, 0, len(order.Items))
	for _, item := range order.Items {
		items = append(items, &kitchenv1.OrderItem{
			MenuItemId: item.MenuItemID.String(),
			Name:       item.Name,
			UnitPrice:  moneyToProto(item.UnitPrice),
			Quantity:   int32(item.Quantity), //nolint:gosec
			LineTotal:  moneyToProto(item.LineTotal),
		})
	}

	return &kitchenv1.Order{
		Id:          order.ID.String(),
		Number:      order.Number,
		CustomerId:  order.CustomerID.String(),
		VenueId:     order.VenueID.String(),
		Status:      orderStatusToProto(order.Status),
		Items:       items,
		ItemsTotal:  moneyToProto(order.ItemsTotal),
		DeliveryFee: moneyToProto(order.DeliveryFee),
		Total:       moneyToProto(order.Total),
		Delivery: &kitchenv1.Delivery{
			RecipientName: order.Delivery.RecipientName,
			Phone:         order.Delivery.Phone,
			Address:       order.Delivery.Address,
			Comment:       order.Delivery.Comment,
		},
		StatusReason: order.StatusReason,
		PrepMinutes:  int32(order.PrepMinutes), //nolint:gosec
		CreatedAt:    timestamppb.New(order.CreatedAt),
		UpdatedAt:    timestamppb.New(order.UpdatedAt),
	}
}

var eventTypeToProto = map[domain.EventType]kitchenv1.OrderEventType{
	domain.EventOrderCreated:       kitchenv1.OrderEventType_ORDER_EVENT_TYPE_CREATED,
	domain.EventOrderStatusChanged: kitchenv1.OrderEventType_ORDER_EVENT_TYPE_STATUS_CHANGED,
}

func orderEventToProto(event domain.OrderEvent) *kitchenv1.OrderEvent {
	protoType, ok := eventTypeToProto[event.Type]
	if !ok {
		protoType = kitchenv1.OrderEventType_ORDER_EVENT_TYPE_UNSPECIFIED
	}
	return &kitchenv1.OrderEvent{
		EventId:        event.ID.String(),
		Type:           protoType,
		OccurredAt:     timestamppb.New(event.OccurredAt),
		Order:          orderToProto(event.Order),
		PreviousStatus: orderStatusToProto(event.PreviousStatus),
	}
}
