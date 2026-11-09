DROP INDEX IF EXISTS articles_collection_idx;
ALTER TABLE articles DROP CONSTRAINT IF EXISTS articles_collection_fkey;
ALTER TABLE articles DROP COLUMN IF EXISTS collection_id;
DROP TABLE IF EXISTS collections;
