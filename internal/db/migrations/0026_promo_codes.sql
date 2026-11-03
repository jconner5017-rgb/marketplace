-- Promo codes that give a seller a number of sales with no marketplace fee.
-- Staff create and edit them in the back office: a public code for new
-- individual sellers, or a single-use code with more sales for one business.
CREATE TABLE promo_codes (
    id               SERIAL PRIMARY KEY,
    code             TEXT NOT NULL,                  -- stored upper case
    label            TEXT NOT NULL DEFAULT '',        -- who it's for, shown only to staff
    audience         TEXT NOT NULL DEFAULT 'any' CHECK (audience IN ('any', 'individual', 'business')),
    fee_free_sales   INT NOT NULL CHECK (fee_free_sales BETWEEN 1 AND 100000),
    max_redemptions  INT CHECK (max_redemptions IS NULL OR max_redemptions >= 1),
    new_sellers_only BOOLEAN NOT NULL DEFAULT true,   -- only sellers with no paid sales yet
    expires_at       TIMESTAMPTZ,                     -- last moment the code can be redeemed
    valid_days       INT CHECK (valid_days IS NULL OR valid_days BETWEEN 1 AND 3650), -- how long redeemed sales last
    active           BOOLEAN NOT NULL DEFAULT true,
    created_by       UUID REFERENCES users (id),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX promo_codes_code ON promo_codes (upper(code));

CREATE TABLE promo_redemptions (
    id           SERIAL PRIMARY KEY,
    code_id      INT NOT NULL REFERENCES promo_codes (id),
    seller_id    UUID NOT NULL REFERENCES users (id),
    sales_granted INT NOT NULL CHECK (sales_granted >= 1),
    sales_used   INT NOT NULL DEFAULT 0 CHECK (sales_used >= 0 AND sales_used <= sales_granted),
    expires_at   TIMESTAMPTZ,
    redeemed_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (code_id, seller_id)
);
CREATE INDEX promo_redemptions_seller ON promo_redemptions (seller_id);

-- Which redemption paid for an order's waived fees, so an unpaid checkout
-- that's cancelled gives the sales back.
CREATE TABLE order_promo_uses (
    order_id      UUID NOT NULL REFERENCES orders (id),
    redemption_id INT NOT NULL REFERENCES promo_redemptions (id),
    cards         INT NOT NULL CHECK (cards >= 1),
    returned      BOOLEAN NOT NULL DEFAULT false,
    PRIMARY KEY (order_id, redemption_id)
);

ALTER TABLE orders ADD COLUMN fee_waived_cents INT NOT NULL DEFAULT 0;
