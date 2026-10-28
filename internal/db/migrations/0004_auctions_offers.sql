-- Auctions (5, 7 or 10 days, proxy bidding) and Best Offer on fixed-price
-- listings. An auction's current price is kept in price_cents so search,
-- sorting and price filters treat both formats the same way.

ALTER TABLE listings DROP CONSTRAINT IF EXISTS listings_status_check;
ALTER TABLE listings ADD CONSTRAINT listings_status_check
    CHECK (status IN ('active', 'sold_out', 'hidden', 'removed', 'ended'));

ALTER TABLE listings
    ADD COLUMN format           TEXT NOT NULL DEFAULT 'fixed' CHECK (format IN ('fixed', 'auction')),
    -- Auction settings
    ADD COLUMN start_cents      INT CHECK (start_cents > 0 AND start_cents <= 100000),
    ADD COLUMN reserve_cents    INT CHECK (reserve_cents > 0 AND reserve_cents <= 100000),
    ADD COLUMN bin_cents        INT CHECK (bin_cents > 0 AND bin_cents <= 100000),
    ADD COLUMN duration_days    SMALLINT CHECK (duration_days IN (5, 7, 10)),
    ADD COLUMN auto_relist      BOOLEAN NOT NULL DEFAULT false,
    -- Auction state
    ADD COLUMN auction_round    SMALLINT NOT NULL DEFAULT 0,   -- 1 after an automatic relist
    ADD COLUMN starts_at        TIMESTAMPTZ,
    ADD COLUMN ends_at          TIMESTAMPTZ,
    ADD COLUMN bid_count        INT NOT NULL DEFAULT 0,
    ADD COLUMN leader_id        UUID REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN leader_max_cents INT,                            -- never shown to anyone
    ADD COLUMN auction_state    TEXT CHECK (auction_state IN ('live', 'won', 'unsold', 'ended_early')),
    ADD COLUMN winner_id        UUID REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN ended_at         TIMESTAMPTZ,
    ADD COLUMN ending_notice    BOOLEAN NOT NULL DEFAULT false, -- watchers told it ends within the hour
    -- Best Offer (fixed price only). *_kind: amount = cents, percent = whole percent of the price.
    ADD COLUMN offers_enabled      BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN offer_accept_kind   TEXT CHECK (offer_accept_kind IN ('amount', 'percent')),
    ADD COLUMN offer_accept_val    INT CHECK (offer_accept_val > 0),
    ADD COLUMN offer_decline_kind  TEXT CHECK (offer_decline_kind IN ('amount', 'percent')),
    ADD COLUMN offer_decline_val   INT CHECK (offer_decline_val > 0),
    ADD CONSTRAINT listings_auction_fields CHECK (
        format = 'fixed' OR (start_cents IS NOT NULL AND duration_days IS NOT NULL AND ends_at IS NOT NULL)),
    ADD CONSTRAINT listings_offers_fixed CHECK (format = 'fixed' OR NOT offers_enabled);

CREATE INDEX listings_ending_idx ON listings (ends_at) WHERE auction_state = 'live';

-- Every bid. A bidder's maximum is private; bid history shows shown_cents,
-- the price the bid produced.
CREATE TABLE bids (
    id           BIGSERIAL PRIMARY KEY,
    listing_id   UUID NOT NULL REFERENCES listings (id) ON DELETE CASCADE,
    round        SMALLINT NOT NULL DEFAULT 0,
    bidder_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    max_cents    INT NOT NULL CHECK (max_cents > 0),
    shown_cents  INT NOT NULL CHECK (shown_cents > 0),
    ip           TEXT,
    user_agent   TEXT,
    removed_at   TIMESTAMPTZ,                 -- removed by staff
    removed_by   UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX bids_listing_idx ON bids (listing_id, round, id);
CREATE INDEX bids_bidder_idx ON bids (bidder_id, created_at DESC);

-- Offers and counteroffers. amount_cents is per card.
CREATE TABLE offers (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    listing_id    UUID NOT NULL REFERENCES listings (id) ON DELETE CASCADE,
    buyer_id      UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    seller_id     UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    parent_id     UUID REFERENCES offers (id) ON DELETE SET NULL,  -- the offer this counters
    from_role     TEXT NOT NULL CHECK (from_role IN ('buyer', 'seller')),
    amount_cents  INT NOT NULL CHECK (amount_cents > 0 AND amount_cents <= 100000),
    quantity      INT NOT NULL DEFAULT 1 CHECK (quantity > 0),
    message       TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'pending'
                  CHECK (status IN ('pending', 'accepted', 'declined', 'countered', 'expired')),
    decided_by    TEXT CHECK (decided_by IN ('auto', 'person', 'system')),
    expires_at    TIMESTAMPTZ NOT NULL,
    responded_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX offers_listing_idx ON offers (listing_id, status);
CREATE INDEX offers_buyer_idx ON offers (buyer_id, created_at DESC);
CREATE INDEX offers_seller_idx ON offers (seller_id, created_at DESC);
CREATE INDEX offers_pending_idx ON offers (expires_at) WHERE status = 'pending';

-- A card set aside for one buyer (auction win or accepted offer) until paid.
CREATE TABLE holds (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    listing_id   UUID NOT NULL REFERENCES listings (id) ON DELETE CASCADE,
    buyer_id     UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    seller_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    source       TEXT NOT NULL CHECK (source IN ('auction', 'offer')),
    offer_id     UUID REFERENCES offers (id) ON DELETE SET NULL,
    quantity     INT NOT NULL CHECK (quantity > 0),
    unit_cents   INT NOT NULL CHECK (unit_cents > 0),
    status       TEXT NOT NULL DEFAULT 'awaiting_payment'
                 CHECK (status IN ('awaiting_payment', 'paid', 'expired', 'cancelled')),
    due_at       TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX holds_buyer_idx ON holds (buyer_id, status);
CREATE INDEX holds_seller_idx ON holds (seller_id, status);
CREATE INDEX holds_due_idx ON holds (due_at) WHERE status = 'awaiting_payment';
