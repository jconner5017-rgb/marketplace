-- Marketing & discounts (2026-11-20): coupons, order discounts and sale
-- events. At checkout the buyer gets the single best of: the sale price,
-- their coupon, or the seller's best order discount. The seller pays for the
-- discount and our fee is on what the buyer pays.

-- Sale events: $ off as well as % off, and any of the sports & games.
ALTER TABLE store_sales DROP CONSTRAINT IF EXISTS store_sales_percent_check;
ALTER TABLE store_sales ALTER COLUMN percent DROP NOT NULL;
ALTER TABLE store_sales
    ADD COLUMN amount_cents INT,
    ADD COLUMN games TEXT[]; -- listing game_or_sport values; NULL = every card
ALTER TABLE store_sales ADD CONSTRAINT store_sales_kind CHECK (
    (percent BETWEEN 5 AND 50 AND amount_cents IS NULL) OR
    (percent IS NULL AND amount_cents BETWEEN 25 AND 50000));

-- A fixed-price card's sale price right now (its own price when there's no
-- sale). Never more than half off, so a $ off can't give a cheap card away.
CREATE FUNCTION store_sale_price(p_seller UUID, p_category INT, p_game TEXT, p_price INT) RETURNS INT
LANGUAGE sql STABLE AS $$
    SELECT coalesce(min(greatest(
               p_price - CASE WHEN ss.amount_cents IS NOT NULL THEN ss.amount_cents ELSE p_price * ss.percent / 100 END,
               p_price - p_price / 2)), p_price)
    FROM store_sales ss JOIN stores st ON st.seller_id = ss.seller_id AND st.status = 'active'
    WHERE ss.seller_id = p_seller AND ss.cancelled_at IS NULL
      AND now() >= ss.starts_at AND now() < ss.ends_at
      AND (ss.category_id IS NULL OR ss.category_id = p_category)
      AND (ss.games IS NULL OR upper(p_game) = ANY (SELECT upper(g) FROM unnest(ss.games) g))
$$;

-- Coupons a seller gives buyers: a code for % or $ off some or all cards.
CREATE TABLE seller_coupons (
    id             SERIAL PRIMARY KEY,
    seller_id      UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    code           TEXT NOT NULL CHECK (code ~ '^[A-Z0-9][A-Z0-9_-]{2,19}$'),
    label          TEXT NOT NULL DEFAULT '' CHECK (char_length(label) <= 60),
    percent        INT CHECK (percent BETWEEN 5 AND 50),
    amount_cents   INT CHECK (amount_cents BETWEEN 50 AND 50000),
    games          TEXT[], -- NULL = every card
    starts_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    ends_at        TIMESTAMPTZ,
    once_per_buyer BOOLEAN NOT NULL DEFAULT true,
    active         BOOLEAN NOT NULL DEFAULT true,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((percent IS NULL) <> (amount_cents IS NULL)),
    CHECK (ends_at IS NULL OR ends_at > starts_at)
);
CREATE UNIQUE INDEX seller_coupons_code ON seller_coupons (seller_id, code);

-- A coupon used on an order (an order that's never paid doesn't use it up).
CREATE TABLE coupon_redemptions (
    order_id   UUID PRIMARY KEY REFERENCES orders (id) ON DELETE CASCADE,
    coupon_id  INT NOT NULL REFERENCES seller_coupons (id) ON DELETE CASCADE,
    buyer_id   UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    cents      INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX coupon_redemptions_buyer ON coupon_redemptions (coupon_id, buyer_id);

-- A coupon a buyer typed in their cart, per seller, until they check out.
CREATE TABLE cart_coupons (
    user_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    seller_id  UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    code       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, seller_id)
);

-- Order discounts the seller sets up: buy at least N cards (optionally only
-- cards priced in a range) and get % or $ off them at checkout.
CREATE TABLE order_discounts (
    id              SERIAL PRIMARY KEY,
    seller_id       UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    min_cards       INT NOT NULL CHECK (min_cards BETWEEN 2 AND 500),
    price_min_cents INT CHECK (price_min_cents >= 0),
    price_max_cents INT CHECK (price_max_cents > 0),
    percent_bp      INT CHECK (percent_bp BETWEEN 1 AND 5000), -- basis points: 2500 = 25%
    amount_cents    INT CHECK (amount_cents BETWEEN 1 AND 50000),
    per_card        BOOLEAN NOT NULL DEFAULT false, -- amount off each qualifying card after the first
    active          BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((percent_bp IS NULL) <> (amount_cents IS NULL)),
    CHECK (NOT per_card OR amount_cents IS NOT NULL),
    CHECK (price_min_cents IS NULL OR price_max_cents IS NULL OR price_max_cents >= price_min_cents)
);
CREATE INDEX order_discounts_seller ON order_discounts (seller_id) WHERE active;

-- The old multi-card discounts become order discounts.
INSERT INTO order_discounts (seller_id, min_cards, percent_bp)
    SELECT user_id, 3, bundle_discount_val FROM seller_profiles
    WHERE bundle_discount_type = 'percent_3plus' AND bundle_discount_val BETWEEN 1 AND 5000;
INSERT INTO order_discounts (seller_id, min_cards, amount_cents, per_card)
    SELECT user_id, 2, bundle_discount_val, true FROM seller_profiles
    WHERE bundle_discount_type = 'per_extra_item' AND bundle_discount_val BETWEEN 1 AND 50000;

-- Which deal an order got, for the buyer, the seller and support.
ALTER TABLE orders
    ADD COLUMN discount_kind  TEXT CHECK (discount_kind IN ('sale', 'coupon', 'order')),
    ADD COLUMN discount_label TEXT;
