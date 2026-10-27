-- Users: every account starts as a buyer. Selling is unlocked later by
-- connecting a Stripe account (seller_profiles, phase 2).
CREATE TABLE users (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email             TEXT NOT NULL,
    username          TEXT NOT NULL,
    password_hash     TEXT NOT NULL,
    role              TEXT NOT NULL DEFAULT 'user'
                      CHECK (role IN ('user', 'admin')),
    status            TEXT NOT NULL DEFAULT 'active'
                      CHECK (status IN ('active', 'suspended', 'banned')),
    email_verified_at TIMESTAMPTZ,
    buyer_strikes     INT NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Case-insensitive uniqueness: "Bob@Mail.com" and "bob@mail.com" are one account.
CREATE UNIQUE INDEX users_email_key    ON users (lower(email));
CREATE UNIQUE INDEX users_username_key ON users (lower(username));

-- Server-side sessions. Only a SHA-256 hash of the cookie token is stored,
-- so a leaked database cannot be used to log in as anyone.
CREATE TABLE sessions (
    token_hash   BYTEA PRIMARY KEY,
    user_id      UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ NOT NULL,
    ip           TEXT,
    user_agent   TEXT
);

CREATE INDEX sessions_user_id_idx    ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);
