-- Extra category numbers an indexer is also searched in, per kind of media, for
-- the numbers it files releases under besides the standard ones (105000 for TV,
-- 7120 for German books). A JSON object such as {"tv":[105000],"books":[7120]};
-- empty for every indexer that exists, which are searched as before.
ALTER TABLE indexers ADD COLUMN extra_categories TEXT NOT NULL DEFAULT '';
