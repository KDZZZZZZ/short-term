-- 恢复终生唯一意向约束。若同一买家对同一商品已存在多条历史意向，此迁移会因
-- 唯一索引失败；生产库中此类行只可能来自验收数据。
DROP INDEX IF EXISTS trades_one_active_intent_per_buyer_idx;

CREATE UNIQUE INDEX trades_one_intent_per_buyer_idx
    ON trades (product_id, buyer_id);
