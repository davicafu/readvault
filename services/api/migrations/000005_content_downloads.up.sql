-- Articles waiting for their text to be downloaded in the background. A
-- worker claims a row for a while (claimed_until), downloads the article and
-- deletes the row; a claim that runs out (the API stopped) is claimed again,
-- up to a few attempts.
CREATE TABLE content_downloads (
    article_id    uuid        PRIMARY KEY,
    user_id       uuid        NOT NULL,
    queued_at     timestamptz NOT NULL DEFAULT now(),
    attempts      integer     NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    claimed_until timestamptz,

    FOREIGN KEY (article_id, user_id) REFERENCES articles (id, user_id) ON DELETE CASCADE
);

CREATE INDEX content_downloads_queued_idx ON content_downloads (queued_at, article_id);
