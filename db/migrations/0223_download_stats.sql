-- Download statistics that survive clearing the history. The Statistics page
-- and the dashboard used to add them up from the download list, so "Clear
-- finished" (or the history's retention) took the totals with it. They are now
-- counted here as downloads happen, by day, kind (movie, tv, music, book) and
-- protocol, and nothing that removes a download from the list touches them.
-- Triggers do the counting, so every way a download is added, finished or
-- failed is covered without each writer having to know.
CREATE TABLE IF NOT EXISTS download_stats (
    day       TEXT    NOT NULL, -- YYYY-MM-DD (UTC)
    kind      TEXT    NOT NULL, -- movie, tv, music, book or other
    protocol  TEXT    NOT NULL, -- usenet or torrent
    grabs     INTEGER NOT NULL DEFAULT 0,
    completed INTEGER NOT NULL DEFAULT 0,
    failed    INTEGER NOT NULL DEFAULT 0,
    bytes     INTEGER NOT NULL DEFAULT 0, -- of the completed ones
    PRIMARY KEY (day, kind, protocol)
) WITHOUT ROWID;

-- Filled once from the downloads still on the list. Running it again changes
-- nothing: it only fills an empty table.
INSERT INTO download_stats (day, kind, protocol, grabs, completed, failed, bytes)
SELECT day, kind, protocol, SUM(grabs), SUM(completed), SUM(failed), SUM(bytes) FROM (
    SELECT substr(added_at, 1, 10) AS day,
           CASE WHEN movie_id IS NOT NULL THEN 'movie' WHEN series_id IS NOT NULL THEN 'tv'
                WHEN album_id IS NOT NULL THEN 'music' WHEN book_id IS NOT NULL THEN 'book' ELSE 'other' END AS kind,
           COALESCE(NULLIF(protocol, ''), 'usenet') AS protocol, 1 AS grabs, 0 AS completed, 0 AS failed, 0 AS bytes
      FROM download_queue
    UNION ALL
    SELECT substr(COALESCE(completed_at, added_at), 1, 10),
           CASE WHEN movie_id IS NOT NULL THEN 'movie' WHEN series_id IS NOT NULL THEN 'tv'
                WHEN album_id IS NOT NULL THEN 'music' WHEN book_id IS NOT NULL THEN 'book' ELSE 'other' END,
           COALESCE(NULLIF(protocol, ''), 'usenet'), 0, 1, 0, COALESCE(size_bytes, 0)
      FROM download_queue WHERE status = 'completed'
    UNION ALL
    SELECT substr(COALESCE(completed_at, added_at), 1, 10),
           CASE WHEN movie_id IS NOT NULL THEN 'movie' WHEN series_id IS NOT NULL THEN 'tv'
                WHEN album_id IS NOT NULL THEN 'music' WHEN book_id IS NOT NULL THEN 'book' ELSE 'other' END,
           COALESCE(NULLIF(protocol, ''), 'usenet'), 0, 0, 1, 0
      FROM download_queue WHERE status = 'failed'
    UNION ALL
    -- Days whose downloads were cleared from the list earlier, but whose
    -- totals the daily snapshots of the Statistics page still hold: what the
    -- list no longer explains is kept as "other" with no protocol.
    SELECT d.day, 'other', 'unknown', 0,
           MAX(d.completed - COALESCE(q.completed, 0), 0), MAX(d.failed - COALESCE(q.failed, 0), 0),
           MAX(d.download_bytes - COALESCE(q.bytes, 0), 0)
      FROM stats_daily d
      LEFT JOIN (SELECT substr(COALESCE(completed_at, added_at), 1, 10) AS day,
                        SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) AS completed,
                        SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) AS failed,
                        COALESCE(SUM(CASE WHEN status = 'completed' THEN size_bytes ELSE 0 END), 0) AS bytes
                   FROM download_queue GROUP BY 1) q ON q.day = d.day
     WHERE d.completed > COALESCE(q.completed, 0) OR d.failed > COALESCE(q.failed, 0) OR d.download_bytes > COALESCE(q.bytes, 0)
)
WHERE NOT EXISTS (SELECT 1 FROM download_stats) AND day <> ''
GROUP BY day, kind, protocol;

CREATE TRIGGER IF NOT EXISTS download_stats_grab AFTER INSERT ON download_queue
BEGIN
    INSERT INTO download_stats (day, kind, protocol, grabs)
    VALUES (substr(COALESCE(NULLIF(new.added_at, ''), strftime('%Y-%m-%dT%H:%M:%fZ', 'now')), 1, 10),
            CASE WHEN new.movie_id IS NOT NULL THEN 'movie' WHEN new.series_id IS NOT NULL THEN 'tv'
                 WHEN new.album_id IS NOT NULL THEN 'music' WHEN new.book_id IS NOT NULL THEN 'book' ELSE 'other' END,
            COALESCE(NULLIF(new.protocol, ''), 'usenet'), 1)
    ON CONFLICT (day, kind, protocol) DO UPDATE SET grabs = grabs + 1;
END;

CREATE TRIGGER IF NOT EXISTS download_stats_completed AFTER UPDATE OF status ON download_queue
WHEN new.status = 'completed' AND old.status <> 'completed'
BEGIN
    INSERT INTO download_stats (day, kind, protocol, completed, bytes)
    VALUES (substr(COALESCE(NULLIF(new.completed_at, ''), strftime('%Y-%m-%dT%H:%M:%fZ', 'now')), 1, 10),
            CASE WHEN new.movie_id IS NOT NULL THEN 'movie' WHEN new.series_id IS NOT NULL THEN 'tv'
                 WHEN new.album_id IS NOT NULL THEN 'music' WHEN new.book_id IS NOT NULL THEN 'book' ELSE 'other' END,
            COALESCE(NULLIF(new.protocol, ''), 'usenet'), 1, COALESCE(new.size_bytes, 0))
    ON CONFLICT (day, kind, protocol) DO UPDATE SET completed = completed + 1, bytes = bytes + excluded.bytes;
END;

CREATE TRIGGER IF NOT EXISTS download_stats_failed AFTER UPDATE OF status ON download_queue
WHEN new.status = 'failed' AND old.status <> 'failed'
BEGIN
    INSERT INTO download_stats (day, kind, protocol, failed)
    VALUES (substr(COALESCE(NULLIF(new.completed_at, ''), strftime('%Y-%m-%dT%H:%M:%fZ', 'now')), 1, 10),
            CASE WHEN new.movie_id IS NOT NULL THEN 'movie' WHEN new.series_id IS NOT NULL THEN 'tv'
                 WHEN new.album_id IS NOT NULL THEN 'music' WHEN new.book_id IS NOT NULL THEN 'book' ELSE 'other' END,
            COALESCE(NULLIF(new.protocol, ''), 'usenet'), 1)
    ON CONFLICT (day, kind, protocol) DO UPDATE SET failed = failed + 1;
END;
