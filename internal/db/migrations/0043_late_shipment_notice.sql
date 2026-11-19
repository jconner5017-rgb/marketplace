-- When the buyer was told a paid order missed its ship-by date (once per order).
ALTER TABLE orders ADD COLUMN IF NOT EXISTS late_notified_at TIMESTAMPTZ;
