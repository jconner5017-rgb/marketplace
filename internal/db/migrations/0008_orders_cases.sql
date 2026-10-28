-- Orders (filled by checkout), shipping, dispute cases, refund and
-- chargeback reports, feedback, and daily listing views.

-- US shipping addresses.
CREATE TABLE addresses (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    line1       TEXT NOT NULL,
    line2       TEXT NOT NULL DEFAULT '',
    city        TEXT NOT NULL,
    state       TEXT NOT NULL CHECK (state ~ '^[A-Z]{2}$'),
    zip         TEXT NOT NULL CHECK (zip ~ '^[0-9]{5}(-[0-9]{4})?$'),
    is_default  BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX addresses_user_idx ON addresses (user_id);

-- Order numbers people can read out: 100001, 100002, ...
CREATE SEQUENCE order_number_seq START 100001;

-- One order per seller per checkout. Money is paid to the seller's own
-- Stripe account; fee_cents is our application fee.
CREATE TABLE orders (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    number                 BIGINT NOT NULL UNIQUE DEFAULT nextval('order_number_seq'),
    buyer_id               UUID NOT NULL REFERENCES users (id),
    seller_id              UUID NOT NULL REFERENCES users (id),
    status                 TEXT NOT NULL DEFAULT 'pending_payment' CHECK (status IN
                               ('pending_payment', 'paid', 'shipped', 'delivered', 'cancelled', 'refunded', 'partially_refunded')),
    subtotal_cents         INT NOT NULL CHECK (subtotal_cents >= 0),
    discount_cents         INT NOT NULL DEFAULT 0 CHECK (discount_cents >= 0),
    total_cents            INT NOT NULL CHECK (total_cents >= 0),
    fee_cents              INT NOT NULL DEFAULT 0 CHECK (fee_cents >= 0),
    fee_schedule_id        INT REFERENCES fee_schedules (id),
    refunded_cents         INT NOT NULL DEFAULT 0 CHECK (refunded_cents >= 0),
    ship_method            TEXT NOT NULL DEFAULT 'label' CHECK (ship_method IN ('envelope', 'label')),
    ship_to                JSONB NOT NULL DEFAULT '{}',
    ship_by                TIMESTAMPTZ,              -- three business days after payment
    stripe_account         TEXT,
    stripe_checkout_id     TEXT UNIQUE,
    stripe_payment_intent  TEXT,
    stripe_charge          TEXT,
    cancel_reason          TEXT,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    paid_at                TIMESTAMPTZ,
    shipped_at             TIMESTAMPTZ,
    delivered_at           TIMESTAMPTZ,
    cancelled_at           TIMESTAMPTZ,
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX orders_buyer_idx ON orders (buyer_id, created_at DESC);
CREATE INDEX orders_seller_idx ON orders (seller_id, created_at DESC);
CREATE INDEX orders_status_idx ON orders (status, created_at DESC);
CREATE INDEX orders_paid_idx ON orders (paid_at) WHERE paid_at IS NOT NULL;

CREATE TABLE order_items (
    id                BIGSERIAL PRIMARY KEY,
    order_id          UUID NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    listing_id        UUID REFERENCES listings (id) ON DELETE SET NULL,
    listing_version   INT,                 -- the listing_versions snapshot the buyer saw
    hold_id           UUID REFERENCES holds (id) ON DELETE SET NULL,
    source            TEXT NOT NULL CHECK (source IN ('cart', 'auction', 'offer')),
    title             TEXT NOT NULL,
    unit_cents        INT NOT NULL CHECK (unit_cents > 0),
    quantity          INT NOT NULL CHECK (quantity > 0)
);
CREATE INDEX order_items_order_idx ON order_items (order_id);
CREATE INDEX order_items_listing_idx ON order_items (listing_id);

CREATE TABLE shipments (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id         UUID NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    method           TEXT NOT NULL CHECK (method IN ('envelope', 'label')),
    carrier          TEXT,
    service          TEXT,
    tracking_number  TEXT,
    label_cost_cents INT,
    status           TEXT NOT NULL DEFAULT 'label_created'
                     CHECK (status IN ('label_created', 'in_transit', 'delivered', 'exception', 'returned')),
    last_event       TEXT,
    last_event_at    TIMESTAMPTZ,
    events           JSONB NOT NULL DEFAULT '[]',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX shipments_order_idx ON shipments (order_id);
CREATE INDEX shipments_status_idx ON shipments (status, updated_at DESC);

-- Dispute cases: item not received (inr), not as described (snad), other.
CREATE SEQUENCE case_number_seq START 5001;
CREATE TABLE cases (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    number       BIGINT NOT NULL UNIQUE DEFAULT nextval('case_number_seq'),
    order_id     UUID NOT NULL REFERENCES orders (id),
    opened_by    UUID REFERENCES users (id) ON DELETE SET NULL,
    kind         TEXT NOT NULL CHECK (kind IN ('inr', 'snad', 'other')),
    status       TEXT NOT NULL DEFAULT 'open' CHECK (status IN
                     ('open', 'waiting_seller', 'waiting_buyer', 'resolved_buyer', 'resolved_seller', 'closed')),
    summary      TEXT NOT NULL,
    assigned_to  UUID REFERENCES users (id) ON DELETE SET NULL,
    outcome      TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at    TIMESTAMPTZ
);
CREATE INDEX cases_status_idx ON cases (status, created_at);
CREATE INDEX cases_order_idx ON cases (order_id);

CREATE TABLE case_notes (
    id            BIGSERIAL PRIMARY KEY,
    case_id       UUID NOT NULL REFERENCES cases (id) ON DELETE CASCADE,
    author_id     UUID REFERENCES users (id) ON DELETE SET NULL,
    author_email  TEXT NOT NULL,
    internal      BOOLEAN NOT NULL DEFAULT true,   -- false = shared with buyer and seller
    body          TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX case_notes_case_idx ON case_notes (case_id, id);

-- Refunds and chargebacks. Sellers refund from their own Stripe accounts;
-- these rows record what members, staff or Stripe reported.
CREATE TABLE refund_reports (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id      UUID REFERENCES orders (id) ON DELETE SET NULL,
    reported_by   UUID REFERENCES users (id) ON DELETE SET NULL,
    source        TEXT NOT NULL CHECK (source IN ('seller', 'buyer', 'staff', 'stripe')),
    kind          TEXT NOT NULL CHECK (kind IN ('refund', 'partial_refund', 'chargeback', 'chargeback_won', 'chargeback_lost')),
    amount_cents  INT NOT NULL CHECK (amount_cents >= 0),
    stripe_ref    TEXT,
    note          TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX refund_reports_order_idx ON refund_reports (order_id);
CREATE INDEX refund_reports_kind_idx ON refund_reports (kind, created_at DESC);

-- Feedback after an order: positive (1), neutral (0), negative (-1).
CREATE TABLE feedback (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id        UUID NOT NULL REFERENCES orders (id),
    from_user       UUID NOT NULL REFERENCES users (id),
    to_user         UUID NOT NULL REFERENCES users (id),
    role            TEXT NOT NULL CHECK (role IN ('buyer_to_seller', 'seller_to_buyer')),
    rating          SMALLINT NOT NULL CHECK (rating IN (-1, 0, 1)),
    comment         TEXT NOT NULL DEFAULT '',
    reply           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    removed_at      TIMESTAMPTZ,
    removed_by      UUID REFERENCES users (id) ON DELETE SET NULL,
    removed_reason  TEXT,
    UNIQUE (order_id, from_user)
);
CREATE INDEX feedback_to_idx ON feedback (to_user, created_at DESC);

-- Listing page views per day (for conversion rate).
CREATE TABLE listing_views_daily (
    listing_id  UUID NOT NULL REFERENCES listings (id) ON DELETE CASCADE,
    day         DATE NOT NULL,
    views       INT NOT NULL DEFAULT 0,
    PRIMARY KEY (listing_id, day)
);
CREATE INDEX listing_views_day_idx ON listing_views_daily (day);
