package http

import (
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/api/gen/openapi/kitchenapi"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/service"
)

var _ kitchenapi.ServerInterface = (*server)(nil)

type server struct {
	catalog     CatalogUseCase
	carts       CartUseCase
	orders      OrderUseCase
	fulfillment FulfillmentUseCase
	partners    PartnerUseCase
}

func NewServer(
	catalog CatalogUseCase,
	carts CartUseCase,
	orders OrderUseCase,
	fulfillment FulfillmentUseCase,
	partners PartnerUseCase,
) kitchenapi.ServerInterface {
	return &server{
		catalog:     catalog,
		carts:       carts,
		orders:      orders,
		fulfillment: fulfillment,
		partners:    partners,
	}
}

func pageFrom(limit, offset *int) service.Page {
	page := service.Page{}
	if limit != nil {
		page.Limit = *limit
	}
	if offset != nil {
		page.Offset = *offset
	}
	return page.Normalize()
}

func statusesFrom(values *[]kitchenapi.OrderStatus) ([]domain.OrderStatus, error) {
	if values == nil || len(*values) == 0 {
		return nil, nil
	}
	statuses := make([]domain.OrderStatus, 0, len(*values))
	for _, value := range *values {
		status := domain.OrderStatus(value)
		if !status.Valid() {
			return nil, domain.Invalid("invalid_status", "unknown order status %q", value)
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func deliveryFrom(request kitchenapi.DeliveryRequest) domain.Delivery {
	delivery := domain.Delivery{
		RecipientName: request.RecipientName,
		Phone:         request.Phone,
		Address:       request.Address,
	}
	if request.Comment != nil {
		delivery.Comment = *request.Comment
	}
	return delivery
}
