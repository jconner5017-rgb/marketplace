-- Admin back office, stage 2: staff notes on members and seller approval.

CREATE TABLE staff_notes (
    id            BIGSERIAL PRIMARY KEY,
    user_id       UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    author_id     UUID REFERENCES users (id) ON DELETE SET NULL,
    author_email  TEXT NOT NULL,
    body          TEXT NOT NULL CHECK (length(body) BETWEEN 1 AND 4000),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX staff_notes_user_idx ON staff_notes (user_id, id DESC);

-- Our own approval step on top of Stripe's identity checks. Rejected
-- sellers' listings are hidden (not deleted).
ALTER TABLE seller_profiles
    ADD COLUMN review_status TEXT NOT NULL DEFAULT 'pending'
        CHECK (review_status IN ('pending', 'approved', 'rejected')),
    ADD COLUMN reviewed_by   UUID REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN reviewed_at   TIMESTAMPTZ,
    ADD COLUMN review_note   TEXT;
CREATE INDEX seller_review_idx ON seller_profiles (review_status, created_at);
