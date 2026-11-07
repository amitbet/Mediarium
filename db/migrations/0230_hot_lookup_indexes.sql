-- Lookups that had no index of their own and read the whole table:
-- albums by release-group id (every Discover cover and music Discover page
-- asks for dozens at once) and the queue rows of one movie, show, album or
-- book (checked on every grab, and by the cascades when a title is removed).
-- Indexes only: no rows or columns change.
CREATE INDEX IF NOT EXISTS idx_albums_mbid ON albums (mbid);
CREATE INDEX IF NOT EXISTS idx_download_queue_movie ON download_queue (movie_id) WHERE movie_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_download_queue_episode ON download_queue (series_id, season, episode) WHERE series_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_download_queue_album ON download_queue (album_id) WHERE album_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_download_queue_book ON download_queue (book_id, book_format) WHERE book_id IS NOT NULL;
