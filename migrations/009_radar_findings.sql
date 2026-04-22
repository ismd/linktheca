-- +goose Up
CREATE TABLE radar_findings (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    feed_id       BIGINT NOT NULL REFERENCES radar_feeds(id) ON DELETE CASCADE,
    content_id    BIGINT REFERENCES article_contents(id),
    external_id   TEXT,
    url           TEXT NOT NULL,
    title         TEXT,
    summary       TEXT,
    embedding     vector(1024),
    published_at  TIMESTAMPTZ,
    discovered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (feed_id, external_id)
);
CREATE INDEX radar_findings_discovered_idx ON radar_findings (discovered_at DESC);
CREATE INDEX radar_findings_embedding_hnsw ON radar_findings USING hnsw (embedding vector_cosine_ops);

-- +goose Down
DROP TABLE radar_findings;
