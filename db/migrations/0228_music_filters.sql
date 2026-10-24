-- Which releases of an artist the music module lists and adds. The library-wide
-- choice lives in the settings; these three columns are one artist's own
-- choice (all NULL = use the library setting). filter_types and
-- filter_secondary are comma separated lists, "none" for an empty one.
ALTER TABLE artists ADD COLUMN filter_types TEXT;
ALTER TABLE artists ADD COLUMN filter_secondary TEXT;
ALTER TABLE artists ADD COLUMN filter_bootlegs INTEGER;

-- Whether a release group has an official release at all, found once and kept
-- (official = 0 means every release in it is a bootleg, promotion or
-- pseudo-release). It is looked at again after a long while.
CREATE TABLE IF NOT EXISTS music_group_status (
    mbid       TEXT PRIMARY KEY,
    official   INTEGER NOT NULL,
    checked_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
