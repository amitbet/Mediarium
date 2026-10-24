-- TVmaze: the exact moment an episode airs (UTC, RFC 3339; NULL when TVmaze has
-- no time for it), and the show's id on TVmaze (0 = not looked up yet,
-- -1 = TVmaze does not know it).
ALTER TABLE episodes ADD COLUMN air_at TEXT;
ALTER TABLE series ADD COLUMN tvmaze_id INTEGER NOT NULL DEFAULT 0;
-- When TVmaze was last asked about the show (RFC 3339, UTC), so a finished show
-- is asked only once.
ALTER TABLE series ADD COLUMN tvmaze_checked_at TEXT NOT NULL DEFAULT '';

-- The wanted lists leave out an episode whose air time is still ahead, so the
-- index that answers them holds air_at too (and stays a covering index).
DROP INDEX IF EXISTS episodes_wanted;
CREATE INDEX episodes_wanted ON episodes (series_id, air_date, file_path, air_at) WHERE status = 'missing' AND monitored = 1;
