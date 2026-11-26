-- Matching moves from the cosine of topic and article to a calibrated score
-- (docs/superpowers/plans/2026-11-26-radar-hub-corrected-matching.md):
--
--   score = (q − c) · (d + t) / 2
--
-- where d is the article embedding, t the title embedding, q the topic
-- embedding and c the mean of a fixed set of reference topics. Subtracting c
-- discounts findings that are close to every topic, and the title keeps a
-- long article from drifting off what it is about.
--
-- Both sides are stored ready to multiply: match_embedding is (d + t) / 2 on
-- findings and q − c on topics. The application fills them in: new findings
-- and edited topics right away, existing rows through a periodic backfill.
-- Until a row has one it takes no part in matching or previews.
--
-- radar_topic_matches.similarity holds this score for matches made from here
-- on; older rows keep the cosine they were matched with.

-- +goose Up
ALTER TABLE radar_findings ADD COLUMN match_embedding vector(1024);
ALTER TABLE radar_topics ADD COLUMN match_embedding vector(1024);

-- Previews rank by the score, so the index follows it. Nothing orders by the
-- plain article embedding any more.
DROP INDEX radar_findings_embedding_hnsw;
CREATE INDEX radar_findings_match_embedding_hnsw
    ON radar_findings USING hnsw (match_embedding vector_ip_ops);

-- The score sits about 0.34 below the cosine it replaces: 0.18 is where 0.52
-- was. Topics nobody tuned move to the new default; tuned ones keep roughly
-- the same strictness relative to it.
ALTER TABLE radar_topics ALTER COLUMN match_threshold SET DEFAULT 0.18;
UPDATE radar_topics
SET match_threshold = CASE
        WHEN match_threshold = 0.52::real THEN 0.18
        ELSE GREATEST(match_threshold - 0.34, 0)
    END;

-- +goose Down
UPDATE radar_topics
SET match_threshold = CASE
        WHEN match_threshold = 0.18::real THEN 0.52
        ELSE LEAST(match_threshold + 0.34, 1)
    END;
ALTER TABLE radar_topics ALTER COLUMN match_threshold SET DEFAULT 0.52;

DROP INDEX radar_findings_match_embedding_hnsw;
CREATE INDEX radar_findings_embedding_hnsw
    ON radar_findings USING hnsw (embedding vector_cosine_ops);

ALTER TABLE radar_topics DROP COLUMN match_embedding;
ALTER TABLE radar_findings DROP COLUMN match_embedding;
