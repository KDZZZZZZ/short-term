-- Marketplace Service 数据库结构：交易、幂等账本和 Outbox。
--
-- 这里强制执行 docs/state-machines.md 要求的规则，即使应用代码有误也必须成立：
-- 一个商品最多有一笔 ACCEPTED 交易，同一商品和买家终生只有一个购买意向，
-- 一个命令键恰好对应一个已提交结果。

CREATE TABLE trades (
    id                   text PRIMARY KEY,
    product_id           text NOT NULL REFERENCES products (id),
    buyer_id             text NOT NULL,
    seller_id            text NOT NULL,
    conversation_id      text,
    price_snapshot_minor bigint NOT NULL,
    status               text NOT NULL,
    buyer_confirmed_at   timestamptz,
    seller_confirmed_at  timestamptz,
    cancel_reason        text,
    created_at           timestamptz NOT NULL DEFAULT now(),
    accepted_at          timestamptz,
    completed_at         timestamptz,
    cancelled_at         timestamptz,
    updated_at           timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT trades_status_enum CHECK (status IN ('PENDING', 'ACCEPTED', 'COMPLETED', 'CANCELLED')),
    CONSTRAINT trades_buyer_is_not_seller CHECK (buyer_id <> seller_id),
    CONSTRAINT trades_price_range CHECK (price_snapshot_minor BETWEEN 0 AND 9999999999),
    CONSTRAINT trades_cancel_reason_len CHECK (cancel_reason IS NULL OR char_length(cancel_reason) BETWEEN 1 AND 200),
    -- 已完成的交易必须同时具有双方确认时间和完成时间。
    CONSTRAINT trades_completed_is_confirmed CHECK (
        status <> 'COMPLETED'
        OR (buyer_confirmed_at IS NOT NULL AND seller_confirmed_at IS NOT NULL AND completed_at IS NOT NULL)
    ),
    CONSTRAINT trades_accepted_has_timestamp CHECK (status <> 'ACCEPTED' OR accepted_at IS NOT NULL),
    CONSTRAINT trades_cancelled_has_timestamp CHECK (status <> 'CANCELLED' OR cancelled_at IS NOT NULL)
);

-- 每个商品最多有一笔已接受交易。这是“同一商品最多存在一个 ACCEPTED 交易”的数据库级保证：
-- 即使两个事务因某种原因都走到写入处，并发接受操作也不可能同时提交。
CREATE UNIQUE INDEX trades_one_accepted_per_product_idx
    ON trades (product_id)
    WHERE status = 'ACCEPTED';

-- Trade 是终生唯一的购买意向，而不是一次请求。即使意向已经 CANCELLED 或 COMPLETED，
-- 同一买家也不能针对同一商品创建第二条记录。
CREATE UNIQUE INDEX trades_one_intent_per_buyer_idx
    ON trades (product_id, buyer_id);

CREATE INDEX trades_buyer_list_idx  ON trades (buyer_id, created_at DESC, id DESC);
CREATE INDEX trades_seller_list_idx ON trades (seller_id, created_at DESC, id DESC);
CREATE INDEX trades_product_idx     ON trades (product_id, status);

-- 幂等账本。
--
-- 一行记录只对应一个已提交的命令。规范化结果是序列化的命令结果及其 schema 版本，
-- 从来不是 HTTP 正文：Gateway 根据 result_code 和此快照确定性地重建第一次响应
--（docs/software-design.md 第 5.3 节）。
CREATE TABLE idempotency_records (
    actor_id        text NOT NULL,
    operation       text NOT NULL,
    idempotency_key text NOT NULL,
    result_code     text NOT NULL,
    schema_version  integer NOT NULL,
    result          bytea NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT idempotency_records_pkey PRIMARY KEY (actor_id, operation, idempotency_key),
    CONSTRAINT idempotency_records_key_len CHECK (char_length(idempotency_key) BETWEEN 16 AND 128)
);

-- 事务型 Outbox。
--
-- 事件与领域变更写入同一事务，并在提交后发布，这正是消除数据库/消息代理双写的方式。
-- 交付语义为至少一次，因此消费者按 event_id 去重。
CREATE TABLE outbox_events (
    event_id       text PRIMARY KEY,
    event_type     text NOT NULL,
    schema_version integer NOT NULL,
    aggregate_type text NOT NULL,
    aggregate_id   text NOT NULL,
    occurred_at    timestamptz NOT NULL,
    trace_id       text NOT NULL DEFAULT '',
    payload        bytea NOT NULL,
    published_at   timestamptz,
    attempts       integer NOT NULL DEFAULT 0,
    last_error     text,
    CONSTRAINT outbox_events_type_not_empty CHECK (char_length(event_type) > 0)
);

-- 发布器按时间从早到晚扫描尚未发布的事件。
CREATE INDEX outbox_events_pending_idx
    ON outbox_events (occurred_at, event_id)
    WHERE published_at IS NULL;
