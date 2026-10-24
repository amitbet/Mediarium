-- The score the TRaSH Guides gave a custom format when it was last read from
-- them (NULL: the format was pasted or made by hand and is never updated from
-- the guides). Formats that already have a TRaSH id came from the guides, so
-- they start at 0, which means "score not known": their conditions follow the
-- guides from now on but no profile score is touched.
ALTER TABLE custom_formats ADD COLUMN trash_score INTEGER;
UPDATE custom_formats SET trash_score = 0 WHERE trash_id != '';
