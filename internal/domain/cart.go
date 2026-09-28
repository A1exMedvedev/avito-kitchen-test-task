package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type CartStatus string

const (
	CartStatusActive CartStatus = "active"

	CartStatusOrdered CartStatus = "ordered"
)

const MaxCartLineQuantity = 99

type CartLine struct {
	MenuItemID uuid.UUID
	Quantity   int
}

type Cart struct {
	ID         uuid.UUID
	CustomerID uuid.UUID
	VenueID    uuid.UUID
	Status     CartStatus
	Lines      []CartLine
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (c *Cart) SetLine(menuItemID uuid.UUID, quantity int) error {
	if quantity < 0 || quantity > MaxCartLineQuantity {
		return Invalid("cart_quantity_invalid",
			"quantity must be between 0 and %d", MaxCartLineQuantity)
	}

	for i := range c.Lines {
		if c.Lines[i].MenuItemID != menuItemID {
			continue
		}
		if quantity == 0 {
			c.Lines = append(c.Lines[:i], c.Lines[i+1:]...)
			return nil
		}
		c.Lines[i].Quantity = quantity
		return nil
	}

	if quantity == 0 {
		return nil
	}
	c.Lines = append(c.Lines, CartLine{MenuItemID: menuItemID, Quantity: quantity})
	return nil
}

func (c *Cart) EnsureVenue(venueID uuid.UUID) error {
	if len(c.Lines) == 0 || c.VenueID == venueID {
		return nil
	}
	return Conflict("cart_venue_conflict",
		"cart already contains items from another venue; clear it before ordering elsewhere")
}

type PricedLine struct {
	MenuItemID uuid.UUID
	Name       string
	Quantity   int
	UnitPrice  Money
	LineTotal  Money

	Orderable         bool
	Problem           UnavailableReason
	AvailableQuantity int
}

type PricedCart struct {
	Cart  Cart
	Venue *Venue

	Lines          []PricedLine
	ItemsTotal     Money
	DeliveryFee    Money
	Total          Money
	MinOrderAmount Money

	CheckoutReady bool
	Blockers      []string
}

func (p PricedCart) UnavailableItems() []UnavailableItem {
	items := make([]UnavailableItem, 0, len(p.Lines))
	for _, line := range p.Lines {
		if line.Orderable {
			continue
		}
		items = append(items, UnavailableItem{
			MenuItemID: line.MenuItemID,
			Name:       line.Name,
			Requested:  line.Quantity,
			Available:  line.AvailableQuantity,
			Reason:     line.Problem,
		})
	}
	return items
}

func PriceCart(cart Cart, venue *Venue, menuItems map[uuid.UUID]MenuItem) PricedCart {
	priced := PricedCart{Cart: cart, Venue: venue, Lines: make([]PricedLine, 0, len(cart.Lines))}
	if venue != nil {
		priced.DeliveryFee = venue.DeliveryFee
		priced.MinOrderAmount = venue.MinOrderAmount
	}

	for _, line := range cart.Lines {
		item, found := menuItems[line.MenuItemID]
		if !found {
			priced.Lines = append(priced.Lines, PricedLine{
				MenuItemID: line.MenuItemID,
				Name:       "unknown item",
				Quantity:   line.Quantity,
				Problem:    ReasonWithdrawn,
			})
			continue
		}

		reason, ok := item.CheckQuantity(line.Quantity)
		pricedLine := PricedLine{
			MenuItemID:        item.ID,
			Name:              item.Name,
			Quantity:          line.Quantity,
			UnitPrice:         item.Price,
			LineTotal:         item.Price.Times(line.Quantity),
			Orderable:         ok,
			Problem:           reason,
			AvailableQuantity: max(item.StockQuantity, 0),
		}
		if !ok {

			pricedLine.LineTotal = 0
		}
		priced.ItemsTotal = priced.ItemsTotal.Add(pricedLine.LineTotal)
		priced.Lines = append(priced.Lines, pricedLine)
	}

	priced.Total = priced.ItemsTotal.Add(priced.DeliveryFee)
	priced.Blockers = collectBlockers(priced, venue)
	priced.CheckoutReady = len(priced.Blockers) == 0
	return priced
}

func collectBlockers(priced PricedCart, venue *Venue) []string {
	var blockers []string

	if len(priced.Lines) == 0 {
		blockers = append(blockers, "cart is empty")
	}
	if venue == nil {
		return append(blockers, "venue is unknown")
	}
	if err := venue.EnsureAcceptsOrders(); err != nil {
		if domainErr, ok := AsError(err); ok {
			blockers = append(blockers, domainErr.Message)
		}
	}
	for _, line := range priced.Lines {
		if !line.Orderable {
			blockers = append(blockers, fmt.Sprintf("%q: %s", line.Name, line.Problem))
		}
	}
	if len(priced.Lines) > 0 && priced.ItemsTotal < venue.MinOrderAmount {
		blockers = append(blockers, fmt.Sprintf(
			"minimum order amount is %d.%02d %s",
			venue.MinOrderAmount/100, venue.MinOrderAmount%100, Currency))
	}
	return blockers
}
