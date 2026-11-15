-- When the reader finished or marked the article as read; NULL while unread.
-- Marking read is not an edit, so it does not change updated_at.
ALTER TABLE articles ADD COLUMN read_at timestamptz;
