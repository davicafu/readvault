-- When the user last replaced an article's tags. Imports never add tags to
-- such an article, so a tag the user removed does not come back.
ALTER TABLE articles ADD COLUMN tags_edited_at timestamptz;
