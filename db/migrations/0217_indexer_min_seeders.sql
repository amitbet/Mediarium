-- The fewest seeders a torrent release from an indexer needs before Mediarium
-- grabs it by itself. 0 means off, which is how every existing indexer starts.
ALTER TABLE indexers ADD COLUMN min_seeders INTEGER NOT NULL DEFAULT 0;
