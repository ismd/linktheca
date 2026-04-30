-- +goose Up
CREATE TABLE radar_topics (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id         BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL,
    embedding       vector(1024),
    match_threshold REAL NOT NULL DEFAULT 0.75,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX radar_topics_user_active_idx ON radar_topics (user_id) WHERE is_active;

-- +goose Down
DROP TABLE radar_topics;
