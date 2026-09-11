-- Idempotency guard for webhook processing. Paystack does not supply a
-- canonical per-delivery event ID, so the dedup key is (payment_id,
-- event_type): a given payment can only be legitimately finalized once
-- per event type. The composite PRIMARY KEY is the actual protection —
-- application code inserting here concurrently for the same payment
-- will have exactly one INSERT succeed; the rest fail with a unique
-- violation, atomically, no race window.
CREATE TABLE processed_webhook_events (
    payment_id   UUID NOT NULL,
    event_type   VARCHAR(64) NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (payment_id, event_type)
);
