-- Transactional outbox: written in the SAME database transaction as
-- the payment status update it accompanies (see
-- repository.MarkProcessedAndUpdate), so the two can never diverge —
-- either both commit or neither does. A separate worker polls
-- published_at IS NULL and publishes to RabbitMQ, marking rows
-- published on success. If the broker is down, rows just accumulate
-- here instead of being lost.
CREATE TABLE outbox_events (
    id            UUID PRIMARY KEY,
    aggregate_id  UUID NOT NULL,
    event_type    VARCHAR(64) NOT NULL,
    routing_key   VARCHAR(64) NOT NULL,
    payload       TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at  TIMESTAMPTZ,
    attempt_count INT NOT NULL DEFAULT 0
);

-- Partial index: only unpublished rows are ever queried by the worker,
-- and that set is always small relative to the full history.
CREATE INDEX idx_outbox_events_unpublished ON outbox_events (created_at) WHERE published_at IS NULL;
