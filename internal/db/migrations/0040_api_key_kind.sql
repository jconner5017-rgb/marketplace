-- Which app a seller API key is for. Only the desktop app (FireCardManager
-- on the seller's PC) cross-lists on eBay, so only its check-ins count for
-- the 3-day "hasn't synced" pause; FireCardManager Web is always online and
-- would otherwise hide a PC that has stopped.
ALTER TABLE api_keys ADD COLUMN kind TEXT NOT NULL DEFAULT 'desktop' CHECK (kind IN ('desktop', 'web'));
