-- Mixed-sport lots (and multi-sport cards) need a sport to list under.
-- Fresh databases get it from the built-in list instead.
INSERT INTO catalog_categories (kind, value, label, short_label, aliases, sort)
SELECT 'sports', 'Multi-Sport', 'Multi-Sport (mixed sports)', 'Multi-Sport', ARRAY['multisport', 'multi sport', 'mixed sports', 'mixed'], 50
WHERE EXISTS (SELECT 1 FROM catalog_categories)
ON CONFLICT DO NOTHING;
