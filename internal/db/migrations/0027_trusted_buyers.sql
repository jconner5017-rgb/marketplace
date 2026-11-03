-- Trusted buyers: a seller's friends and regulars, who may choose to have a
-- single raw card up to $100 sent in a stamped envelope instead of a tracked
-- package. Envelopes have no delivery confirmation, so both sides agree to
-- it: the seller by trusting the buyer, the buyer by choosing it.
CREATE TABLE trusted_buyers (
    seller_id UUID NOT NULL REFERENCES users (id),
    buyer_id  UUID NOT NULL REFERENCES users (id),
    via       TEXT NOT NULL DEFAULT 'list' CHECK (via IN ('list', 'invite', 'order')),
    added_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (seller_id, buyer_id),
    CHECK (seller_id <> buyer_id)
);
CREATE INDEX trusted_buyers_buyer ON trusted_buyers (buyer_id);

-- A seller's invite link: whoever opens it and accepts joins their list.
ALTER TABLE seller_profiles ADD COLUMN trust_invite TEXT UNIQUE;

-- envelope_trusted: shipping by envelope over the usual $20 limit, agreed
-- by a trusted buyer. Not-received cases aren't covered on these orders.
-- envelope_offer: the seller offered an envelope after the sale
-- (offered, accepted or declined).
ALTER TABLE orders ADD COLUMN envelope_trusted BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE orders ADD COLUMN envelope_offer TEXT CHECK (envelope_offer IN ('offered', 'accepted', 'declined'));
