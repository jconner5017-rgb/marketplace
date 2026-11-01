-- Listings sent by inventory software (FireCardManager) are paused when
-- that software stops checking in, so a card can't sell twice while it's
-- offline, and come back on its next sync.
ALTER TABLE listings
    ADD COLUMN inventory_sync BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN sync_paused_at TIMESTAMPTZ;
UPDATE listings SET inventory_sync = true
WHERE external_id IS NOT NULL AND (starts_with(external_id, 'FCM-') OR starts_with(external_id, 'TCG-'));
CREATE INDEX listings_sync_paused_idx ON listings (seller_id) WHERE sync_paused_at IS NOT NULL;
