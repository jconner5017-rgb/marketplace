-- FireCardManager connection: seller API keys, idempotent writes, signed
-- outbound webhooks with retries, nightly inventory reconciliation, and
-- cross-listing oversells.

CREATE TABLE api_keys (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name          TEXT NOT NULL,
    prefix        TEXT NOT NULL,            -- first characters, to tell keys apart
    key_hash      TEXT NOT NULL UNIQUE,     -- SHA-256 of the key; the key itself is shown once
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at  TIMESTAMPTZ,
    revoked_at    TIMESTAMPTZ
);
CREATE INDEX api_keys_user_idx ON api_keys (user_id);

-- A write sent twice with the same Idempotency-Key returns the first answer.
CREATE TABLE idempotency_keys (
    user_id       UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    key           TEXT NOT NULL,
    method        TEXT NOT NULL,
    path          TEXT NOT NULL,
    request_hash  TEXT NOT NULL,
    status        INT NOT NULL,
    response      BYTEA NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, key)
);

ALTER TABLE seller_profiles
    ADD COLUMN webhook_url     TEXT,
    ADD COLUMN webhook_secret  TEXT,
    ADD COLUMN inventory_url   TEXT,
    ADD COLUMN reconciled_at   TIMESTAMPTZ;

CREATE TABLE webhook_deliveries (
    id               BIGSERIAL PRIMARY KEY,
    user_id          UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    event            TEXT NOT NULL,
    event_id         TEXT NOT NULL UNIQUE,
    payload          JSONB NOT NULL,
    attempts         INT NOT NULL DEFAULT 0,
    next_attempt_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    delivered_at     TIMESTAMPTZ,
    last_status      INT,
    last_error       TEXT,
    alerted          BOOLEAN NOT NULL DEFAULT false,
    gave_up_at       TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX webhook_due_idx ON webhook_deliveries (next_attempt_at) WHERE delivered_at IS NULL AND gave_up_at IS NULL;
CREATE INDEX webhook_user_idx ON webhook_deliveries (user_id, created_at DESC);

-- A card that sold here and somewhere else at nearly the same time.
CREATE TABLE oversells (
    id                 BIGSERIAL PRIMARY KEY,
    order_id           UUID NOT NULL REFERENCES orders (id),
    seller_id          UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    listing_id         UUID REFERENCES listings (id) ON DELETE SET NULL,
    outcome            TEXT NOT NULL CHECK (outcome IN ('refunded', 'kept')),
    sold_elsewhere_at  TIMESTAMPTZ NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (order_id, listing_id)
);
CREATE INDEX oversells_seller_idx ON oversells (seller_id, created_at);
