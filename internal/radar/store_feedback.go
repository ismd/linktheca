package radar

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
)

// TopicLearnInput is what learning a topic starts from besides its marks.
type TopicLearnInput struct {
	ID             int64
	Description    string
	MatchThreshold float32
	// Embedding is nil while the topic has not been embedded.
	Embedding *pgvector.Vector
}

// GetTopicLearnInput reads a topic's description, threshold and stored
// embedding.
func (s *Store) GetTopicLearnInput(ctx context.Context, topicID int64) (*TopicLearnInput, error) {
	var t TopicLearnInput
	err := s.db.QueryRow(ctx,
		`SELECT id, description, match_threshold, embedding FROM radar_topics WHERE id = $1`, topicID,
	).Scan(&t.ID, &t.Description, &t.MatchThreshold, &t.Embedding)
	if err != nil {
		return nil, wrapPgError(err)
	}

	return &t, nil
}

// ListTopicExamples returns every finding marked for a topic that has a match
// vector, whether or not the user still subscribes to its feed: what they
// taught the topic stays.
func (s *Store) ListTopicExamples(ctx context.Context, topicID int64) ([]Example, error) {
	rows, err := s.db.Query(ctx, `
		SELECT fb.finding_id, f.match_embedding, fb.verdict = 'relevant'
		FROM radar_topic_feedback fb
		JOIN radar_findings f ON f.id = fb.finding_id
		WHERE fb.topic_id = $1 AND f.match_embedding IS NOT NULL
		ORDER BY fb.finding_id`, topicID)
	if err != nil {
		return nil, fmt.Errorf("list topic examples: %w", err)
	}

	return collectExamples(rows)
}

// FindingExamples turns marks the client holds for a topic not saved yet into
// examples, keeping only findings the user subscribes to that have a match
// vector.
func (s *Store) FindingExamples(ctx context.Context, userID int64, feedback []TopicFeedback) ([]Example, error) {
	if len(feedback) == 0 {
		return nil, nil
	}

	ids, verdicts := splitFeedback(feedback)
	rows, err := s.db.Query(ctx, `
		SELECT f.id, f.match_embedding, m.verdict = 'relevant'
		FROM unnest($2::bigint[], $3::text[]) AS m(finding_id, verdict)
		JOIN radar_findings f ON f.id = m.finding_id
		JOIN radar_feed_subscriptions rfs ON rfs.feed_id = f.feed_id AND rfs.user_id = $1
		WHERE f.match_embedding IS NOT NULL
		ORDER BY f.id`, userID, ids, verdicts)
	if err != nil {
		return nil, fmt.Errorf("finding examples: %w", err)
	}

	return collectExamples(rows)
}

func collectExamples(rows pgx.Rows) ([]Example, error) {
	defer rows.Close()

	var out []Example
	for rows.Next() {
		var (
			e   Example
			vec pgvector.Vector
		)
		if err := rows.Scan(&e.FindingID, &vec, &e.Relevant); err != nil {
			return nil, fmt.Errorf("scan example: %w", err)
		}
		e.Vec = vec.Slice()
		out = append(out, e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}

	return out, nil
}

func splitFeedback(feedback []TopicFeedback) ([]int64, []string) {
	ids := make([]int64, len(feedback))
	verdicts := make([]string, len(feedback))
	for i, f := range feedback {
		ids[i], verdicts[i] = f.FindingID, f.Verdict
	}

	return ids, verdicts
}

// SetTopicFeedback records the user's verdict on one finding for one of their
// topics. ErrNotFound when the topic is not theirs or the finding is not in a
// feed they subscribe to.
func (s *Store) SetTopicFeedback(ctx context.Context, userID, topicID, findingID int64, verdict string) error {
	cmd, err := s.db.Exec(ctx, `
		INSERT INTO radar_topic_feedback (topic_id, finding_id, verdict)
		SELECT t.id, f.id, $4::text
		FROM radar_topics t
		JOIN radar_findings f ON f.id = $3
		JOIN radar_feed_subscriptions rfs ON rfs.feed_id = f.feed_id AND rfs.user_id = t.user_id
		WHERE t.id = $2 AND t.user_id = $1
		ON CONFLICT (topic_id, finding_id)
		DO UPDATE SET verdict = EXCLUDED.verdict, created_at = now()`,
		userID, topicID, findingID, verdict)
	if err != nil {
		return fmt.Errorf("set topic feedback: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// ReplaceTopicFeedback makes feedback the user's marks for a topic. Marks on
// findings in feeds they no longer subscribe to cannot be shown to them, so
// they are neither listed nor replaced; findings outside their feeds are
// skipped.
func (s *Store) ReplaceTopicFeedback(ctx context.Context, userID, topicID int64, feedback []TopicFeedback) error {
	ids, verdicts := splitFeedback(feedback)

	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			DELETE FROM radar_topic_feedback fb
			USING radar_topics t, radar_findings f, radar_feed_subscriptions rfs
			WHERE fb.topic_id = $2
			  AND t.id = fb.topic_id AND t.user_id = $1
			  AND f.id = fb.finding_id
			  AND rfs.feed_id = f.feed_id AND rfs.user_id = $1
			  AND fb.finding_id <> ALL($3)`,
			userID, topicID, ids); err != nil {
			return fmt.Errorf("delete topic feedback: %w", err)
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO radar_topic_feedback (topic_id, finding_id, verdict)
			SELECT t.id, f.id, m.verdict
			FROM unnest($3::bigint[], $4::text[]) AS m(finding_id, verdict)
			JOIN radar_topics t ON t.id = $2 AND t.user_id = $1
			JOIN radar_findings f ON f.id = m.finding_id
			JOIN radar_feed_subscriptions rfs ON rfs.feed_id = f.feed_id AND rfs.user_id = $1
			ON CONFLICT (topic_id, finding_id)
			DO UPDATE SET verdict = EXCLUDED.verdict,
			              created_at = CASE WHEN radar_topic_feedback.verdict = EXCLUDED.verdict
			                                THEN radar_topic_feedback.created_at ELSE now() END`,
			userID, topicID, ids, verdicts); err != nil {
			return fmt.Errorf("insert topic feedback: %w", err)
		}

		return nil
	})
}

// DeleteTopicFeedback takes back the user's verdict on a finding for one of
// their topics. ErrNotFound when there was none.
func (s *Store) DeleteTopicFeedback(ctx context.Context, userID, topicID, findingID int64) error {
	cmd, err := s.db.Exec(ctx, `
		DELETE FROM radar_topic_feedback fb
		USING radar_topics t
		WHERE fb.topic_id = $2 AND fb.finding_id = $3
		  AND t.id = fb.topic_id AND t.user_id = $1`,
		userID, topicID, findingID)
	if err != nil {
		return fmt.Errorf("delete topic feedback: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// ListTopicFeedback returns the user's marks for one of their topics on
// findings they can still see, newest first.
func (s *Store) ListTopicFeedback(ctx context.Context, userID, topicID int64) ([]TopicFeedbackView, error) {
	rows, err := s.db.Query(ctx, `
		SELECT fb.verdict, fb.created_at,
		       f.id, f.feed_id, fd.title AS feed_title,
		       f.url, f.title, COALESCE(f.summary, ac.excerpt),
		       f.published_at, f.discovered_at
		FROM radar_topic_feedback fb
		JOIN radar_topics t ON t.id = fb.topic_id AND t.user_id = $1
		JOIN radar_findings f ON f.id = fb.finding_id
		JOIN radar_feeds fd ON fd.id = f.feed_id
		JOIN radar_feed_subscriptions rfs ON rfs.feed_id = f.feed_id AND rfs.user_id = $1
		LEFT JOIN article_contents ac ON ac.id = f.content_id
		WHERE fb.topic_id = $2
		ORDER BY fb.created_at DESC, f.id DESC`, userID, topicID)
	if err != nil {
		return nil, fmt.Errorf("list topic feedback: %w", err)
	}
	defer rows.Close()

	items := []TopicFeedbackView{}
	for rows.Next() {
		var v TopicFeedbackView
		if err := rows.Scan(
			&v.Verdict, &v.CreatedAt,
			&v.Finding.ID, &v.Finding.FeedID, &v.Finding.FeedTitle,
			&v.Finding.URL, &v.Finding.Title, &v.Finding.Summary,
			&v.Finding.PublishedAt, &v.Finding.DiscoveredAt,
		); err != nil {
			return nil, fmt.Errorf("scan topic feedback: %w", err)
		}
		items = append(items, v)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}

	return items, nil
}

// StreamMean returns the mean match vector of the latest findings, or nil
// when there are none yet.
func (s *Store) StreamMean(ctx context.Context, sample int) ([]float32, error) {
	var mean *pgvector.Vector
	err := s.db.QueryRow(ctx, `
		SELECT avg(match_embedding)
		FROM (
			SELECT match_embedding FROM radar_findings
			WHERE match_embedding IS NOT NULL
			ORDER BY discovered_at DESC
			LIMIT $1
		) latest`, sample).Scan(&mean)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("stream mean: %w", err)
	}

	if mean == nil {
		return nil, nil
	}

	return mean.Slice(), nil
}
