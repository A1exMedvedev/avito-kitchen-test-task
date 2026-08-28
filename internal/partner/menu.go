package partner

import (
	"context"
	"fmt"
	"log/slog"

	kitchenv1 "backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/api/gen/kitchen/v1"
)

type demoCategory struct {
	Name     string
	Position int32
	Items    []demoItem
}

type demoItem struct {
	Name        string
	Description string
	Price       int64
	Stock       int32
	WeightGrams int32
	Position    int32
}

var demoMenu = []demoCategory{
	{
		Name:     "Хинкали",
		Position: 1,
		Items: []demoItem{
			{Name: "Хинкали с говядиной и свининой", Description: "5 шт., ручная лепка.",
				Price: 45000, Stock: 30, WeightGrams: 500, Position: 1},
			{Name: "Хинкали с бараниной", Description: "5 шт., с зеленью и специями.",
				Price: 52000, Stock: 18, WeightGrams: 520, Position: 2},
			{Name: "Хинкали с сыром сулугуни", Description: "5 шт., вегетарианские.",
				Price: 48000, Stock: 12, WeightGrams: 480, Position: 3},
		},
	},
	{
		Name:     "Хачапури",
		Position: 2,
		Items: []demoItem{
			{Name: "Хачапури по-аджарски", Description: "Лодочка с яйцом и маслом.",
				Price: 59000, Stock: 10, WeightGrams: 420, Position: 1},
			{Name: "Хачапури по-имеретински", Description: "Классическое, с сулугуни.",
				Price: 54000, Stock: 8, WeightGrams: 400, Position: 2},
		},
	},
	{
		Name:     "Супы",
		Position: 3,
		Items: []demoItem{
			{Name: "Харчо", Description: "Говядина, рис, ткемали.",
				Price: 39000, Stock: 14, WeightGrams: 350, Position: 1},
			{Name: "Чихиртма", Description: "Куриный суп с яично-лимонной заправкой.",
				Price: 37000, Stock: 9, WeightGrams: 350, Position: 2},
		},
	},
	{
		Name:     "Напитки",
		Position: 4,
		Items: []demoItem{
			{Name: "Лимонад «Тархун»", Description: "Стеклянная бутылка, 0.5 л.",
				Price: 22000, Stock: 40, WeightGrams: 500, Position: 1},
			{Name: "Чай травяной", Description: "Чайник 0.7 л.",
				Price: 28000, Stock: 25, WeightGrams: 700, Position: 2},
		},
	},
}

func (v *venue) syncMenu(ctx context.Context) error {
	current, err := v.client.partner.GetMenu(ctx, &kitchenv1.GetMenuRequest{})
	if err != nil {
		return fmt.Errorf("read current menu: %w", err)
	}

	categoryIDByName := make(map[string]string, len(current.GetCategories()))
	for _, category := range current.GetCategories() {
		categoryIDByName[category.GetName()] = category.GetId()
	}
	itemIDByName := make(map[string]string, len(current.GetItems()))
	for _, item := range current.GetItems() {
		itemIDByName[item.GetName()] = item.GetId()
	}

	created, updated := 0, 0
	for _, category := range demoMenu {
		request := &kitchenv1.UpsertCategoryRequest{
			Name:     category.Name,
			Position: category.Position,
		}
		request.Id = categoryIDByName[category.Name]

		saved, err := v.client.partner.UpsertCategory(ctx, request)
		if err != nil {
			return fmt.Errorf("upsert category %q: %w", category.Name, err)
		}

		for _, item := range category.Items {
			existingID, exists := itemIDByName[item.Name]
			request := &kitchenv1.UpsertMenuItemRequest{
				CategoryId:  saved.GetId(),
				Name:        item.Name,
				Description: item.Description,
				Price:       &kitchenv1.Money{MinorUnits: item.Price, Currency: "RUB"},
				IsAvailable: true,
				WeightGrams: item.WeightGrams,
				Position:    item.Position,
			}
			if exists {
				request.Id = existingID
			} else {
				request.StockQuantity = item.Stock
			}

			if _, err := v.client.partner.UpsertMenuItem(ctx, request); err != nil {
				return fmt.Errorf("upsert item %q: %w", item.Name, err)
			}
			if exists {
				updated++
			} else {
				created++
			}
		}
	}

	v.logger.Info("menu synchronised",
		slog.Int("created", created),
		slog.Int("updated", updated))
	return nil
}

func (v *venue) Restock(ctx context.Context) error {
	menu, err := v.client.partner.GetMenu(ctx, &kitchenv1.GetMenuRequest{})
	if err != nil {
		return fmt.Errorf("read menu for restock: %w", err)
	}

	target := int32(v.policy().RestockTo) //nolint:gosec
	replenished := 0
	for _, item := range menu.GetItems() {
		if item.GetStockQuantity() >= target {
			continue
		}
		_, err := v.client.partner.SetItemStock(ctx, &kitchenv1.SetItemStockRequest{
			ItemId:        item.GetId(),
			StockQuantity: target,
			IsAvailable:   true,
		})
		if err != nil {
			return fmt.Errorf("restock %q: %w", item.GetName(), err)
		}
		replenished++
	}

	if replenished > 0 {
		v.logger.Info("restocked menu items", slog.Int("count", replenished))
	}
	return nil
}
