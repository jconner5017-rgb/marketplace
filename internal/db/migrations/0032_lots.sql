-- Lots: one listing that sells several cards together (like an eBay lot).
-- NULL means a single card.
ALTER TABLE listings ADD COLUMN lot_count INT
    CHECK (lot_count IS NULL OR lot_count BETWEEN 2 AND 5000);
CREATE INDEX listings_lot_idx ON listings (status) WHERE lot_count IS NOT NULL;
