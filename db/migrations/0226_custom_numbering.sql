-- A person's own numbering for a show, for when the files and releases count
-- its seasons and episodes differently from the movie database, and the extra
-- names to search for. One row per show; a show's copies use the main show's
-- row. rules is a JSON list of {librarySeason, releaseSeason, offset, from, to},
-- names a JSON list of strings.
CREATE TABLE IF NOT EXISTS series_numbering_custom (
    series_id INTEGER PRIMARY KEY REFERENCES series(id) ON DELETE CASCADE,
    rules     TEXT NOT NULL DEFAULT '',
    names     TEXT NOT NULL DEFAULT ''
);
