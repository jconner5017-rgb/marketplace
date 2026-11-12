-- A seller's return policy, shown on every listing. Cards that arrive damaged
-- or not as described are always covered for 30 days (site rule); sellers can
-- also accept any return within 30 days of delivery (buyer pays return shipping).
ALTER TABLE seller_profiles ADD COLUMN returns_policy TEXT NOT NULL DEFAULT 'damaged_only'
    CHECK (returns_policy IN ('damaged_only', '30_days'));
