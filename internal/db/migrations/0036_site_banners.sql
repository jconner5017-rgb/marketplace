-- Editable promo banners staff manage in the back office. One row per
-- spot on the site; "home" sits on the homepage between Newly listed and
-- the shelves below it. Shown only while active and an image is set.
CREATE TABLE site_banners (
    slot        TEXT PRIMARY KEY CHECK (slot IN ('home')),
    image_url   TEXT NOT NULL DEFAULT '' CHECK (char_length(image_url) <= 500),
    link_url    TEXT NOT NULL DEFAULT '' CHECK (char_length(link_url) <= 500),
    alt_text    TEXT NOT NULL DEFAULT '' CHECK (char_length(alt_text) <= 150),
    active      BOOLEAN NOT NULL DEFAULT false,
    updated_by  UUID REFERENCES users (id) ON DELETE SET NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO site_banners (slot) VALUES ('home');
