-- +goose Up
CREATE TABLE radar_feeds (
    id                     BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    url                    TEXT NOT NULL UNIQUE,
    kind                   TEXT NOT NULL DEFAULT 'rss'
                           CHECK (kind IN ('rss', 'atom')),
    title                  TEXT,
    last_fetched_at        TIMESTAMPTZ,
    last_error             TEXT,
    etag                   TEXT,
    last_modified          TEXT,
    fetch_interval_seconds INT NOT NULL DEFAULT 3600,
    is_active              BOOLEAN NOT NULL DEFAULT TRUE,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX radar_feeds_active_fetched_idx ON radar_feeds (is_active, last_fetched_at);

-- +goose Down
DROP TABLE radar_feeds;
