-- Favorite Service owns only user/product relationship facts. User and product
-- identifiers are logical cross-service references, not database foreign keys.
CREATE TABLE favorites (
    user_id      text NOT NULL,
    product_id   text NOT NULL,
    favorited_at timestamptz NOT NULL,
    PRIMARY KEY (user_id, product_id),
    CONSTRAINT favorites_user_id_len CHECK (char_length(user_id) BETWEEN 1 AND 64),
    CONSTRAINT favorites_product_id_len CHECK (char_length(product_id) BETWEEN 1 AND 64)
);

-- Stable page order required by docs/software-design.md section 8.2.
CREATE INDEX favorites_user_page_idx
    ON favorites (user_id, favorited_at DESC, product_id DESC);
