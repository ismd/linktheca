-- Topics learn from the user's marks on findings
-- (docs/superpowers/plans/2026-11-27-radar-topic-feedback.md): ✓ "about this
-- topic" moves the topic towards the finding, ✗ "not about this topic" hides
-- the finding and raises the topic's cutoff above it.
--
-- radar_topics keeps what the user set and what was learned apart:
-- match_threshold stays the base the user chooses, while match_embedding,
-- match_offset and match_cutoff are recomputed from the description and the
-- marks whenever either changes. A finding matches when
--
--   -(match_embedding <#> finding.match_embedding) - match_offset >= match_cutoff
--
-- With no marks, offset is 0 and the cutoff is the threshold, so existing
-- topics match exactly as before.

-- +goose Up
CREATE TABLE radar_topic_feedback (
    topic_id   BIGINT NOT NULL REFERENCES radar_topics(id) ON DELETE CASCADE,
    finding_id BIGINT NOT NULL REFERENCES radar_findings(id) ON DELETE CASCADE,
    verdict    TEXT NOT NULL CHECK (verdict IN ('relevant', 'irrelevant')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (topic_id, finding_id)
);

ALTER TABLE radar_topics
    ADD COLUMN match_offset real NOT NULL DEFAULT 0,
    ADD COLUMN match_cutoff real NOT NULL DEFAULT 0.18;
UPDATE radar_topics SET match_cutoff = match_threshold;

-- +goose Down
-- Topics that learned from ✓ keep their moved match vectors; run
-- reembed-topics after rolling back to match on descriptions alone again.
ALTER TABLE radar_topics
    DROP COLUMN match_cutoff,
    DROP COLUMN match_offset;
DROP TABLE radar_topic_feedback;
