-- The collections are the user's own after the up migration: they may have
-- been renamed, filled or emptied since. Rolling back keeps them; running up
-- again creates nothing twice.
SELECT 1;
