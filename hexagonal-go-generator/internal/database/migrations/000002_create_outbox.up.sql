-- Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
-- Author: Awase Khirni Syed
--
-- 000002_create_outbox.up.sql - transactional outbox table for reliable
-- domain-event delivery. The relay worker (internal/infrastructure/outbox)
-- polls this table and dispatches pending rows to the enterprise bus.

CREATE TABLE IF NOT EXISTS outbox (
    id           BIGSERIAL    PRIMARY KEY,
    event_id     UUID         NOT NULL UNIQUE,
    topic        TEXT         NOT NULL,
    payload      JSONB        NOT NULL,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    processed_at TIMESTAMPTZ
);

-- Index for the relay's "WHERE processed_at IS NULL ORDER BY id" query.
CREATE INDEX IF NOT EXISTS idx_outbox_unprocessed
    ON outbox(id)
    WHERE processed_at IS NULL;
