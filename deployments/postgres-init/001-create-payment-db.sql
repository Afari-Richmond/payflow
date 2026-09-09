-- Runs once, only when Postgres initializes a fresh data volume.
-- payflow_order is created automatically via POSTGRES_DB; every
-- additional per-service database goes here (database-per-service).
CREATE DATABASE payflow_payment;
