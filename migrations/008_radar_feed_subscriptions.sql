-- +goose Up
CREATE TABLE radar_feed_subscriptions (
    user_id    INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    feed_id    BIGINT NOT NULL REFERENCES radar_feeds(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, feed_id)
);

-- +goose Down
DROP TABLE radar_feed_subscriptions;
