-- +goose Up
-- +goose StatementBegin

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE venues (
                        id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                        slug             TEXT NOT NULL UNIQUE,
                        name             TEXT NOT NULL,
                        description      TEXT NOT NULL DEFAULT '',
                        cuisines         TEXT[] NOT NULL DEFAULT '{}',
                        city             TEXT NOT NULL,
                        address          TEXT NOT NULL,

                        status           TEXT NOT NULL DEFAULT 'active'
                            CHECK (status IN ('active', 'suspended')),
                        is_open          BOOLEAN NOT NULL DEFAULT FALSE,

                        avg_prep_minutes INTEGER NOT NULL DEFAULT 30 CHECK (avg_prep_minutes > 0),
                        min_order_amount BIGINT NOT NULL DEFAULT 0 CHECK (min_order_amount >= 0),
                        delivery_fee     BIGINT NOT NULL DEFAULT 0 CHECK (delivery_fee >= 0),
                        rating           NUMERIC(2, 1) NOT NULL DEFAULT 0
                            CHECK (rating >= 0 AND rating <= 5),

                        created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
                        updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_venues_city_active ON venues (city) WHERE status = 'active';
CREATE INDEX idx_venues_cuisines ON venues USING GIN (cuisines);
CREATE INDEX idx_venues_name_trgm ON venues USING GIN (name gin_trgm_ops);

CREATE TABLE venue_api_keys (
                                id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                                venue_id   UUID NOT NULL REFERENCES venues (id) ON DELETE CASCADE,
                                name       TEXT NOT NULL,
                                key_hash   TEXT NOT NULL UNIQUE,
                                created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
                                revoked_at TIMESTAMPTZ
);

CREATE INDEX idx_venue_api_keys_venue ON venue_api_keys (venue_id);

CREATE TABLE customers (
                           id           UUID PRIMARY KEY,
                           display_name TEXT NOT NULL DEFAULT '',
                           phone        TEXT NOT NULL DEFAULT '',
                           created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
                           updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE menu_categories (
                                 id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                                 venue_id   UUID NOT NULL REFERENCES venues (id) ON DELETE CASCADE,
                                 name       TEXT NOT NULL,
                                 position   INTEGER NOT NULL DEFAULT 0,
                                 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
                                 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

                                 UNIQUE (venue_id, name)
);

CREATE INDEX idx_menu_categories_venue ON menu_categories (venue_id, position);

CREATE TABLE menu_items (
                            id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                            venue_id       UUID NOT NULL REFERENCES venues (id) ON DELETE CASCADE,
                            category_id    UUID NOT NULL REFERENCES menu_categories (id) ON DELETE RESTRICT,
                            name           TEXT NOT NULL,
                            description    TEXT NOT NULL DEFAULT '',
                            price          BIGINT NOT NULL CHECK (price > 0),

                            is_available   BOOLEAN NOT NULL DEFAULT TRUE,
                            stock_quantity INTEGER NOT NULL DEFAULT 0 CHECK (stock_quantity >= 0),

                            weight_grams   INTEGER NOT NULL DEFAULT 0 CHECK (weight_grams >= 0),
                            position       INTEGER NOT NULL DEFAULT 0,

                            created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
                            updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
                            deleted_at     TIMESTAMPTZ
);

CREATE INDEX idx_menu_items_venue ON menu_items (venue_id, position) WHERE deleted_at IS NULL;
CREATE INDEX idx_menu_items_category ON menu_items (category_id) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uq_menu_items_venue_name
    ON menu_items (venue_id, name) WHERE deleted_at IS NULL;

CREATE TABLE carts (
                       id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                       customer_id UUID NOT NULL REFERENCES customers (id) ON DELETE CASCADE,
                       venue_id    UUID NOT NULL REFERENCES venues (id) ON DELETE CASCADE,
                       status      TEXT NOT NULL DEFAULT 'active'
                           CHECK (status IN ('active', 'ordered')),
                       created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
                       updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_carts_active_customer
    ON carts (customer_id) WHERE status = 'active';

CREATE TABLE cart_items (
                            id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
                            cart_id      UUID NOT NULL REFERENCES carts (id) ON DELETE CASCADE,
                            menu_item_id UUID NOT NULL REFERENCES menu_items (id) ON DELETE CASCADE,
                            quantity     INTEGER NOT NULL CHECK (quantity > 0 AND quantity <= 99),
                            created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

                            UNIQUE (cart_id, menu_item_id)
);

CREATE INDEX idx_cart_items_cart ON cart_items (cart_id);

CREATE TABLE orders (
                        id              UUID PRIMARY KEY,
                        number          TEXT NOT NULL UNIQUE,
                        customer_id     UUID NOT NULL REFERENCES customers (id) ON DELETE RESTRICT,
                        venue_id        UUID NOT NULL REFERENCES venues (id) ON DELETE RESTRICT,

                        status          TEXT NOT NULL CHECK (status IN (
                                                                        'created', 'accepted', 'cooking', 'ready',
                                                                        'in_delivery', 'delivered', 'rejected', 'cancelled')),

                        items_total     BIGINT NOT NULL CHECK (items_total >= 0),
                        delivery_fee    BIGINT NOT NULL CHECK (delivery_fee >= 0),
                        total           BIGINT NOT NULL CHECK (total >= 0),

                        recipient_name  TEXT NOT NULL,
                        phone           TEXT NOT NULL,
                        address         TEXT NOT NULL,
                        comment         TEXT NOT NULL DEFAULT '',

                        status_reason   TEXT NOT NULL DEFAULT '',
                        prep_minutes    INTEGER NOT NULL DEFAULT 0,

                        idempotency_key TEXT NOT NULL,
                        version         INTEGER NOT NULL DEFAULT 1,

                        created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
                        updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_orders_idempotency
    ON orders (customer_id, idempotency_key);
CREATE INDEX idx_orders_customer ON orders (customer_id, created_at DESC);
CREATE INDEX idx_orders_venue ON orders (venue_id, created_at DESC);

CREATE INDEX idx_orders_venue_active ON orders (venue_id, created_at)
    WHERE status IN ('created', 'accepted', 'cooking', 'ready', 'in_delivery');

CREATE TABLE order_items (
                             id           UUID PRIMARY KEY,
                             order_id     UUID NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
                             menu_item_id UUID NOT NULL REFERENCES menu_items (id) ON DELETE RESTRICT,
                             name         TEXT NOT NULL,
                             unit_price   BIGINT NOT NULL CHECK (unit_price >= 0),
                             quantity     INTEGER NOT NULL CHECK (quantity > 0),
                             line_total   BIGINT NOT NULL CHECK (line_total >= 0)
);

CREATE INDEX idx_order_items_order ON order_items (order_id);

CREATE TABLE order_status_history (
                                      id          UUID PRIMARY KEY,
                                      order_id    UUID NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
                                      from_status TEXT,
                                      to_status   TEXT NOT NULL,
                                      actor       TEXT NOT NULL CHECK (actor IN ('customer', 'venue', 'system')),
                                      reason      TEXT NOT NULL DEFAULT '',
                                      occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_order_status_history_order
    ON order_status_history (order_id, occurred_at);

CREATE TABLE outbox_events (
                               id             UUID PRIMARY KEY,
                               event_type     TEXT NOT NULL,
                               aggregate_type TEXT NOT NULL DEFAULT 'order',
                               aggregate_id   UUID NOT NULL,
                               venue_id       UUID NOT NULL,
                               payload        JSONB NOT NULL DEFAULT '{}'::jsonb,
                               occurred_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
                               published_at   TIMESTAMPTZ
);

CREATE INDEX idx_outbox_pending ON outbox_events (occurred_at)
    WHERE published_at IS NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS outbox_events;
DROP TABLE IF EXISTS order_status_history;
DROP TABLE IF EXISTS order_items;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS cart_items;
DROP TABLE IF EXISTS carts;
DROP TABLE IF EXISTS menu_items;
DROP TABLE IF EXISTS menu_categories;
DROP TABLE IF EXISTS customers;
DROP TABLE IF EXISTS venue_api_keys;
DROP TABLE IF EXISTS venues;
-- +goose StatementEnd
