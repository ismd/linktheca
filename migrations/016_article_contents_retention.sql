-- Radar now downloads the page behind every finding into article_contents and
-- prunes those pages again once nobody needs them (see
-- radar.Store.PruneFindingContent). A pruned page must not take its findings
-- with it: the finding keeps its title, summary and embedding and just loses
-- the link to the text.
--
-- The indexes serve the reference checks of the pruning query and the
-- foreign keys Postgres verifies on every delete.

-- +goose Up
ALTER TABLE radar_findings DROP CONSTRAINT radar_findings_content_id_fkey;
ALTER TABLE radar_findings ADD CONSTRAINT radar_findings_content_id_fkey
    FOREIGN KEY (content_id) REFERENCES article_contents(id) ON DELETE SET NULL;

CREATE INDEX radar_findings_content_idx ON radar_findings (content_id);
CREATE INDEX library_items_content_idx ON library_items (content_id);
CREATE INDEX radar_topic_matches_finding_idx ON radar_topic_matches (finding_id);
CREATE INDEX article_contents_fetched_idx ON article_contents (fetched_at);

-- +goose Down
DROP INDEX article_contents_fetched_idx;
DROP INDEX radar_topic_matches_finding_idx;
DROP INDEX library_items_content_idx;
DROP INDEX radar_findings_content_idx;

ALTER TABLE radar_findings DROP CONSTRAINT radar_findings_content_id_fkey;
ALTER TABLE radar_findings ADD CONSTRAINT radar_findings_content_id_fkey
    FOREIGN KEY (content_id) REFERENCES article_contents(id);
