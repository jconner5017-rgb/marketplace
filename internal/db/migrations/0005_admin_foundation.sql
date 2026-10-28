-- Admin back office, stage 1: staff roles, account status, authenticator
-- sign-in for staff, sign-in history, and a tamper-evident audit log.

-- Account status: frozen = can sign in and look, can't buy, sell, bid or
-- message; suspended = can't sign in until suspended_until; banned = never.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_status_check;
ALTER TABLE users ADD CONSTRAINT users_status_check
    CHECK (status IN ('active', 'frozen', 'suspended', 'banned'));
ALTER TABLE users
    ADD COLUMN staff_role        TEXT CHECK (staff_role IN ('super_admin', 'moderator', 'support', 'auditor')),
    ADD COLUMN suspended_until   TIMESTAMPTZ,
    ADD COLUMN status_reason     TEXT,
    ADD COLUMN status_changed_at TIMESTAMPTZ;
UPDATE users SET staff_role = 'super_admin' WHERE role = 'admin';
CREATE INDEX users_staff_idx ON users (staff_role) WHERE staff_role IS NOT NULL;

-- When this sign-in last passed the staff authenticator check.
ALTER TABLE sessions ADD COLUMN mfa_verified_at TIMESTAMPTZ;

-- Staff authenticator (TOTP). The secret is encrypted with MFA_ENCRYPTION_KEY;
-- backup codes are stored as SHA-256 hashes and removed when used.
CREATE TABLE staff_mfa (
    user_id        UUID PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    secret_enc     BYTEA NOT NULL,
    confirmed_at   TIMESTAMPTZ,
    backup_hashes  TEXT[] NOT NULL DEFAULT '{}',
    failed_count   INT NOT NULL DEFAULT 0,
    locked_until   TIMESTAMPTZ,
    last_step      BIGINT NOT NULL DEFAULT 0,   -- last time step used, so a code works once
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Every sign-in attempt, for account-takeover checks and the user page.
CREATE TABLE login_events (
    id          BIGSERIAL PRIMARY KEY,
    user_id     UUID REFERENCES users (id) ON DELETE CASCADE,
    email       TEXT NOT NULL,
    ip          TEXT,
    user_agent  TEXT,
    success     BOOLEAN NOT NULL,
    reason      TEXT,          -- bad_password, disabled, unknown_email
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX login_events_user_idx ON login_events (user_id, created_at DESC);
CREATE INDEX login_events_ip_idx ON login_events (ip, created_at DESC);

-- Audit log: one row per staff action. Rows can't be changed or deleted, and
-- each row's hash covers the previous row's hash, so removing or editing a
-- row (even directly in the database) breaks the chain.
CREATE TABLE audit_log (
    id           BIGSERIAL PRIMARY KEY,
    at           TIMESTAMPTZ NOT NULL,
    actor_id     UUID,          -- no foreign key: entries outlive accounts
    actor_email  TEXT NOT NULL,
    actor_role   TEXT,
    action       TEXT NOT NULL,
    target_type  TEXT,
    target_id    TEXT,
    reason       TEXT NOT NULL DEFAULT '',
    details      JSONB NOT NULL DEFAULT '{}',
    ip           TEXT,
    prev_hash    BYTEA,
    hash         BYTEA NOT NULL
);
CREATE INDEX audit_actor_idx ON audit_log (actor_id, id DESC);
CREATE INDEX audit_target_idx ON audit_log (target_type, target_id, id DESC);
CREATE INDEX audit_action_idx ON audit_log (action, id DESC);

CREATE FUNCTION audit_log_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit_log is append-only';
END $$;
CREATE TRIGGER audit_log_no_update BEFORE UPDATE OR DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_immutable();
CREATE TRIGGER audit_log_no_truncate BEFORE TRUNCATE ON audit_log
    FOR EACH STATEMENT EXECUTE FUNCTION audit_log_immutable();
