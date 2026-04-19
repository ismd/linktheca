-- +goose Up
CREATE TABLE article_contents (
    id                   BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    url                  TEXT NOT NULL UNIQUE,
    canonical_url        TEXT,
    title                TEXT,
    byline               TEXT,
    excerpt              TEXT,
    text                 TEXT,
    html                 TEXT,
    lang                 TEXT,
    reading_time_seconds INT,
    fetched_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    fetch_error          TEXT
);

CREATE INDEX article_contents_fts_idx ON article_contents USING GIN (
    to_tsvector('simple', coalesce(title, '') || ' ' || coalesce(text, ''))
);

-- +goose Down
DROP TABLE article_contents;
