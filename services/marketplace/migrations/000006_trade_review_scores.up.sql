-- Marketplace Service 数据库结构：买家评价升级为打分制。
--
-- 评分必填（1–5 整数），文字改为可选：空字符串表示买家未填写文字。
-- 可空评分列使 000005 中已存在的纯文字评价仍然可读；平均值聚合忽略
-- 没有评分的历史行。
ALTER TABLE trade_reviews
    ADD COLUMN score smallint,
    DROP CONSTRAINT trade_reviews_content_len,
    ADD CONSTRAINT trade_reviews_content_max_len CHECK (char_length(content) BETWEEN 0 AND 500),
    ADD CONSTRAINT trade_reviews_score_range CHECK (score IS NULL OR score BETWEEN 1 AND 5);
