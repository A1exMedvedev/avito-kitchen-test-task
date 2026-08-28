package http

import (
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/api/gen/openapi/kitchenapi"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"github.com/google/uuid"
)

func optional[T comparable](value T) *T {
	var zero T
	if value == zero {
		return nil
	}
	return &value
}

func money(amount domain.Money) kitchenapi.Money {
	return kitchenapi.Money{Amount: int64(amount), Currency: domain.Currency}
}

func venueToAPI(venue domain.Venue) kitchenapi.Venue {
	cuisines := venue.Cuisines
	if cuisines == nil {
		cuisines = []string{}
	}
	return kitchenapi.Venue{
		Id:             venue.ID,
		Slug:           venue.Slug,
		Name:           venue.Name,
		Description:    optional(venue.Description),
		Cuisines:       cuisines,
		City:           venue.City,
		Address:        venue.Address,
		Status:         kitchenapi.VenueStatus(venue.Status),
		IsOpen:         venue.IsOpen,
		AcceptsOrders:  venue.AcceptsOrders(),
		AvgPrepMinutes: venue.AvgPrepMinutes,
		MinOrderAmount: money(venue.MinOrderAmount),
		DeliveryFee:    money(venue.DeliveryFee),
		Rating:         venue.Rating,
	}
}

func venuePageToAPI(venues []domain.Venue, total, limit, offset int) kitchenapi.VenuePage {
	items := make([]kitchenapi.Venue, 0, len(venues))
	for _, venue := range venues {
		items = append(items, venueToAPI(venue))
	}
	return kitchenapi.VenuePage{Items: items, Total: total, Limit: limit, Offset: offset}
}

func menuItemToAPI(item domain.MenuItem) kitchenapi.MenuItem {
	return kitchenapi.MenuItem{
		Id:          item.ID,
		CategoryId:  item.CategoryID,
		Name:        item.Name,
		Description: optional(item.Description),
		Price:       money(item.Price),
		IsAvailable: item.IsAvailable,
		Availability: kitchenapi.ItemAvailability{
			Orderable:     item.Orderable(),
			StockQuantity: item.StockQuantity,
		},
		WeightGrams: optional(item.WeightGrams),
		Position:    item.Position,
	}
}

func menuToAPI(menu domain.Menu) kitchenapi.Menu {
	sections := make([]kitchenapi.MenuSection, 0, len(menu.Sections))
	for _, section := range menu.Sections {
		items := make([]kitchenapi.MenuItem, 0, len(section.Items))
		for _, item := range section.Items {
			items = append(items, menuItemToAPI(item))
		}
		sections = append(sections, kitchenapi.MenuSection{
			Category: kitchenapi.MenuCategory{
				Id:       section.Category.ID,
				Name:     section.Category.Name,
				Position: section.Category.Position,
			},
			Items: items,
		})
	}
	return kitchenapi.Menu{VenueId: menu.VenueID, Sections: sections}
}

func categoryToAPI(category domain.MenuCategory) kitchenapi.MenuCategory {
	return kitchenapi.MenuCategory{
		Id:       category.ID,
		Name:     category.Name,
		Position: category.Position,
	}
}

func cartToAPI(cart domain.PricedCart) kitchenapi.Cart {
	lines := make([]kitchenapi.CartLine, 0, len(cart.Lines))
	for _, line := range cart.Lines {
		apiLine := kitchenapi.CartLine{
			MenuItemId:        line.MenuItemID,
			Name:              line.Name,
			Quantity:          line.Quantity,
			UnitPrice:         money(line.UnitPrice),
			LineTotal:         money(line.LineTotal),
			Orderable:         line.Orderable,
			AvailableQuantity: new(line.AvailableQuantity),
		}
		if line.Problem != "" {
			apiLine.Problem = new(kitchenapi.CartLineProblem(line.Problem))
		}
		lines = append(lines, apiLine)
	}

	result := kitchenapi.Cart{
		Lines:         lines,
		ItemsTotal:    money(cart.ItemsTotal),
		DeliveryFee:   money(cart.DeliveryFee),
		Total:         money(cart.Total),
		CheckoutReady: cart.CheckoutReady,
	}

	if cart.Cart.ID != uuid.Nil {
		result.Id = new(cart.Cart.ID)
	}
	if cart.Venue != nil {
		result.VenueId = new(cart.Venue.ID)
		result.VenueName = new(cart.Venue.Name)
		result.MinOrderAmount = new(money(cart.MinOrderAmount))
	}
	if len(cart.Blockers) > 0 {
		result.Blockers = new(cart.Blockers)
	}
	return result
}

func orderItemsToAPI(items []domain.OrderItem) []kitchenapi.OrderItem {
	apiItems := make([]kitchenapi.OrderItem, 0, len(items))
	for _, item := range items {
		apiItems = append(apiItems, kitchenapi.OrderItem{
			MenuItemId: item.MenuItemID,
			Name:       item.Name,
			Quantity:   item.Quantity,
			UnitPrice:  money(item.UnitPrice),
			LineTotal:  money(item.LineTotal),
		})
	}
	return apiItems
}

func deliveryToAPI(delivery domain.Delivery) kitchenapi.Delivery {
	return kitchenapi.Delivery{
		RecipientName: delivery.RecipientName,
		Phone:         delivery.Phone,
		Address:       delivery.Address,
		Comment:       optional(delivery.Comment),
	}
}

func orderToAPI(order domain.Order) kitchenapi.Order {
	return kitchenapi.Order{
		Id:           order.ID,
		Number:       order.Number,
		VenueId:      order.VenueID,
		VenueName:    optional(order.VenueName),
		CustomerId:   new(order.CustomerID),
		Status:       kitchenapi.OrderStatus(order.Status),
		Items:        orderItemsToAPI(order.Items),
		ItemsTotal:   money(order.ItemsTotal),
		DeliveryFee:  money(order.DeliveryFee),
		Total:        money(order.Total),
		Delivery:     deliveryToAPI(order.Delivery),
		StatusReason: optional(order.StatusReason),
		PrepMinutes:  optional(order.PrepMinutes),
		CreatedAt:    order.CreatedAt,
		UpdatedAt:    order.UpdatedAt,
	}
}

func orderDetailsToAPI(order domain.Order) kitchenapi.OrderDetails {
	timeline := make([]kitchenapi.OrderStatusChange, 0, len(order.Timeline))
	for _, change := range order.Timeline {
		entry := kitchenapi.OrderStatusChange{
			Status:     kitchenapi.OrderStatus(change.ToStatus),
			Actor:      kitchenapi.OrderStatusChangeActor(change.Actor),
			Reason:     optional(change.Reason),
			OccurredAt: change.OccurredAt,
		}
		if change.FromStatus != "" {
			entry.FromStatus = new(kitchenapi.OrderStatus(change.FromStatus))
		}
		timeline = append(timeline, entry)
	}

	base := orderToAPI(order)
	return kitchenapi.OrderDetails{
		Id:           base.Id,
		Number:       base.Number,
		VenueId:      base.VenueId,
		VenueName:    base.VenueName,
		CustomerId:   base.CustomerId,
		Status:       base.Status,
		Items:        base.Items,
		ItemsTotal:   base.ItemsTotal,
		DeliveryFee:  base.DeliveryFee,
		Total:        base.Total,
		Delivery:     base.Delivery,
		StatusReason: base.StatusReason,
		PrepMinutes:  base.PrepMinutes,
		CreatedAt:    base.CreatedAt,
		UpdatedAt:    base.UpdatedAt,
		Timeline:     timeline,
	}
}

func orderPageToAPI(orders []domain.Order, total, limit, offset int) kitchenapi.OrderPage {
	items := make([]kitchenapi.Order, 0, len(orders))
	for _, order := range orders {
		items = append(items, orderToAPI(order))
	}
	return kitchenapi.OrderPage{Items: items, Total: total, Limit: limit, Offset: offset}
}
