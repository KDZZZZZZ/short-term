-- Marketplace Service 数据库结构：商品用户评论。
--
-- 买家评论改为开放的用户评论（docs/state-machines.md，用户评论）：评论不再
-- 绑定交易，任何用户可以对任何现存商品发布任意多条评论，因此丢弃 buyer 唯一
-- 约束与 trade 来源，仅保留内容长度约束；即使应用代码有误，CHECK 也会阻止
-- 空评论和超长评论进入数据表。
DROP TABLE IF EXISTS product_reviews;

CREATE TABLE product_comments (
    id         text PRIMARY KEY,
    product_id text NOT NULL REFERENCES products (id),
    user_id    text NOT NULL,
    content    text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT product_comments_content_len CHECK (char_length(content) BETWEEN 1 AND 500)
);

-- 公开评论列表按 created_at DESC 排序并以 id 作为确定性的平局裁决，
-- 因此索引采用相同的排序方式。
CREATE INDEX product_comments_product_list_idx
    ON product_comments (product_id, created_at DESC, id DESC);
