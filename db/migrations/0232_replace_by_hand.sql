-- A release a person picked by hand (Choose a release) can replace the file the
-- title already has, after the person agreed to it. replace_by_hand remembers
-- that choice for the download, also after a restart; 0 for everything else
-- (automatic grabs, API users, older clients), which behave as before.
ALTER TABLE download_queue ADD COLUMN replace_by_hand INTEGER NOT NULL DEFAULT 0;
-- A finished download whose file was not imported because the title's own file
-- was kept: which file was kept, and what became of the downloaded data
-- ("deleted", "seeding" or "client"; empty when nothing was kept).
ALTER TABLE download_queue ADD COLUMN kept_file TEXT NOT NULL DEFAULT '';
ALTER TABLE download_queue ADD COLUMN kept_data TEXT NOT NULL DEFAULT '';
