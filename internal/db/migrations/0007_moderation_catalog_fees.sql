-- Admin back office, stage 3: listing moderation, member reports, the
-- category and feature catalog, and the fee schedule.

-- "blocked" = taken down by staff (kept for records; the seller can't undo it).
ALTER TABLE listings DROP CONSTRAINT IF EXISTS listings_status_check;
ALTER TABLE listings ADD CONSTRAINT listings_status_check
    CHECK (status IN ('active', 'sold_out', 'hidden', 'removed', 'ended', 'blocked'));
ALTER TABLE listings
    ADD COLUMN blocked_prev_status TEXT,
    ADD COLUMN moderated_by        UUID REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN moderated_at        TIMESTAMPTZ,
    ADD COLUMN moderation_reason   TEXT;

-- Reports from members about listings, members, messages or offers.
CREATE TABLE reports (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    reporter_id      UUID REFERENCES users (id) ON DELETE SET NULL,
    target_type      TEXT NOT NULL CHECK (target_type IN ('listing', 'user', 'message', 'offer')),
    target_id        TEXT NOT NULL,
    target_user_id   UUID REFERENCES users (id) ON DELETE SET NULL,  -- whose listing/message it is
    reason           TEXT NOT NULL CHECK (reason IN ('counterfeit', 'prohibited', 'keyword_spam', 'misdescribed',
                         'off_platform', 'abuse', 'shill_bidding', 'other')),
    details          TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'actioned', 'dismissed')),
    resolved_by      UUID REFERENCES users (id) ON DELETE SET NULL,
    resolved_at      TIMESTAMPTZ,
    resolution_note  TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX reports_one_open ON reports (reporter_id, target_type, target_id) WHERE status = 'open';
CREATE INDEX reports_status_idx ON reports (status, created_at);
CREATE INDEX reports_target_idx ON reports (target_type, target_id);
CREATE INDEX reports_target_user_idx ON reports (target_user_id, status);

-- Sports and games buyers browse by. value is what listings store.
-- Rows are filled from the built-in lists the first time the API starts.
CREATE TABLE catalog_categories (
    id          SERIAL PRIMARY KEY,
    kind        TEXT NOT NULL CHECK (kind IN ('sports', 'tcg')),
    value       TEXT NOT NULL,
    label       TEXT NOT NULL,
    short_label TEXT NOT NULL,
    aliases     TEXT[] NOT NULL DEFAULT '{}',
    active      BOOLEAN NOT NULL DEFAULT true,
    sort        INT NOT NULL DEFAULT 0,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX catalog_categories_value ON catalog_categories (kind, lower(value));

-- Card features (item specifics): holo, rookie, auto, ...
CREATE TABLE catalog_attributes (
    key         TEXT PRIMARY KEY CHECK (key ~ '^[a-z0-9_]{2,40}$'),
    label       TEXT NOT NULL,
    active      BOOLEAN NOT NULL DEFAULT true,
    sort        INT NOT NULL DEFAULT 0,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Our fee per card sold: flat_cents up to threshold_cents, then percent_bps
-- of the price, at least flat_cents and at most cap_cents. The schedule in
-- force is the latest one whose effective_at has passed.
CREATE TABLE fee_schedules (
    id               SERIAL PRIMARY KEY,
    flat_cents       INT NOT NULL CHECK (flat_cents >= 0),
    threshold_cents  INT NOT NULL CHECK (threshold_cents >= 0),
    percent_bps      INT NOT NULL CHECK (percent_bps BETWEEN 0 AND 5000),
    cap_cents        INT NOT NULL CHECK (cap_cents >= 0),
    effective_at     TIMESTAMPTZ NOT NULL,
    note             TEXT NOT NULL DEFAULT '',
    created_by       UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO fee_schedules (flat_cents, threshold_cents, percent_bps, cap_cents, effective_at, note)
VALUES (150, 10000, 300, 2500, '2026-01-01', 'Launch fees');

-- Optional paid listing upgrades. Not offered until switched on.
CREATE TABLE fee_upgrades (
    key          TEXT PRIMARY KEY,
    label        TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    price_cents  INT NOT NULL CHECK (price_cents >= 0),
    active       BOOLEAN NOT NULL DEFAULT false,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO fee_upgrades (key, label, description, price_cents) VALUES
    ('bold_title', 'Bold title', 'The listing title shows in bold in search results.', 50),
    ('featured', 'Featured placement', 'Shown in a featured row on the home page and category pages for 7 days.', 499),
    ('extra_photos', 'Extra photos', 'Up to 24 photos instead of 12.', 25);
