-- Envelope postage bought through EasyPost (USPS First-Class letters).
-- Sellers pay from a prepaid postage balance or per envelope on their card.
ALTER TABLE seller_profiles
    ADD COLUMN postage_balance_cents INT NOT NULL DEFAULT 0 CHECK (postage_balance_cents >= 0);

CREATE TABLE postage_credits (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    seller_id      UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind           TEXT NOT NULL CHECK (kind IN ('topup', 'postage', 'refund', 'adjust')),
    amount_cents   INT NOT NULL,               -- + adds to the balance, - spends it
    balance_after  INT NOT NULL,
    order_id       UUID REFERENCES orders (id) ON DELETE SET NULL,
    stripe_object  TEXT,                       -- top-up PaymentIntent
    note           TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX postage_credits_seller_idx ON postage_credits (seller_id, created_at DESC);

-- Envelope postage rates keep the EasyPost shipment they belong to.
ALTER TABLE label_rates ADD COLUMN shipment_id TEXT;
-- How the postage was paid: 'balance', 'card' or 'test' (Test key, nothing charged).
ALTER TABLE shipments ADD COLUMN postage_paid_by TEXT;
