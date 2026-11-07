-- Which accounts hold their role because a sign-in group said so.
-- group_role is empty for every account today (nothing changes), "viewer" when the
-- Viewer group of single sign-on or the directory made the account a viewer.
-- A role an administrator sets by hand clears it, so a group never takes back
-- something that was given by hand.
ALTER TABLE users ADD COLUMN group_role TEXT NOT NULL DEFAULT '';
