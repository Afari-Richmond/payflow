CREATE TABLE payments (
    id                  UUID PRIMARY KEY,
    order_id            UUID NOT NULL,
    amount_minor        BIGINT NOT NULL CHECK (amount_minor > 0),
    currency            VARCHAR(3) NOT NULL,
    status              VARCHAR(32) NOT NULL,
    provider_reference  TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- order_id references order-service's orders table, which lives in a
-- separate database (database-per-service) — this is a logical
-- reference only, no FOREIGN KEY constraint is possible across
-- databases.
CREATE INDEX idx_payments_order_id ON payments (order_id);
