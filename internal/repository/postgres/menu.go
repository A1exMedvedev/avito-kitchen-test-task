package postgres

import (
	"context"
	"fmt"

	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/domain"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/repository/postgres/models"
	"backend-trainee-assignment-autumn-2026-a1exmedvedev-fa51bcd1/internal/service"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type menuRepository struct {
	base
}

func NewMenuRepository(pool *pgxpool.Pool) service.MenuRepository {
	return &menuRepository{base{pool: pool}}
}

const menuItemColumns = `
	id, venue_id, category_id, name, description, price, is_available,
	stock_quantity, weight_grams, position, created_at, updated_at, deleted_at`

const menuCategoryColumns = `id, venue_id, name, position, created_at, updated_at`

func (r *menuRepository) ListCategories(ctx context.Context, venueID uuid.UUID) ([]domain.MenuCategory, error) {
	rows, err := r.q(ctx).Query(ctx,
		`SELECT `+menuCategoryColumns+`
		   FROM menu_categories
		  WHERE venue_id = $1
		  ORDER BY position, name`, venueID)
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("list categories: %w", err))
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.MenuCategory])
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("scan categories: %w", err))
	}
	return categoriesToDomain(collected), nil
}

func (r *menuRepository) GetCategory(ctx context.Context, venueID, categoryID uuid.UUID) (*domain.MenuCategory, error) {
	rows, err := r.q(ctx).Query(ctx,
		`SELECT `+menuCategoryColumns+`
		   FROM menu_categories
		  WHERE id = $1 AND venue_id = $2`, categoryID, venueID)
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("get category: %w", err))
	}
	row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[models.MenuCategory])
	if err != nil {
		if isNoRows(err) {
			return nil, domain.NotFound("category_not_found", "menu category not found")
		}
		return nil, domain.Internal(fmt.Errorf("scan category: %w", err))
	}
	category := categoryToDomain(row)
	return &category, nil
}

func (r *menuRepository) SaveCategory(ctx context.Context, category *domain.MenuCategory) error {
	_, err := r.q(ctx).Exec(ctx, `
		INSERT INTO menu_categories (id, venue_id, name, position, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE
		   SET name = EXCLUDED.name,
		       position = EXCLUDED.position,
		       updated_at = EXCLUDED.updated_at`,
		category.ID, category.VenueID, category.Name, category.Position,
		category.CreatedAt, category.UpdatedAt)
	if err != nil {
		if pgErrorCode(err) == sqlStateUniqueViolation {
			return domain.Conflict("category_name_taken",
				"a category named %q already exists", category.Name)
		}
		return domain.Internal(fmt.Errorf("save category: %w", err))
	}
	return nil
}

func (r *menuRepository) DeleteCategory(ctx context.Context, venueID, categoryID uuid.UUID) error {
	tag, err := r.q(ctx).Exec(ctx,
		`DELETE FROM menu_categories WHERE id = $1 AND venue_id = $2`, categoryID, venueID)
	if err != nil {
		if pgErrorCode(err) == sqlStateForeignKeyViolation {
			return domain.Conflict("category_not_empty", "category still holds menu items")
		}
		return domain.Internal(fmt.Errorf("delete category: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return domain.NotFound("category_not_found", "menu category not found")
	}
	return nil
}

func (r *menuRepository) CountCategoryItems(ctx context.Context, venueID, categoryID uuid.UUID) (int, error) {
	var count int
	err := r.q(ctx).QueryRow(ctx, `
		SELECT count(*) FROM menu_items
		 WHERE venue_id = $1 AND category_id = $2 AND deleted_at IS NULL`,
		venueID, categoryID).Scan(&count)
	if err != nil {
		return 0, domain.Internal(fmt.Errorf("count category items: %w", err))
	}
	return count, nil
}

func (r *menuRepository) ListItems(ctx context.Context, venueID uuid.UUID, includeWithdrawn bool) ([]domain.MenuItem, error) {
	query := `SELECT ` + menuItemColumns + `
		   FROM menu_items
		  WHERE venue_id = $1 AND ($2 OR deleted_at IS NULL)
		  ORDER BY position, name`

	rows, err := r.q(ctx).Query(ctx, query, venueID, includeWithdrawn)
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("list menu items: %w", err))
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.MenuItem])
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("scan menu items: %w", err))
	}
	return menuItemsToDomain(collected), nil
}

func (r *menuRepository) GetItem(ctx context.Context, venueID, itemID uuid.UUID) (*domain.MenuItem, error) {
	rows, err := r.q(ctx).Query(ctx,
		`SELECT `+menuItemColumns+` FROM menu_items WHERE id = $1 AND venue_id = $2`,
		itemID, venueID)
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("get menu item: %w", err))
	}
	row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[models.MenuItem])
	if err != nil {
		if isNoRows(err) {
			return nil, domain.NotFound("menu_item_not_found", "menu item not found")
		}
		return nil, domain.Internal(fmt.Errorf("scan menu item: %w", err))
	}
	item := menuItemToDomain(row)
	return &item, nil
}

func (r *menuRepository) SaveItem(ctx context.Context, item *domain.MenuItem) error {
	_, err := r.q(ctx).Exec(ctx, `
		INSERT INTO menu_items (id, venue_id, category_id, name, description, price,
		                        is_available, stock_quantity, weight_grams, position,
		                        created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO UPDATE
		   SET category_id    = EXCLUDED.category_id,
		       name           = EXCLUDED.name,
		       description    = EXCLUDED.description,
		       price          = EXCLUDED.price,
		       is_available   = EXCLUDED.is_available,
		       stock_quantity = EXCLUDED.stock_quantity,
		       weight_grams   = EXCLUDED.weight_grams,
		       position       = EXCLUDED.position,
		       updated_at     = EXCLUDED.updated_at`,
		item.ID, item.VenueID, item.CategoryID, item.Name, item.Description,
		int64(item.Price), item.IsAvailable, item.StockQuantity, item.WeightGrams,
		item.Position, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		if pgErrorCode(err) == sqlStateUniqueViolation {
			return domain.Conflict("item_name_taken",
				"a dish named %q is already on the menu", item.Name)
		}
		return domain.Internal(fmt.Errorf("save menu item: %w", err))
	}
	return nil
}

func (r *menuRepository) DeleteItem(ctx context.Context, venueID, itemID uuid.UUID) error {
	tag, err := r.q(ctx).Exec(ctx, `
		UPDATE menu_items
		   SET deleted_at = now(), is_available = FALSE, updated_at = now()
		 WHERE id = $1 AND venue_id = $2 AND deleted_at IS NULL`, itemID, venueID)
	if err != nil {
		return domain.Internal(fmt.Errorf("delete menu item: %w", err))
	}
	if tag.RowsAffected() == 0 {
		return domain.NotFound("menu_item_not_found", "menu item not found")
	}
	return nil
}

func (r *menuRepository) GetItemsByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.MenuItem, error) {
	return r.itemsByIDs(ctx, ids, false)
}

func (r *menuRepository) LockItemsByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]domain.MenuItem, error) {
	return r.itemsByIDs(ctx, ids, true)
}

func (r *menuRepository) itemsByIDs(ctx context.Context, ids []uuid.UUID, forUpdate bool) (map[uuid.UUID]domain.MenuItem, error) {
	if len(ids) == 0 {
		return map[uuid.UUID]domain.MenuItem{}, nil
	}

	query := `SELECT ` + menuItemColumns + ` FROM menu_items WHERE id = ANY($1) ORDER BY id`
	if forUpdate {
		query += ` FOR UPDATE`
	}

	rows, err := r.q(ctx).Query(ctx, query, ids)
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("load menu items: %w", err))
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.MenuItem])
	if err != nil {
		return nil, domain.Internal(fmt.Errorf("scan menu items: %w", err))
	}

	items := make(map[uuid.UUID]domain.MenuItem, len(collected))
	for _, row := range collected {
		items[row.ID] = menuItemToDomain(row)
	}
	return items, nil
}

func (r *menuRepository) AdjustStock(ctx context.Context, adjustments []service.StockAdjustment) error {
	if len(adjustments) == 0 {
		return nil
	}

	ids := make([]uuid.UUID, 0, len(adjustments))
	deltas := make([]int32, 0, len(adjustments))
	for _, adjustment := range adjustments {
		ids = append(ids, adjustment.MenuItemID)
		deltas = append(deltas, int32(adjustment.Delta)) //nolint:gosec
	}

	_, err := r.q(ctx).Exec(ctx, `
		UPDATE menu_items AS m
		   SET stock_quantity = m.stock_quantity + d.delta,
		       updated_at = now()
		  FROM unnest($1::uuid[], $2::int[]) AS d(id, delta)
		 WHERE m.id = d.id`, ids, deltas)
	if err != nil {
		if pgErrorCode(err) == sqlStateCheckViolation {

			return domain.Conflict("items_unavailable",
				"another order took the last portions while you were checking out")
		}
		return domain.Internal(fmt.Errorf("adjust stock: %w", err))
	}
	return nil
}
