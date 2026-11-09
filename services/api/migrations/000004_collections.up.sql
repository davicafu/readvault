-- Collections group articles. An article is in at most one collection;
-- tags stay independent of collections.
CREATE TABLE collections (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name       text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 64),
    -- Per-user identity, derived from the name the same way as tag slugs.
    slug       text        NOT NULL CHECK (char_length(slug) BETWEEN 1 AND 64 AND slug = lower(slug)),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT collections_user_slug_key UNIQUE (user_id, slug),
    -- Lets articles check that article and collection share an owner.
    CONSTRAINT collections_id_user_key UNIQUE (id, user_id)
);

ALTER TABLE articles ADD COLUMN collection_id uuid;

-- Deleting a collection leaves its articles without one; it never deletes
-- them. Only collection_id is cleared: user_id stays.
ALTER TABLE articles ADD CONSTRAINT articles_collection_fkey
    FOREIGN KEY (collection_id, user_id) REFERENCES collections (id, user_id)
    ON DELETE SET NULL (collection_id);

CREATE INDEX articles_collection_idx ON articles (collection_id) WHERE collection_id IS NOT NULL;
