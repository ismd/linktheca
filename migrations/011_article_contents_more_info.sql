-- +goose Up
ALTER TABLE article_contents ADD COLUMN image_url TEXT;
ALTER TABLE article_contents ADD COLUMN favicon TEXT;
ALTER TABLE article_contents ADD COLUMN site_name TEXT;
ALTER TABLE article_contents ADD COLUMN published_time TIMESTAMPTZ;
ALTER TABLE article_contents ADD COLUMN modified_time TIMESTAMPTZ;

-- +goose Down
ALTER TABLE article_contents DROP COLUMN image_url;
ALTER TABLE article_contents DROP COLUMN favicon;
ALTER TABLE article_contents DROP COLUMN site_name;
ALTER TABLE article_contents DROP COLUMN published_time;
ALTER TABLE article_contents DROP COLUMN modified_time;
