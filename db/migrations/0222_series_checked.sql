-- When a show's TMDB details (status, episodes) were last read, so the
-- scheduled refresh can look at finished shows rarely and running ones daily.
ALTER TABLE series ADD COLUMN status_checked_at TEXT NOT NULL DEFAULT '';
