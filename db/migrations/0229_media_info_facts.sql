-- The Library lists read each file's short facts (codecs, languages, length).
-- They sat behind the full details JSON in the row, so every read had to walk
-- the JSON's overflow pages: on a library of tens of thousands of files that
-- was most of the time a list took. The length now has a column of its own,
-- and one index holds every short fact, so the lists never touch the JSON.
ALTER TABLE media_info ADD COLUMN duration REAL NOT NULL DEFAULT 0;

UPDATE media_info SET duration = COALESCE(CAST(json_extract(info, '$.duration') AS REAL), 0) WHERE failed = 0 AND json_valid(info);

CREATE INDEX IF NOT EXISTS media_info_facts ON media_info (path, failed, vcodec, hdr, res, acodecs, alangs, slangs, duration);
