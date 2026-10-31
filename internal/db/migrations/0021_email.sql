-- Email: confirming addresses, password resets, an outbox, and staff blasts.

-- Members who signed up before confirmation existed aren't required to
-- confirm (they're still asked to); everyone new is.
ALTER TABLE users
    ADD COLUMN verify_required BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN email_opt_out   BOOLEAN NOT NULL DEFAULT false;
UPDATE users SET verify_required = false;

CREATE TABLE email_tokens (
    token_hash  BYTEA PRIMARY KEY,
    user_id     UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    purpose     TEXT NOT NULL CHECK (purpose IN ('verify', 'reset')),
    email       TEXT NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX email_tokens_user_idx ON email_tokens (user_id, purpose, created_at DESC);

CREATE TABLE email_broadcasts (
    id          BIGSERIAL PRIMARY KEY,
    subject     TEXT NOT NULL,
    body        TEXT NOT NULL,
    audience    TEXT NOT NULL CHECK (audience IN ('all', 'sellers')),
    created_by  UUID REFERENCES users (id) ON DELETE SET NULL,
    recipients  INT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE email_outbox (
    id            BIGSERIAL PRIMARY KEY,
    user_id       UUID REFERENCES users (id) ON DELETE CASCADE,
    to_email      TEXT NOT NULL,
    kind          TEXT NOT NULL CHECK (kind IN ('transactional', 'broadcast')),
    subject       TEXT NOT NULL,
    body          TEXT NOT NULL,
    broadcast_id  BIGINT REFERENCES email_broadcasts (id) ON DELETE CASCADE,
    status        TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'sent', 'failed')),
    attempts      INT NOT NULL DEFAULT 0,
    last_error    TEXT,
    send_after    TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at       TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX email_outbox_queue_idx ON email_outbox (send_after, id) WHERE status = 'queued';
CREATE INDEX email_outbox_broadcast_idx ON email_outbox (broadcast_id) WHERE broadcast_id IS NOT NULL;
