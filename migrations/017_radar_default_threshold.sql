-- Findings are now embedded from the opening of the article rather than the
-- feed's title and summary (radar_full_text matching, see
-- docs/superpowers/specs/2026-11-20-radar-full-text-matching-design.md).
-- The noise floor stayed where it was, but articles that already had a good
-- summary score a little lower against their topic, so 0.55 now misses
-- matches it used to catch. 0.52 came out best on a labeled sample.
--
-- Topics nobody tuned move with the default; tuned ones are left alone.

-- +goose Up
ALTER TABLE radar_topics ALTER COLUMN match_threshold SET DEFAULT 0.52;
UPDATE radar_topics SET match_threshold = 0.52 WHERE match_threshold = 0.55::real;

-- +goose Down
ALTER TABLE radar_topics ALTER COLUMN match_threshold SET DEFAULT 0.55;
UPDATE radar_topics SET match_threshold = 0.55 WHERE match_threshold = 0.52::real;
