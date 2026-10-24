-- Size limits per kind of title (settings keys quality.size_limits for movies
-- and quality.size_limits_tv for shows).
--
-- Until now one table served both. So nobody's checks change, the show table
-- starts as a copy of the table that is saved now (switch and numbers), once,
-- here. A value already set for shows is never touched, and an install that
-- never saved a table (which reads as off) is left with none.
INSERT INTO settings (key, value, encrypted)
SELECT 'quality.size_limits_tv', value, encrypted
FROM settings
WHERE key = 'quality.size_limits'
  AND NOT EXISTS (SELECT 1 FROM settings WHERE key = 'quality.size_limits_tv');
