-- Marketplace Service schema: products and their images.
--
-- Product status is driven only by the product action endpoints and by the
-- trade state machine (docs/state-machines.md); the CHECK below keeps an
-- unknown value out of the table even if a future code path forgets.
CREATE TABLE products (
    id           text PRIMARY KEY,
    seller_id    text NOT NULL,
    title        text NOT NULL,
    price_minor  bigint NOT NULL,
    category     text NOT NULL,
    description  text NOT NULL,
    status       text NOT NULL,
    version      bigint NOT NULL DEFAULT 1,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT products_title_len CHECK (char_length(title) BETWEEN 1 AND 100),
    CONSTRAINT products_description_len CHECK (char_length(description) BETWEEN 1 AND 2000),
    CONSTRAINT products_price_range CHECK (price_minor BETWEEN 0 AND 9999999999),
    CONSTRAINT products_category_enum CHECK (category IN ('TEXTBOOK', 'DIGITAL', 'LIFE', 'OTHER')),
    CONSTRAINT products_status_enum CHECK (status IN ('ON_SALE', 'RESERVED', 'SOLD', 'OFF_SHELF')),
    CONSTRAINT products_version_positive CHECK (version >= 1)
);

-- The public list returns ON_SALE products ordered by created_at DESC with id
-- as the deterministic tie-breaker, so the index carries the same order.
CREATE INDEX products_public_list_idx
    ON products (status, created_at DESC, id DESC);

CREATE INDEX products_seller_list_idx
    ON products (seller_id, created_at DESC, id DESC);

-- Keyword search matches a substring of the title. A substring match cannot
-- use a btree index; docs/software-design.md section 8.2 leaves the Chinese
-- search indexing strategy open until the requirement is confirmed, so no
-- pretend index is created here.
CREATE TABLE product_images (
    id         text PRIMARY KEY,
    product_id text NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    object_key text NOT NULL,
    sort_order integer NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT product_images_sort_order_range CHECK (sort_order BETWEEN 1 AND 3),
    CONSTRAINT product_images_object_key_not_empty CHECK (char_length(object_key) > 0),
    -- Together with the range check this is what caps a product at three
    -- images, whatever the application layer does.
    CONSTRAINT product_images_slot_unique UNIQUE (product_id, sort_order)
);

CREATE INDEX product_images_product_idx
    ON product_images (product_id, sort_order);
