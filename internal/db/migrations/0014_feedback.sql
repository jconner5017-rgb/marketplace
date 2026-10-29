-- Feedback model: one-way negatives, blind reveal, sub-ratings, held
-- negatives, new-seller grace, extortion scrubbing, short public replies,
-- automatic seller feedback, nudges and verified outside feedback.

-- Sellers can't leave buyers negatives (stops retaliation).
UPDATE feedback SET rating = 0 WHERE role = 'seller_to_buyer' AND rating < 0;
ALTER TABLE feedback ADD CONSTRAINT feedback_seller_no_negative CHECK (role <> 'seller_to_buyer' OR rating >= 0);

ALTER TABLE feedback
    -- Buyer's 1–5 sub-ratings of the seller.
    ADD COLUMN item_rating           SMALLINT CHECK (item_rating BETWEEN 1 AND 5),
    ADD COLUMN shipping_rating       SMALLINT CHECK (shipping_rating BETWEEN 1 AND 5),
    ADD COLUMN communication_rating  SMALLINT CHECK (communication_rating BETWEEN 1 AND 5),
    -- Hidden from the other party (and the public) until both have left
    -- feedback or the reveal window closes.
    ADD COLUMN visible_at            TIMESTAMPTZ,
    -- published | held (a negative's 3-day cooling-off) | mediation (a new
    -- seller's first negative, waiting on staff)
    ADD COLUMN state                 TEXT NOT NULL DEFAULT 'published' CHECK (state IN ('published', 'held', 'mediation')),
    ADD COLUMN held_until            TIMESTAMPTZ,
    ADD COLUMN reply_at              TIMESTAMPTZ,
    ADD COLUMN auto                  BOOLEAN NOT NULL DEFAULT false,
    -- Comments that tried to trade feedback for money are scrubbed; the
    -- original stays here for staff.
    ADD COLUMN scrubbed_comment      TEXT,
    ADD COLUMN updated_at            TIMESTAMPTZ NOT NULL DEFAULT now();
-- Existing feedback stays visible.
UPDATE feedback SET visible_at = created_at WHERE visible_at IS NULL;
CREATE INDEX feedback_received_idx ON feedback (to_user, role, created_at DESC);

ALTER TABLE seller_profiles
    -- Leave buyers a positive automatically: off | paid | delivered
    ADD COLUMN auto_feedback          TEXT NOT NULL DEFAULT 'off' CHECK (auto_feedback IN ('off', 'paid', 'delivered')),
    ADD COLUMN auto_feedback_comment  TEXT NOT NULL DEFAULT 'Great buyer, fast payment. Thank you!';

ALTER TABLE orders ADD COLUMN feedback_nudged_at TIMESTAMPTZ;

-- Feedback a seller earned elsewhere, checked by staff. Shown as its own
-- badge, never mixed into our score.
CREATE TABLE external_reputation (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    platform          TEXT NOT NULL CHECK (platform IN ('ebay', 'etsy', 'discogs', 'tcgplayer', 'comc', 'other')),
    profile_url       TEXT NOT NULL,
    feedback_count    INT NOT NULL CHECK (feedback_count >= 0),
    positive_percent  NUMERIC(5, 2) NOT NULL CHECK (positive_percent BETWEEN 0 AND 100),
    verify_code       TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'verified', 'rejected')),
    review_note       TEXT,
    reviewed_by       UUID REFERENCES users (id) ON DELETE SET NULL,
    reviewed_at       TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, platform)
);
CREATE INDEX external_reputation_status_idx ON external_reputation (status, created_at);
