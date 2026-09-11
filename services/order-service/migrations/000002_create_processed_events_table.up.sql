-- Idempotency guard for RabbitMQ consumption. Unlike payment-service's
-- webhook dedup, RabbitMQ's envelope carries a genuinely unique
-- event_id per message (assigned once at publish time and unchanged
-- across redeliveries), so it's used directly as the PRIMARY KEY —
-- the real protection against a redelivered message being processed
-- twice, atomically, no race window.
CREATE TABLE processed_events (
    event_id     UUID PRIMARY KEY,
    event_type   VARCHAR(64) NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
