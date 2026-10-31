-- USPS Intelligent Mail barcodes for envelope orders. Serials cycle through
-- 0-999999; USPS only needs each barcode unique for 45 days.
CREATE SEQUENCE imb_serial_seq MINVALUE 0 MAXVALUE 999999 START 1 CYCLE;

CREATE TABLE envelope_barcodes (
    order_id    UUID PRIMARY KEY REFERENCES orders (id) ON DELETE CASCADE,
    tracking    TEXT NOT NULL,   -- 20 digits: barcode ID, service type, mailer ID, serial
    routing     TEXT NOT NULL,   -- delivery ZIP, ZIP+4 or ZIP+4+delivery point
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX envelope_barcodes_tracking_idx ON envelope_barcodes (tracking);

-- When the scan job last asked USPS about a letter.
ALTER TABLE shipments ADD COLUMN scans_checked_at TIMESTAMPTZ;
