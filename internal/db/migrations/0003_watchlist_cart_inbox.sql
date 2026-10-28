-- Cards a user is keeping an eye on.
CREATE TABLE watchlist (
    user_id     UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    listing_id  UUID NOT NULL REFERENCES listings (id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, listing_id)
);
CREATE INDEX watchlist_listing_idx ON watchlist (listing_id);

-- Cart lines. Prices are read live from listings, never stored here.
CREATE TABLE cart_items (
    user_id     UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    listing_id  UUID NOT NULL REFERENCES listings (id) ON DELETE CASCADE,
    quantity    INT NOT NULL CHECK (quantity BETWEEN 1 AND 100),
    added_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, listing_id)
);

-- One conversation per buyer per listing.
CREATE TABLE message_threads (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    listing_id       UUID REFERENCES listings (id) ON DELETE SET NULL,
    buyer_id         UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    seller_id        UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_message_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (listing_id, buyer_id)
);
CREATE INDEX message_threads_buyer_idx  ON message_threads (buyer_id, last_message_at DESC);
CREATE INDEX message_threads_seller_idx ON message_threads (seller_id, last_message_at DESC);

-- Every message is kept for dispute evidence and admin review (disclosed in
-- the terms). Blocked messages are stored but never shown to the recipient.
CREATE TABLE messages (
    id          BIGSERIAL PRIMARY KEY,
    thread_id   UUID NOT NULL REFERENCES message_threads (id) ON DELETE CASCADE,
    sender_id   UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    body        TEXT NOT NULL,
    blocked     TEXT,              -- reason, e.g. 'off_platform_payment'; NULL = delivered
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX messages_thread_idx ON messages (thread_id, id);
CREATE INDEX messages_blocked_idx ON messages (created_at) WHERE blocked IS NOT NULL;

CREATE TABLE thread_reads (
    thread_id     UUID NOT NULL REFERENCES message_threads (id) ON DELETE CASCADE,
    user_id       UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    last_read_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (thread_id, user_id)
);

-- Per-user notices for the bell (new message, later: sales, shipping).
CREATE TABLE notifications (
    id          BIGSERIAL PRIMARY KEY,
    user_id     UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,      -- message, order, system
    title       TEXT NOT NULL,
    body        TEXT NOT NULL DEFAULT '',
    link        TEXT,
    read_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX notifications_user_idx ON notifications (user_id, created_at DESC);
CREATE INDEX notifications_unread_idx ON notifications (user_id) WHERE read_at IS NULL;

-- Site-wide announcements from the marketplace, shown in everyone's bell.
CREATE TABLE announcements (
    id          BIGSERIAL PRIMARY KEY,
    title       TEXT NOT NULL,
    body        TEXT NOT NULL DEFAULT '',
    link        TEXT,
    created_by  UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE announcement_reads (
    announcement_id  BIGINT NOT NULL REFERENCES announcements (id) ON DELETE CASCADE,
    user_id          UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    PRIMARY KEY (announcement_id, user_id)
);
