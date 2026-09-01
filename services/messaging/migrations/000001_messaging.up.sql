-- Messaging Service owns product-context conversations, text messages,
-- command idempotency records and its transactional Outbox.

CREATE TABLE conversations (
    id              text PRIMARY KEY,
    product_id      text NOT NULL,
    buyer_id        text NOT NULL,
    seller_id       text NOT NULL,
    created_at      timestamptz NOT NULL,
    last_message_at timestamptz,
    CONSTRAINT conversations_distinct_parties CHECK (buyer_id <> seller_id),
    CONSTRAINT conversations_context_unique UNIQUE (product_id, buyer_id, seller_id)
);

CREATE INDEX conversations_buyer_list_idx
    ON conversations (buyer_id, (COALESCE(last_message_at, created_at)) DESC, id DESC);
CREATE INDEX conversations_seller_list_idx
    ON conversations (seller_id, (COALESCE(last_message_at, created_at)) DESC, id DESC);

CREATE TABLE messages (
    id              text PRIMARY KEY,
    conversation_id text NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    sender_id       text NOT NULL,
    content         text NOT NULL,
    read_at         timestamptz,
    created_at      timestamptz NOT NULL,
    CONSTRAINT messages_content_len CHECK (char_length(content) BETWEEN 1 AND 1000)
);

CREATE INDEX messages_conversation_page_idx
    ON messages (conversation_id, created_at DESC, id DESC);
CREATE INDEX messages_unread_idx
    ON messages (conversation_id, sender_id, created_at, id)
    WHERE read_at IS NULL;

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

CREATE INDEX outbox_events_pending_idx
    ON outbox_events (occurred_at, event_id)
    WHERE published_at IS NULL;
