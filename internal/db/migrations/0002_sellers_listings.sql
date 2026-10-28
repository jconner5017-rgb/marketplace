-- A user becomes a seller by connecting a Stripe Standard account.
CREATE TABLE seller_profiles (
    user_id              UUID PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    stripe_account_id    TEXT UNIQUE NOT NULL,
    -- Mirrored from Stripe (account.updated webhook or a status refresh).
    charges_enabled      BOOLEAN NOT NULL DEFAULT false,
    details_submitted    BOOLEAN NOT NULL DEFAULT false,
    -- Reputation tier: 1 = $20 cap, 2 = $250 cap, 3 = $1,000 cap.
    tier                 SMALLINT NOT NULL DEFAULT 1 CHECK (tier BETWEEN 1 AND 3),
    tier_reviewed_at     TIMESTAMPTZ,
    -- Optional multi-card discount: amount off each extra card (cents),
    -- or percent off (basis points) when 3+ cards are bought.
    bundle_discount_type TEXT CHECK (bundle_discount_type IN ('per_extra_item', 'percent_3plus')),
    bundle_discount_val  INT CHECK (bundle_discount_val >= 0),
    stripe_customer_id   TEXT,            -- saved card for label purchases (phase 4)
    fcm_connected        BOOLEAN NOT NULL DEFAULT false,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE listings (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    seller_id       UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    external_id     TEXT,                    -- FireCardManager item ID
    category        TEXT NOT NULL CHECK (category IN ('sports', 'tcg')),
    game_or_sport   TEXT NOT NULL,           -- Pokemon, Magic, Baseball, ...
    set_name        TEXT NOT NULL,
    year            SMALLINT CHECK (year BETWEEN 1860 AND 2100),
    card_number     TEXT,
    title           TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    condition       TEXT,                    -- raw cards: NM, LP, MP, HP, DMG
    grader          TEXT CHECK (grader IN ('PSA', 'BGS', 'SGC', 'CGC', 'TAG', 'OTHER')),
    grade           NUMERIC(3,1) CHECK (grade >= 1 AND grade <= 10),
    cert_number     TEXT,
    attributes      TEXT[] NOT NULL DEFAULT '{}',   -- holo, first_edition, auto, patch, rookie, ...
    photo_urls      TEXT[] NOT NULL DEFAULT '{}',
    price_cents     INT NOT NULL CHECK (price_cents > 0 AND price_cents <= 100000),
    quantity        INT NOT NULL CHECK (quantity >= 0),
    package_preset  TEXT NOT NULL DEFAULT 'envelope'
                    CHECK (package_preset IN ('envelope', 'bubble', 'box')),
    weight_oz       NUMERIC(5,2) NOT NULL DEFAULT 1 CHECK (weight_oz > 0),
    status          TEXT NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'sold_out', 'hidden', 'removed')),
    version         INT NOT NULL DEFAULT 1,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- A graded card needs a grader and a grade; a raw card needs a condition.
    CHECK ((grader IS NULL AND grade IS NULL AND condition IS NOT NULL)
        OR (grader IS NOT NULL AND grade IS NOT NULL)),
    search tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('simple', coalesce(title, '')), 'A') ||
        setweight(to_tsvector('simple', coalesce(game_or_sport, '') || ' ' || coalesce(set_name, '')
                  || ' ' || coalesce(card_number, '') || ' ' || coalesce(grader, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(description, '')), 'C')
    ) STORED
);

CREATE UNIQUE INDEX listings_seller_external_key ON listings (seller_id, external_id)
    WHERE external_id IS NOT NULL;
CREATE INDEX listings_search_idx   ON listings USING GIN (search);
CREATE INDEX listings_browse_idx   ON listings (status, category, created_at DESC);
CREATE INDEX listings_price_idx    ON listings (status, price_cents);
CREATE INDEX listings_seller_idx   ON listings (seller_id, status);
CREATE INDEX listings_game_idx     ON listings (lower(game_or_sport));

-- Snapshot of every listing version, so a chargeback evidence packet can
-- show exactly what the buyer saw at the time of sale.
CREATE TABLE listing_versions (
    listing_id  UUID NOT NULL REFERENCES listings (id) ON DELETE CASCADE,
    version     INT NOT NULL,
    snapshot    JSONB NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (listing_id, version)
);

-- Stripe webhook events already processed (Stripe can deliver an event twice).
CREATE TABLE stripe_events (
    id           TEXT PRIMARY KEY,
    type         TEXT NOT NULL,
    received_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
