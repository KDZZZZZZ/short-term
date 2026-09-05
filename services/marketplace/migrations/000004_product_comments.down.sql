DROP TABLE IF EXISTS product_comments;

-- 恢复 000003 的买家评论结构。
CREATE TABLE product_reviews (
    id         text PRIMARY KEY,
    product_id text NOT NULL REFERENCES products (id),
    trade_id   text NOT NULL REFERENCES trades (id),
    buyer_id   text NOT NULL,
    comment    text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT product_reviews_comment_len CHECK (char_length(comment) BETWEEN 1 AND 500),
    CONSTRAINT product_reviews_buyer_unique UNIQUE (product_id, buyer_id)
);

CREATE INDEX product_reviews_product_list_idx
    ON product_reviews (product_id, created_at DESC, id DESC);
