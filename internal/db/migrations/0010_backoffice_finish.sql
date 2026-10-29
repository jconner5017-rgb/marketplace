-- Back office, final pieces: admin IP allowlist, money ledger, member
-- warnings, appeals, and identifiers blocked after a ban.

-- Staff can reach /admin only from these networks. No rows = no restriction.
CREATE TABLE admin_ip_rules (
    id          BIGSERIAL PRIMARY KEY,
    cidr        CIDR NOT NULL UNIQUE,
    label       TEXT NOT NULL DEFAULT '',
    created_by  UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Every money event on an order, append-only. Amounts are positive; kind
-- says which way the money moved. Corrections are new rows.
CREATE TABLE ledger_entries (
    id             BIGSERIAL PRIMARY KEY,
    order_id       UUID REFERENCES orders (id),
    seller_id      UUID REFERENCES users (id),
    kind           TEXT NOT NULL CHECK (kind IN ('charge', 'application_fee', 'discount', 'label_cost', 'refund',
                       'fee_refund', 'chargeback_opened', 'chargeback_won', 'chargeback_lost', 'adjustment')),
    amount_cents   INT NOT NULL CHECK (amount_cents >= 0),
    stripe_object  TEXT,
    note           TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (kind, stripe_object)
);
CREATE INDEX ledger_order_idx ON ledger_entries (order_id, id);
CREATE INDEX ledger_seller_idx ON ledger_entries (seller_id, created_at);

CREATE FUNCTION ledger_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'ledger_entries is append-only';
END $$;
CREATE TRIGGER ledger_no_update BEFORE UPDATE OR DELETE ON ledger_entries
    FOR EACH ROW EXECUTE FUNCTION ledger_immutable();

ALTER TABLE refund_reports ADD COLUMN status TEXT CHECK (status IN ('open', 'won', 'lost', 'closed'));
CREATE UNIQUE INDEX refund_reports_stripe_idx ON refund_reports (kind, stripe_ref) WHERE stripe_ref IS NOT NULL;

-- Formal warnings staff give members (shown to the member).
CREATE TABLE member_warnings (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    reason      TEXT NOT NULL,
    created_by  UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    rescinded_at TIMESTAMPTZ   -- set when an appeal overturns it
);
CREATE INDEX member_warnings_user_idx ON member_warnings (user_id, created_at DESC);

-- One appeal per decision. "about" names the decision; ref is its id
-- (warning id, listing id) or the status change time for suspensions and bans.
CREATE TABLE appeals (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    about          TEXT NOT NULL CHECK (about IN ('suspension', 'ban', 'freeze', 'warning', 'demotion', 'listing')),
    ref            TEXT NOT NULL,
    message        TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'upheld', 'overturned')),
    decided_by     UUID REFERENCES users (id) ON DELETE SET NULL,
    decision_note  TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_at     TIMESTAMPTZ,
    UNIQUE (user_id, about, ref)
);
CREATE INDEX appeals_open_idx ON appeals (status, created_at);

-- Identifiers of banned members, hashed. Email matches refuse sign-up;
-- IP and address matches raise a fraud alert.
CREATE TABLE blocked_identifiers (
    kind        TEXT NOT NULL CHECK (kind IN ('email', 'ip', 'address', 'card')),
    value_hash  TEXT NOT NULL,
    user_id     UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (kind, value_hash)
);

-- Seller tier changes, by the nightly reputation job or staff.
CREATE TABLE tier_changes (
    id          BIGSERIAL PRIMARY KEY,
    user_id     UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    from_tier   SMALLINT NOT NULL,
    to_tier     SMALLINT NOT NULL,
    reason      TEXT NOT NULL,
    by_staff    UUID REFERENCES users (id) ON DELETE SET NULL,
    stats       JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE seller_profiles ADD COLUMN tier_locked BOOLEAN NOT NULL DEFAULT false;  -- staff demotion holds until lifted
ALTER TABLE listings ADD COLUMN tier_hidden BOOLEAN NOT NULL DEFAULT false;          -- hidden by a demotion, restored on promotion
