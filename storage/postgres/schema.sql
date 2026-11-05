CREATE TABLE IF NOT EXISTS payments (
    id UUID PRIMARY KEY,
    amount BIGINT NOT NULL CHECK (amount > 0),
    currency TEXT NOT NULL,
    method TEXT NOT NULL,
    description TEXT NOT NULL,
    status TEXT NOT NULL,
    provider_id TEXT NOT NULL DEFAULT '',
    confirmation_url TEXT NOT NULL DEFAULT '',
    cancel_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS payments_provider_id_unique
    ON payments (provider_id)
    WHERE provider_id <> '';

CREATE TABLE IF NOT EXISTS payment_requests (
    idempotency_key TEXT PRIMARY KEY,
    request_hash TEXT NOT NULL,
    payment_id UUID NOT NULL REFERENCES payments(id) DEFERRABLE INITIALLY DEFERRED
);
