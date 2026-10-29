-- Seller photo uploads. Photos are re-encoded (strips EXIF/GPS) and kept
-- either in this table (PHOTO_STORE=db, the default) or in S3-compatible
-- object storage such as Cloudflare R2 (PHOTO_STORE=s3), with only the
-- metadata here. Unused uploads are swept after a day.
CREATE TABLE photos (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id      UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    store         TEXT NOT NULL CHECK (store IN ('db', 's3')),
    object_key    TEXT,
    url           TEXT NOT NULL UNIQUE,
    content_type  TEXT NOT NULL,
    width         INT NOT NULL,
    height        INT NOT NULL,
    size_bytes    INT NOT NULL,
    data          BYTEA,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX photos_owner_idx ON photos (owner_id, created_at DESC);
CREATE INDEX listings_photo_urls_gin ON listings USING gin (photo_urls);
