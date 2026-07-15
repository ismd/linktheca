-- +goose Up
ALTER TABLE article_contents ADD COLUMN image TEXT;

-- +goose Down
ALTER TABLE article_contents DROP COLUMN image;
