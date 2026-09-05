-- 买家取消/卖家拒绝后的意向不再占用名额：
-- 进行中（PENDING/ACCEPTED）意向在同一 (product_id, buyer_id) 上唯一，
-- 历史终态记录保留，买家可再次发起新的购买意向（docs/state-machines.md，交易状态）。
DROP INDEX IF EXISTS trades_one_intent_per_buyer_idx;

CREATE UNIQUE INDEX trades_one_active_intent_per_buyer_idx
    ON trades (product_id, buyer_id)
    WHERE status IN ('PENDING', 'ACCEPTED');
