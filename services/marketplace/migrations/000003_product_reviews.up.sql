-- Marketplace Service 数据库结构：商品买家评论。
--
-- 评论是交易完成后的不可变记录，没有更新或删除路径。即使应用代码有误，
-- 下面的 CHECK 和唯一约束也会阻止空评论、超长评论以及同一买家对同一商品的
-- 第二条评论进入数据表（docs/state-machines.md，买家评论）。
CREATE TABLE product_reviews (
    id         text PRIMARY KEY,
    product_id text NOT NULL REFERENCES products (id),
    trade_id   text NOT NULL REFERENCES trades (id),
    buyer_id   text NOT NULL,
    comment    text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT product_reviews_comment_len CHECK (char_length(comment) BETWEEN 1 AND 500),
    -- 交易保证 (product_id, buyer_id) 终生最多一笔，评论沿用同一联合唯一，
    -- 并发重复提交时恰好一个事务能够插入成功。
    CONSTRAINT product_reviews_buyer_unique UNIQUE (product_id, buyer_id)
);

-- 公开评论列表按 created_at DESC 排序并以 id 作为确定性的平局裁决，
-- 因此索引采用相同的排序方式。
CREATE INDEX product_reviews_product_list_idx
    ON product_reviews (product_id, created_at DESC, id DESC);
