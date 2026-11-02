-- Which Stripe mode a seller's saved label card (platform customer) was
-- made in. Everything saved before this was made with the sandbox key.
-- A card from the other mode doesn't exist under the current key, so it's
-- dropped at startup and the seller saves a card again.
ALTER TABLE seller_profiles ADD COLUMN stripe_customer_livemode BOOLEAN NOT NULL DEFAULT false;
