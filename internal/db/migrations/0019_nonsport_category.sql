-- Non-sport cards get their own top-level category instead of sitting
-- under trading card games: sports | tcg | nonsport.
ALTER TABLE listings DROP CONSTRAINT IF EXISTS listings_category_check;
ALTER TABLE listings ADD CONSTRAINT listings_category_check CHECK (category IN ('sports', 'tcg', 'nonsport'));
ALTER TABLE catalog_categories DROP CONSTRAINT IF EXISTS catalog_categories_kind_check;
ALTER TABLE catalog_categories ADD CONSTRAINT catalog_categories_kind_check CHECK (kind IN ('sports', 'tcg', 'nonsport'));

-- Move the themes added by 0018 (and any listings already using them).
UPDATE catalog_categories SET kind = 'nonsport', sort = sort - 20, updated_at = now()
WHERE kind = 'tcg' AND value IN ('Superheroes', 'Sci-Fi', 'Fantasy', 'Anime', 'Horror', 'Entertainment', 'Music', 'Historical', 'Novelty');
UPDATE listings SET category = 'nonsport', updated_at = now()
WHERE category = 'tcg' AND game_or_sport IN ('Superheroes', 'Sci-Fi', 'Fantasy', 'Anime', 'Horror', 'Entertainment', 'Music', 'Historical', 'Novelty');

INSERT INTO catalog_categories (kind, value, label, short_label, aliases, sort)
SELECT 'nonsport', 'Other non-sport', 'Other non-sport cards', 'Other non-sport', ARRAY['other', 'mixed', 'non-sport', 'nonsport'], 99
WHERE EXISTS (SELECT 1 FROM catalog_categories)
ON CONFLICT DO NOTHING;
