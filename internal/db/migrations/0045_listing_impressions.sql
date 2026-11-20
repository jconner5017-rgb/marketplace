-- Seller hub traffic (2026-11-20): how often each listing was shown in search
-- results, browse rows and store pages (impressions), per day, so sellers see
-- impressions and click-through rate (views / impressions) like on eBay.
-- Crawlers aren't counted.
CREATE TABLE listing_impressions_daily (
    listing_id  UUID NOT NULL REFERENCES listings (id) ON DELETE CASCADE,
    day         DATE NOT NULL,
    impressions INT NOT NULL DEFAULT 0,
    PRIMARY KEY (listing_id, day)
);
CREATE INDEX listing_impressions_day ON listing_impressions_daily (day);
