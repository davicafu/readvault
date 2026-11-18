-- A downloaded PDF is kept as it is, for the browser to show or save: it
-- has no Markdown text. NULL for a text saved from a web page or a file.
ALTER TABLE article_contents
    ADD COLUMN pdf bytea CHECK (pdf IS NULL OR octet_length(pdf) BETWEEN 1 AND 52428800);
