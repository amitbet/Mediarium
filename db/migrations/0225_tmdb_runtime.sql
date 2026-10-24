-- How long a movie or an episode of a show is meant to be, as TMDB says it,
-- kept per TMDB id. The check that a file is really complete (an import turns
-- away a one-minute "episode", and the Library can list files that look cut
-- off) compares a file's length with it. minutes 0 means TMDB doesn't know;
-- that is looked at again after a while.
CREATE TABLE IF NOT EXISTS tmdb_runtime (
    kind       TEXT    NOT NULL, -- movie or tv
    tmdb_id    INTEGER NOT NULL,
    minutes    REAL    NOT NULL DEFAULT 0,
    checked_at TEXT    NOT NULL,
    PRIMARY KEY (kind, tmdb_id)
) WITHOUT ROWID;
