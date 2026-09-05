-- Marketplace Service 数据库结构：买家评价。
--
-- 买家评价是交易完成后买家发布的不可变记录，与商品用户评论相互独立。
-- 即使应用代码有误，下面的 CHECK 和唯一约束也会阻止空评价、超长评价以及
-- 同一笔交易的第二条评价进入数据表（docs/state-machines.md，买家评价）。
CREATE TABLE trade_reviews (
    id         text PRIMARY KEY,
    trade_id   text NOT NULL REFERENCES trades (id),
    product_id text NOT NULL REFERENCES products (id),
    buyer_id   text NOT NULL,
    content    text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT trade_reviews_content_len CHECK (char_length(content) BETWEEN 1 AND 500),
    -- 每笔交易最多一条买家评价；交易本身终生唯一，因此每个商品也最多一条。
    CONSTRAINT trade_reviews_trade_unique UNIQUE (trade_id)
);

-- 我的商品页按商品批量读取评价；已售出商品详情按商品读取单条评价。
CREATE INDEX trade_reviews_product_idx
    ON trade_reviews (product_id);
