-- Shipping labels bought through the platform, paid with the seller's
-- saved card (a Stripe Customer on the platform's own account).
ALTER TABLE seller_profiles
    ADD COLUMN ship_from             JSONB,
    ADD COLUMN label_payment_method  TEXT,
    ADD COLUMN label_card            TEXT;   -- e.g. "Visa ending 4242", for display

ALTER TABLE shipments
    ADD COLUMN provider             TEXT,
    ADD COLUMN provider_label_id    TEXT,
    ADD COLUMN provider_rate_id     TEXT,
    ADD COLUMN label_url            TEXT,
    ADD COLUMN tracking_url         TEXT,
    ADD COLUMN charge_cents         INT,     -- what the seller's card paid (label + processing)
    ADD COLUMN stripe_label_charge  TEXT,    -- PaymentIntent on the platform account
    ADD COLUMN signature            BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN voided_at            TIMESTAMPTZ;
ALTER TABLE shipments DROP CONSTRAINT IF EXISTS shipments_status_check;
ALTER TABLE shipments ADD CONSTRAINT shipments_status_check
    CHECK (status IN ('label_created', 'in_transit', 'delivered', 'exception', 'returned', 'voided'));
CREATE INDEX shipments_tracking_idx ON shipments (tracking_number);

-- Rates shown to a seller, so a purchase uses our recorded price.
CREATE TABLE label_rates (
    rate_id       TEXT PRIMARY KEY,
    order_id      UUID NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    provider      TEXT NOT NULL,
    carrier       TEXT NOT NULL,
    service       TEXT NOT NULL,
    amount_cents  INT NOT NULL CHECK (amount_cents > 0),
    days          INT,
    signature     BOOLEAN NOT NULL DEFAULT false,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX label_rates_order_idx ON label_rates (order_id, created_at DESC);
