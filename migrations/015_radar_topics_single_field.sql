-- Topics are now described by a single field: the description is the only
-- text that gets embedded, and the name is a display label. Topics created
-- before this kept part of their meaning in the name ("Rust" + "new releases
-- and ecosystem news"), so fold the name into the description wherever the
-- description does not mention it already. For those rows the embedded text
-- stays exactly what it was ("Name: description"); the rest need
-- cmd/reembed-topics.

-- +goose Up
UPDATE radar_topics
SET description = name || ': ' || description,
    updated_at  = now()
WHERE strpos(lower(description), lower(name)) = 0;

-- +goose Down
UPDATE radar_topics
SET description = substr(description, length(name) + 3)
WHERE starts_with(description, name || ': ');
