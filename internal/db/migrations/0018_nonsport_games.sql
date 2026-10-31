-- Non-sport trading cards (superheroes, sci-fi, horror, ...) listed as
-- trading card games. Only for databases whose catalog is already filled;
-- a fresh database gets them from the built-in list instead.
INSERT INTO catalog_categories (kind, value, label, short_label, aliases, sort)
SELECT 'tcg', v.value, v.label, v.short, v.aliases, v.sort
FROM (VALUES
    ('Superheroes',   'Superheroes (Marvel, DC)', 'Superheroes', ARRAY['superhero', 'marvel', 'dc comics', 'comics'], 20),
    ('Sci-Fi',        'Sci-Fi',                   'Sci-Fi',      ARRAY['scifi', 'sci fi', 'science fiction', 'star trek'], 21),
    ('Fantasy',       'Fantasy',                  'Fantasy',     ARRAY[]::text[], 22),
    ('Anime',         'Anime',                    'Anime',       ARRAY[]::text[], 23),
    ('Horror',        'Horror',                   'Horror',      ARRAY[]::text[], 24),
    ('Entertainment', 'Movies & TV',              'Movies & TV', ARRAY['movies', 'tv', 'television', 'non-sport'], 25),
    ('Music',         'Music',                    'Music',       ARRAY['rock'], 26),
    ('Historical',    'Historical',               'Historical',  ARRAY['history'], 27),
    ('Novelty',       'Novelty',                  'Novelty',     ARRAY[]::text[], 28)
) AS v(value, label, short, aliases, sort)
WHERE EXISTS (SELECT 1 FROM catalog_categories)
ON CONFLICT DO NOTHING;
UPDATE catalog_categories SET sort = 99 WHERE kind = 'tcg' AND value = 'Other TCG' AND EXISTS (SELECT 1 FROM catalog_categories WHERE value = 'Superheroes');
