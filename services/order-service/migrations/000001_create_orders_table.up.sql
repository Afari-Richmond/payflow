CREATE TABLE orders (
    id            UUID PRIMARY KEY,
    email         TEXT NOT NULL,
    amount_minor  BIGINT NOT NULL CHECK (amount_minor > 0),
    currency      VARCHAR(3) NOT NULL,
    status        VARCHAR(32) NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
