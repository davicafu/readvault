-- ReadVault initial schema (milestone 1).
-- Ownership is explicit: every article and tag belongs to a user. Until
-- authentication exists, a fixed development owner is seeded below.

CREATE TABLE users (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    email      text        UNIQUE CHECK (email IS NULL OR char_length(email) BETWEEN 3 AND 320),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Fixed development owner. The email stays NULL until authentication exists.
INSERT INTO users (id) VALUES ('00000000-0000-0000-0000-000000000001');

CREATE TABLE articles (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- URL exactly as imported, used for display and navigation.
    original_url    text        NOT NULL CHECK (char_length(original_url) BETWEEN 1 AND 2048),
    -- Normalized identity used for deduplication.
    canonical_url   text        NOT NULL CHECK (char_length(canonical_url) BETWEEN 1 AND 2048),
    title           text        NOT NULL DEFAULT '' CHECK (char_length(title) <= 500),
    -- Set when the user edits the title; imports never overwrite an edited title.
    title_edited_at timestamptz,
    -- Folder path in the source file, outermost first. Empty when unfiled.
    original_folder text[]      NOT NULL DEFAULT '{}',
    -- Date the bookmark was saved in the browser or CSV, when known.
    bookmarked_at   timestamptz,
    search          tsvector    GENERATED ALWAYS AS (
                        to_tsvector('simple', title || ' ' || canonical_url)
                    ) STORED,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT articles_user_canonical_url_key UNIQUE (user_id, canonical_url),
    -- Lets article_tags check that article and tag share an owner.
    CONSTRAINT articles_id_user_key UNIQUE (id, user_id)
);

-- Library pagination: newest first, and by title.
CREATE INDEX articles_user_created_idx ON articles (user_id, created_at DESC, id DESC);
CREATE INDEX articles_user_title_idx ON articles (user_id, lower(title), id);
CREATE INDEX articles_search_idx ON articles USING gin (search);

CREATE TABLE tags (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name       text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 64),
    slug       text        NOT NULL CHECK (char_length(slug) BETWEEN 1 AND 64 AND slug = lower(slug)),
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT tags_user_slug_key UNIQUE (user_id, slug),
    CONSTRAINT tags_id_user_key UNIQUE (id, user_id)
);

CREATE TABLE article_tags (
    -- Repeated here so the foreign keys guarantee article and tag share an owner.
    user_id    uuid        NOT NULL,
    article_id uuid        NOT NULL,
    tag_id     uuid        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (article_id, tag_id),
    FOREIGN KEY (article_id, user_id) REFERENCES articles (id, user_id) ON DELETE CASCADE,
    FOREIGN KEY (tag_id, user_id) REFERENCES tags (id, user_id) ON DELETE CASCADE
);

CREATE INDEX article_tags_tag_idx ON article_tags (tag_id);
