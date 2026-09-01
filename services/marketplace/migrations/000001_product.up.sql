-- Marketplace Service 数据库结构：商品及其图片。
--
-- 商品状态只能由商品操作端点和交易状态机驱动（docs/state-machines.md）；
-- 即使未来某条代码路径遗漏校验，下面的 CHECK 也会阻止未知值进入数据表。
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

-- 公开列表返回 ON_SALE 商品，按 created_at DESC 排序并以 id 作为确定性的平局裁决，
-- 因此索引采用相同的排序方式。
CREATE INDEX products_public_list_idx
    ON products (status, created_at DESC, id DESC);

CREATE INDEX products_seller_list_idx
    ON products (seller_id, created_at DESC, id DESC);

-- 关键词搜索匹配标题子串。子串匹配无法使用 btree 索引；
-- docs/software-design.md 第 8.2 节指出中文搜索索引策略要等需求确认后再决定，
-- 因此这里不创建一个假装有效的索引。
CREATE TABLE product_images (
    id         text PRIMARY KEY,
    product_id text NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    object_key text NOT NULL,
    sort_order integer NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT product_images_sort_order_range CHECK (sort_order BETWEEN 1 AND 3),
    CONSTRAINT product_images_object_key_not_empty CHECK (char_length(object_key) > 0),
    -- 该约束与范围检查一起将商品图片数限制为最多三张，无论应用层如何处理。
    CONSTRAINT product_images_slot_unique UNIQUE (product_id, sort_order)
);

CREATE INDEX product_images_product_idx
    ON product_images (product_id, sort_order);
