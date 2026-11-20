-- Small copies of card photos for tiles and rows (search, home, stores):
-- about 400 px wide instead of the full photo. NULL = not made yet,
-- '' = couldn't be made (the tile uses the full photo).
ALTER TABLE photos ADD COLUMN thumb_url TEXT;

-- When the photo storage's public address changes (PHOTO_PUBLIC_URL), the
-- stored addresses are moved over and the old prefix is remembered, so
-- apps that send an old address are mapped to the new one.
CREATE TABLE photo_url_aliases (
    old_prefix TEXT PRIMARY KEY,
    new_prefix TEXT NOT NULL,
    moved_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- "Newest" with no category filter (home page, all listings).
CREATE INDEX listings_newest_idx ON listings (status, created_at DESC, id);
