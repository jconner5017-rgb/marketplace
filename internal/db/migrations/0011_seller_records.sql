-- Sellers can have Stripe Tax add sales tax at checkout (on their own Stripe account).
ALTER TABLE seller_profiles ADD COLUMN collect_tax BOOLEAN NOT NULL DEFAULT false;
-- Sales tax Stripe added on top of the order total (the seller remits it).
ALTER TABLE orders ADD COLUMN tax_cents INT NOT NULL DEFAULT 0 CHECK (tax_cents >= 0);
-- Sellers already raised by hand keep their level until staff release it,
-- so the new daily reputation check doesn't undo earlier decisions.
UPDATE seller_profiles SET tier_locked = true WHERE tier > 1;
