-- 恢复 000005 的纯文字评价结构。若存在空文字或带评分的行，本迁移会因
-- 内容长度约束失败；生产库中此类行只可能来自验收数据。
ALTER TABLE trade_reviews
    DROP CONSTRAINT trade_reviews_score_range,
    DROP CONSTRAINT trade_reviews_content_max_len,
    DROP COLUMN IF EXISTS score,
    ADD CONSTRAINT trade_reviews_content_len CHECK (char_length(content) BETWEEN 1 AND 500);
