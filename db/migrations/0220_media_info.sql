-- What the library's video files hold (codec, size, HDR, audio and subtitle
-- tracks), read from the files' headers. One row per file path, so a file
-- shared by several episodes is read once. size and mtime are what the file
-- looked like when it was read: a file with the same size and time is not
-- read again. The short columns are the same facts in a form the Library can
-- filter on (comma-separated lists); info holds the full result as JSON.
CREATE TABLE media_info (
    path       TEXT    PRIMARY KEY,
    size       INTEGER NOT NULL,
    mtime      INTEGER NOT NULL,
    scanned_at TEXT    NOT NULL,
    checked_at TEXT    NOT NULL DEFAULT '',  -- last time the file was looked at (even if unchanged)
    failed     INTEGER NOT NULL DEFAULT 0,   -- 1 when the headers could not be read
    info       TEXT    NOT NULL DEFAULT '',
    container  TEXT    NOT NULL DEFAULT '',
    vcodec     TEXT    NOT NULL DEFAULT '',
    hdr        TEXT    NOT NULL DEFAULT '',
    res        TEXT    NOT NULL DEFAULT '',
    acodecs    TEXT    NOT NULL DEFAULT '',
    alangs     TEXT    NOT NULL DEFAULT '',
    slangs     TEXT    NOT NULL DEFAULT ''
);
