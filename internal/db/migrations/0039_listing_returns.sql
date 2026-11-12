-- Return policy per listing (seller hub bulk edit). NULL = the seller's default
-- (seller_profiles.returns_policy). Buyer pays return shipping either way.
ALTER TABLE listings ADD COLUMN returns_policy TEXT CHECK (returns_policy IN ('damaged_only', '30_days'));
