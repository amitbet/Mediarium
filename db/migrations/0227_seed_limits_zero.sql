-- Seeding limits: empty is unlimited, 0 is "do not seed".
--
-- Until now a 0 in the seed ratio limit, the seed time limit and the upload
-- limit meant "no limit". From now on an empty box is no limit and 0 switches
-- seeding (or uploading) off. So nobody's torrents start behaving differently,
-- every 0 that is saved now is rewritten to its old meaning, once, here.

-- An indexer's own rule: a 0 overrode the global limit with "no limit". When
-- the global limit is a real number that has to stay a way to say "no limit
-- here" (-1); when the global one is none anyway, an empty field says the same.
UPDATE indexer_seed_rules
SET ratio = CASE
    WHEN COALESCE((SELECT CAST(value AS REAL) FROM settings WHERE key = 'torrent.seed_ratio_limit'), 0) > 0 THEN -1
    ELSE NULL END
WHERE ratio = 0;

UPDATE indexer_seed_rules
SET hours = CASE
    WHEN COALESCE((SELECT CAST(value AS REAL) FROM settings WHERE key = 'torrent.seed_time_limit_h'), 0) > 0 THEN -1
    ELSE NULL END
WHERE hours = 0;

DELETE FROM indexer_seed_rules WHERE ratio IS NULL AND hours IS NULL;

-- The global limits: a saved number that is zero (0, 0.0, 0,0) becomes empty.
-- Text that is not a number at all was read as 0 before and reads as unlimited
-- now, so it needs no change.
UPDATE settings
SET value = ''
WHERE key IN ('torrent.seed_ratio_limit', 'torrent.seed_time_limit_h', 'torrent.upload_limit_kb')
  AND trim(value) <> ''
  AND trim(value) NOT GLOB '*[^0-9.,]*'
  AND CAST(replace(trim(value), ',', '.') AS REAL) = 0;
