-- The saved text of an article, as Markdown, and the images of its body.
-- One row per article; saving again replaces the text and its images.
CREATE TABLE article_contents (
    article_id     uuid        PRIMARY KEY,
    -- Repeated so the foreign keys guarantee content and article share an owner.
    user_id        uuid        NOT NULL,
    -- complete: the whole text; incomplete: the page said it is a paid
    -- preview; failed: the download failed and nothing was saved yet.
    status         text        NOT NULL CHECK (status IN ('complete', 'incomplete', 'failed')),
    source         text        NOT NULL CHECK (source IN ('fetched', 'uploaded_html', 'uploaded_markdown')),
    markdown       text        NOT NULL DEFAULT '' CHECK (octet_length(markdown) <= 5242880),
    words          integer     NOT NULL DEFAULT 0 CHECK (words >= 0),
    -- Why the last download failed, as a stable code; NULL when it did not.
    problem        text        CHECK (problem IS NULL OR char_length(problem) BETWEEN 1 AND 64),
    -- Body images that could not be downloaded or were not usable.
    missing_images integer     NOT NULL DEFAULT 0 CHECK (missing_images >= 0),
    saved_at       timestamptz NOT NULL DEFAULT now(),

    FOREIGN KEY (article_id, user_id) REFERENCES articles (id, user_id) ON DELETE CASCADE,
    CONSTRAINT article_contents_article_user_key UNIQUE (article_id, user_id)
);

CREATE TABLE article_images (
    id           uuid    PRIMARY KEY DEFAULT gen_random_uuid(),
    article_id   uuid    NOT NULL,
    user_id      uuid    NOT NULL,
    -- Order of first appearance in the text.
    position     integer NOT NULL CHECK (position >= 0),
    -- The address the Markdown refers to; the web app shows the stored copy.
    source_url   text    NOT NULL CHECK (char_length(source_url) BETWEEN 1 AND 4096),
    -- Raster formats only: SVG can carry scripts.
    content_type text    NOT NULL CHECK (content_type IN ('image/png', 'image/jpeg', 'image/gif', 'image/webp')),
    data         bytea   NOT NULL CHECK (octet_length(data) BETWEEN 1 AND 5242880),

    FOREIGN KEY (article_id, user_id) REFERENCES article_contents (article_id, user_id) ON DELETE CASCADE,
    CONSTRAINT article_images_article_position_key UNIQUE (article_id, position)
);
