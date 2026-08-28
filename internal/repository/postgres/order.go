package postgres

import (
	"context"
	"fmt"
	"strings"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/repository/postgres/models"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/service"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type orderRepository struct {
	base
}

func NewOrderRepository(pool *pgxpool.Pool) service.OrderRepository {
	return &orderRepository{base{pool: pool}}
}

const orderColumns = `
	o.id, o.number, o.customer_id, o.venue_id, v.name AS venue_name, o.status,
	o.items_total, o.delivery_fee, o.total, o.recipient_name, o.phone, o.address,
	o.comment, o.status_reason, o.prep_minutes, o.idempotency_key, o.version,
	o.created_at, o.updated_at`

func (r *orderRepository) Create(ctx context.Context, order *domain.Order) error {
	querier := r.q(ctx)

	_, err := querier.Exec(ctx, `
		INSERT INTO orders (id, number, customer_id, venue_id, status, items_total,
		                    delivery_fee, total, recipient_name, phone, address,
		                    comment, status_reason, prep_minutes, idempotency_key,
		                    version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
		        $16, $17, $18)`,
		order.ID, order.Number, order.CustomerID, order.VenueID, string(order.Status),
		int64(order.ItemsTotal), int64(order.DeliveryFee), int64(order.Total),
		order.Delivery.RecipientName, order.Delivery.Phone, order.Delivery.Address,
		order.Delivery.Comment, order.StatusReason, order.PrepMinutes,
		order.IdempotencyKey, order.Version, order.CreatedAt, order.UpdatedAt)
	if err != nil {
		if pgErrorCode(err) == sqlStateUniqueViolation {

			switch constraint := pgConstraint(err); {
			case strings.Contains(constraint, "number"):
				return domain.Conflict("order_number_conflict", "order number collision")
			case strings.Contains(constraint, "idempotency"):
				return domain.Conflict("order_duplicate",
					"an order with this Idempotency-Key already exists")
			}
		}
		return domain.Internal(fmt.Errorf("insert order: %w", err))
	}

	if err := insertOrderItems(ctx, querier, order); err != nil {
		return err
	}
	for _, change := range order.Timeline {
		if err := insertStatusChange(ctx, querier, change); err != nil {
			return err
		}
	}
	return nil
}

func insertOrderItems(ctx context.Context, querier querier, order *domain.Order) error {
	if len(order.Items) == 0 {
		return nil
	}

	ids := make([]uuid.UUID, 0, len(order.Items))
	menuItemIDs := make([]uuid.UUID, 0, len(order.Items))
	names := make([]string, 0, len(order.Items))
	unitPrices := make([]int64, 0, len(order.Items))
	quantities := make([]int32, 0, len(order.Items))
	lineTotals := make([]int64, 0, len(order.Items))

	for _, item := range order.Items {
		ids = append(ids, item.ID)
		menuItemIDs = append(menuItemIDs, item.MenuItemID)
		names = append(names, item.Name)
		unitPrices = append(unitPrices, int64(item.UnitPrice))
		quantities = append(quantities, int32(item.Quantity)) //nolint:gosec
		lineTotals = append(lineTotals, int64(item.LineTotal))
	}

	_, err := querier.Exec(ctx, `
		INSERT INTO order_items (id, order_id, menu_item_id, name, unit_price,
		                         quantity, line_total)
		SELECT id, $1, menu_item_id, name, unit_price, quantity, line_total
		  FROM unnest($2::uuid[], $3::uuid[], $4::text[], $5::bigint[], $6::int[],
		              $7::bigint[])
		    AS t(id, menu_item_id, name, unit_price, quantity, line_total)`,
		order.ID, ids, menuItemIDs, names, unitPrices, quantities, lineTotals)
	if err != nil {
		return domain.Internal(fmt.Errorf("insert order items: %w", err))
	}
	return nil
}

func insertStatusChange(ctx context.Context, querier querier, change domain.StatusChange) error {
	var fromStatus *string
	if change.FromStatus != "" {
		value := string(change.FromStatus)
		fromStatus = &value
	}

	_, err := querier.Exec(ctx, `
		INSERT INTO order_status_history (id, order_id, from_status, to_status,
		                                  actor, reason, occurred_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		change.ID, change.OrderID, fromStatus, string(change.ToStatus),
		string(change.Actor), change.Reason, change.OccurredAt)
	if err != nil {
		return domain.Internal(fmt.Errorf("insert order status history: %w", err))
	}
	return nil
}

func (r *orderRepository) Get(ctx context.Context, id uuid.UUID) (*domain.Order, error) {
	return r.get(ctx, id, false)
}

func (r *orderRepository) GetForUpdate(ctx context.Context, id uuid.UUID) (*domain.Order, error) {
	return r.get(ctx, id, true)
}

func (r *orderRepository) get(ctx context.Context, id uuid.UUID, forUpdate bool) (*domain.Order, error) {
	query := `SELECT ` + orderColumns + `
		   FROM orders o
		   JOIN venues v ON v.id = o.venue_id
		  WHERE o.id = $1`
	if forUpdate {

		query += ` FOR UPDATE OF o`
	}

	rows, err := r.q(ctx).Query(ctx, query, id)
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("query order: %w", err))
	}
	orderRow, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[models.Order])
	if err != nil {
		if isNoRows(err) {
			return nil, domain.NotFound("order_not_found", "order not found")
		}
		return nil, domain.Internal(fmt.Errorf("scan order: %w", err))
	}

	items, err := r.loadItems(ctx, []uuid.UUID{id})
	if err != nil {
		return nil, err
	}
	history, err := r.loadHistory(ctx, id)
	if err != nil {
		return nil, err
	}

	order := orderToDomain(orderRow, items[id], history)
	return &order, nil
}

func (r *orderRepository) GetByIdempotencyKey(ctx context.Context, customerID uuid.UUID, key string) (*domain.Order, error) {
	var id uuid.UUID
	err := r.q(ctx).QueryRow(ctx,
		`SELECT id FROM orders WHERE customer_id = $1 AND idempotency_key = $2`,
		customerID, key).Scan(&id)
	if err != nil {
		if isNoRows(err) {
			return nil, nil //nolint:nilnil
		}
		return nil, domain.Internal(fmt.Errorf("lookup idempotency key: %w", err))
	}
	return r.get(ctx, id, false)
}

func (r *orderRepository) List(ctx context.Context, filter service.OrderFilter) ([]domain.Order, int, error) {
	conditions := []string{"TRUE"}
	args := []any{}

	if filter.CustomerID != nil {
		args = append(args, *filter.CustomerID)
		conditions = append(conditions, fmt.Sprintf("o.customer_id = $%d", len(args)))
	}
	if filter.VenueID != nil {
		args = append(args, *filter.VenueID)
		conditions = append(conditions, fmt.Sprintf("o.venue_id = $%d", len(args)))
	}
	if len(filter.Statuses) > 0 {
		statuses := make([]string, 0, len(filter.Statuses))
		for _, status := range filter.Statuses {
			statuses = append(statuses, string(status))
		}
		args = append(args, statuses)
		conditions = append(conditions, fmt.Sprintf("o.status = ANY($%d)", len(args)))
	}
	where := "WHERE " + strings.Join(conditions, " AND ")

	var total int
	if err := r.q(ctx).QueryRow(ctx,
		`SELECT count(*) FROM orders o `+where, args...).Scan(&total); err != nil {
		return nil, 0, domain.Internal(fmt.Errorf("count orders: %w", err))
	}

	args = append(args, filter.Page.Limit, filter.Page.Offset)
	listSQL := fmt.Sprintf(`
		SELECT %s
		  FROM orders o
		  JOIN venues v ON v.id = o.venue_id
		  %s
		 ORDER BY o.created_at DESC
		 LIMIT $%d OFFSET $%d`, orderColumns, where, len(args)-1, len(args))

	rows, err := r.q(ctx).Query(ctx, listSQL, args...)
	if err != nil {
		return nil, 0, domain.Internal(fmt.Errorf("list orders: %w", err))
	}
	orderRows, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.Order])
	if err != nil {
		return nil, 0, domain.Internal(fmt.Errorf("scan orders: %w", err))
	}
	if len(orderRows) == 0 {
		return []domain.Order{}, total, nil
	}

	ids := make([]uuid.UUID, 0, len(orderRows))
	for _, row := range orderRows {
		ids = append(ids, row.ID)
	}
	itemsByOrder, err := r.loadItems(ctx, ids)
	if err != nil {
		return nil, 0, err
	}

	orders := make([]domain.Order, 0, len(orderRows))
	for _, row := range orderRows {
		orders = append(orders, orderToDomain(row, itemsByOrder[row.ID], nil))
	}
	return orders, total, nil
}

func (r *orderRepository) SaveStatus(ctx context.Context, order *domain.Order, change domain.StatusChange) error {
	querier := r.q(ctx)

	tag, err := querier.Exec(ctx, `
		UPDATE orders
		   SET status        = $2,
		       status_reason = $3,
		       prep_minutes  = $4,
		       version       = version + 1,
		       updated_at    = $5
		 WHERE id = $1 AND version = $6`,
		order.ID, string(order.Status), order.StatusReason, order.PrepMinutes,
		order.UpdatedAt, order.Version)
	if err != nil {
		return domain.Internal(fmt.Errorf("update order status: %w", err))
	}
	if tag.RowsAffected() == 0 {

		return domain.Conflict("order_modified",
			"the order changed while this request was in flight; retry")
	}
	order.Version++

	return insertStatusChange(ctx, querier, change)
}

func (r *orderRepository) loadItems(ctx context.Context, orderIDs []uuid.UUID) (map[uuid.UUID][]models.OrderItem, error) {
	rows, err := r.q(ctx).Query(ctx, `
		SELECT id, order_id, menu_item_id, name, unit_price, quantity, line_total
		  FROM order_items
		 WHERE order_id = ANY($1)
		 ORDER BY name`, orderIDs)
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("query order items: %w", err))
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.OrderItem])
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("scan order items: %w", err))
	}

	byOrder := make(map[uuid.UUID][]models.OrderItem, len(orderIDs))
	for _, item := range collected {
		byOrder[item.OrderID] = append(byOrder[item.OrderID], item)
	}
	return byOrder, nil
}

func (r *orderRepository) loadHistory(ctx context.Context, orderID uuid.UUID) ([]models.OrderStatusHistory, error) {
	rows, err := r.q(ctx).Query(ctx, `
		SELECT id, order_id, from_status, to_status, actor, reason, occurred_at
		  FROM order_status_history
		 WHERE order_id = $1
		 ORDER BY occurred_at, id`, orderID)
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("query order history: %w", err))
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.OrderStatusHistory])
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("scan order history: %w", err))
	}
	return collected, nil
}
