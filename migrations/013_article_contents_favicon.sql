-- +goose Up
ALTER TABLE article_contents RENAME COLUMN favicon TO favicon_url;
ALTER TABLE article_contents ADD COLUMN favicon TEXT;

-- +goose Down
ALTER TABLE article_contents DROP COLUMN favicon;
ALTER TABLE article_contents RENAME COLUMN favicon_url TO favicon;