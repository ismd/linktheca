-- +goose Up
ALTER TABLE radar_feeds
  ADD COLUMN owner_user_id BIGINT NULL REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE radar_feeds DROP CONSTRAINT radar_feeds_url_key;

CREATE UNIQUE INDEX radar_feeds_global_url_idx
  ON radar_feeds (url) WHERE owner_user_id IS NULL;
CREATE UNIQUE INDEX radar_feeds_owner_url_idx
  ON radar_feeds (url, owner_user_id) WHERE owner_user_id IS NOT NULL;
CREATE INDEX radar_feeds_owner_idx
  ON radar_feeds (owner_user_id) WHERE owner_user_id IS NOT NULL;

-- +goose Down
DROP INDEX radar_feeds_owner_idx;
DROP INDEX radar_feeds_owner_url_idx;
DROP INDEX radar_feeds_global_url_idx;
ALTER TABLE radar_feeds ADD CONSTRAINT radar_feeds_url_key UNIQUE (url);
ALTER TABLE radar_feeds DROP COLUMN owner_user_id;
