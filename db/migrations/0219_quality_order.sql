-- A quality profile's own order of qualities and groups, best first, as JSON
-- (NULL: the built-in order). Existing profiles keep NULL and so behave exactly
-- as before.
ALTER TABLE quality_profiles ADD COLUMN quality_order TEXT;
