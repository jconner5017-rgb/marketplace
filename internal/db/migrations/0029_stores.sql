-- Business stores: a branded storefront for a business seller at
-- /store/{slug}, with the seller's own categories, store-wide sales, and
-- followers who hear about new listings. Free; staff check the business
-- details before a store goes live.
CREATE TABLE stores (
    seller_id         UUID PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    slug              TEXT NOT NULL,
    name              TEXT NOT NULL CHECK (char_length(name) BETWEEN 2 AND 60),
    tagline           TEXT NOT NULL DEFAULT '' CHECK (char_length(tagline) <= 120),
    about             TEXT NOT NULL DEFAULT '' CHECK (char_length(about) <= 4000),
    policies          TEXT NOT NULL DEFAULT '' CHECK (char_length(policies) <= 4000),
    logo_url          TEXT,
    banner_url        TEXT,
    -- Business details, checked by staff, never shown publicly.
    legal_name        TEXT NOT NULL CHECK (char_length(legal_name) BETWEEN 2 AND 120),
    business_type     TEXT NOT NULL CHECK (business_type IN ('llc', 'corporation', 'partnership', 'sole_proprietor', 'other')),
    website           TEXT NOT NULL DEFAULT '',
    business_note     TEXT NOT NULL DEFAULT '' CHECK (char_length(business_note) <= 1000),
    status            TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'active', 'rejected', 'suspended')),
    review_note       TEXT NOT NULL DEFAULT '',
    reviewed_by       UUID REFERENCES users (id) ON DELETE SET NULL,
    reviewed_at       TIMESTAMPTZ,
    last_notified_at  TIMESTAMPTZ NOT NULL DEFAULT now(), -- followers told about listings up to here
    last_update_at    TIMESTAMPTZ,                        -- last update sent to followers
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX stores_slug ON stores (lower(slug));
CREATE INDEX stores_status ON stores (status, created_at);

-- The seller's own categories ("Vintage baseball", "Graded Pokemon").
CREATE TABLE store_categories (
    id         SERIAL PRIMARY KEY,
    seller_id  UUID NOT NULL REFERENCES stores (seller_id) ON DELETE CASCADE,
    name       TEXT NOT NULL CHECK (char_length(name) BETWEEN 1 AND 40),
    position   INT NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX store_categories_name ON store_categories (seller_id, lower(name));

ALTER TABLE listings ADD COLUMN store_category_id INT REFERENCES store_categories (id) ON DELETE SET NULL;
CREATE INDEX listings_store_category ON listings (store_category_id) WHERE store_category_id IS NOT NULL;

-- Store-wide (or one-category) sales: a percent off fixed-price listings
-- for a set time. Buyers pay the sale price; it's what fees are based on.
CREATE TABLE store_sales (
    id           SERIAL PRIMARY KEY,
    seller_id    UUID NOT NULL REFERENCES stores (seller_id) ON DELETE CASCADE,
    name         TEXT NOT NULL CHECK (char_length(name) BETWEEN 2 AND 60),
    percent      INT NOT NULL CHECK (percent BETWEEN 5 AND 50),
    category_id  INT REFERENCES store_categories (id) ON DELETE CASCADE,
    starts_at    TIMESTAMPTZ NOT NULL,
    ends_at      TIMESTAMPTZ NOT NULL,
    cancelled_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (ends_at > starts_at AND ends_at <= starts_at + interval '31 days')
);
CREATE INDEX store_sales_seller ON store_sales (seller_id, ends_at);

-- The percent off a listing gets right now from its store's sale (0 when none).
CREATE FUNCTION store_sale_percent(p_seller UUID, p_category INT) RETURNS INT
LANGUAGE sql STABLE AS $$
    SELECT coalesce(max(ss.percent), 0)
    FROM store_sales ss JOIN stores st ON st.seller_id = ss.seller_id AND st.status = 'active'
    WHERE ss.seller_id = p_seller AND ss.cancelled_at IS NULL
      AND now() >= ss.starts_at AND now() < ss.ends_at
      AND (ss.category_id IS NULL OR ss.category_id = p_category)
$$;

CREATE TABLE store_followers (
    seller_id   UUID NOT NULL REFERENCES stores (seller_id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (seller_id, user_id)
);
CREATE INDEX store_followers_user ON store_followers (user_id);

-- Updates a store sent to its followers (kept for staff review).
CREATE TABLE store_updates (
    id          SERIAL PRIMARY KEY,
    seller_id   UUID NOT NULL REFERENCES stores (seller_id) ON DELETE CASCADE,
    subject     TEXT NOT NULL,
    body        TEXT NOT NULL,
    recipients  INT NOT NULL DEFAULT 0,
    emailed     INT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
