-- More trading card games, so every game on eBay's Collectible Card Game
-- list has a home (FireCardManagerTCG lists them here by these names). Only
-- for databases whose catalog is already filled; a fresh database gets them
-- from the built-in list.
INSERT INTO catalog_categories (kind, value, label, short_label, aliases, sort)
SELECT 'tcg', v.value, v.label, v.short, v.aliases, v.sort
FROM (VALUES
    ('Sorcery',           'Sorcery: Contested Realm',          'Sorcery',           ARRAY['sorcery contested realm', 'sorcery tcg'], 40),
    ('MetaZoo',           'MetaZoo',                           'MetaZoo',           ARRAY['metazoo ccg', 'metazoo tcg'], 41),
    ('Riftbound',         'Riftbound: League of Legends TCG',  'Riftbound',         ARRAY['riftbound tcg', 'league of legends tcg'], 42),
    ('Netrunner',         'Netrunner',                         'Netrunner',         ARRAY['android netrunner', 'netrunner lcg'], 43),
    ('World of Warcraft', 'World of Warcraft TCG',             'World of Warcraft', ARRAY['wow tcg', 'world of warcraft tcg'], 44),
    ('My Little Pony',    'My Little Pony CCG',                'My Little Pony',    ARRAY['mlp ccg', 'my little pony ccg'], 45)
) AS v(value, label, short, aliases, sort)
WHERE EXISTS (SELECT 1 FROM catalog_categories)
ON CONFLICT DO NOTHING;
