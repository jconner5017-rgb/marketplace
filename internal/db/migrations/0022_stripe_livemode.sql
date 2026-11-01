-- Which Stripe mode a seller's account was made in. Every account so far
-- was made with the sandbox (test) key; when the site switches to the live
-- key, those sellers set up payments again with a real account.
ALTER TABLE seller_profiles ADD COLUMN stripe_livemode BOOLEAN NOT NULL DEFAULT false;
