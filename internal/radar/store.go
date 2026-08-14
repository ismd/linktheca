package radar

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

func (s *Store) CreateTopic(ctx context.Context, p CreateTopicParams) (*Topic, error) {
	row := s.db.QueryRow(ctx, `
		INSERT INTO radar_topics (user_id, name, description, match_threshold)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, name, description, match_threshold, is_active,
		          embedding IS NOT NULL, created_at, updated_at
	`, p.UserID, p.Name, p.Description, p.MatchThreshold)

	var t Topic
	if err := row.Scan(&t.ID, &t.UserID, &t.Name, &t.Description,
		&t.MatchThreshold, &t.IsActive, &t.HasEmbedding, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, fmt.Errorf("create topic: %w", err)
	}

	return &t, nil
}

func (s *Store) UpdateTopicEmbedding(ctx context.Context, topicID int64, vec pgvector.Vector) error {
	cmd, err := s.db.Exec(ctx,
		`UPDATE radar_topics SET embedding=$1, updated_at=now() WHERE id=$2`,
		vec, topicID)
	if err != nil {
		return fmt.Errorf("update topic embedding: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// wrapPgError converts known Postgres errors into package-level sentinels.
func wrapPgError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique violation
			return ErrDuplicate
		case "23503": // foreign key violation
			return ErrFeedNotFound
		}
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}

	return err
}

func (s *Store) AddFeed(ctx context.Context, p AddFeedParams) (*Feed, error) {
	row := s.db.QueryRow(ctx, `
		INSERT INTO radar_feeds (url, kind, fetch_interval_seconds)
		VALUES ($1, $2, $3)
		RETURNING id, url, kind, title, fetch_interval_seconds, is_active,
		          last_fetched_at, last_error, created_at
	`, p.URL, p.Kind, p.FetchIntervalSeconds)

	var f Feed
	if err := row.Scan(&f.ID, &f.URL, &f.Kind, &f.Title,
		&f.FetchIntervalSeconds, &f.IsActive,
		&f.LastFetchedAt, &f.LastError, &f.CreatedAt); err != nil {
		return nil, wrapPgError(err)
	}

	return &f, nil
}

func (s *Store) Subscribe(ctx context.Context, userID, feedID int64) (*Subscription, error) {
	row := s.db.QueryRow(ctx, `
		INSERT INTO radar_feed_subscriptions (user_id, feed_id)
		VALUES ($1, $2)
		ON CONFLICT (user_id, feed_id) DO UPDATE SET created_at = radar_feed_subscriptions.created_at
		RETURNING user_id, feed_id, created_at
	`, userID, feedID)

	var sub Subscription
	if err := row.Scan(&sub.UserID, &sub.FeedID, &sub.CreatedAt); err != nil {
		return nil, wrapPgError(err)
	}

	return &sub, nil
}

// ListDueFeeds returns IDs of active feeds whose next-fetch moment has passed.
func (s *Store) ListDueFeeds(ctx context.Context, limit int) ([]int64, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id FROM radar_feeds
		WHERE is_active
		  AND (last_fetched_at IS NULL
		       OR last_fetched_at + fetch_interval_seconds * interval '1 second' < now())
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list due feeds: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	return ids, rows.Err()
}

type FeedFetchState struct {
	URL          string
	Etag         *string
	LastModified *string
}

func (s *Store) GetFeedForFetch(ctx context.Context, feedID int64) (*FeedFetchState, error) {
	row := s.db.QueryRow(ctx,
		`SELECT url, etag, last_modified FROM radar_feeds WHERE id=$1`, feedID)
	var st FeedFetchState
	if err := row.Scan(&st.URL, &st.Etag, &st.LastModified); err != nil {
		return nil, wrapPgError(err)
	}

	return &st, nil
}

func (s *Store) MarkFeedFetched(ctx context.Context, feedID int64, etag, lastModified *string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE radar_feeds
		SET last_fetched_at = now(), etag = $1, last_modified = $2, last_error = NULL
		WHERE id = $3
	`, etag, lastModified, feedID)

	return err
}

func (s *Store) MarkFeedError(ctx context.Context, feedID int64, errMsg string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE radar_feeds SET last_fetched_at = now(), last_error = $1 WHERE id = $2
	`, errMsg, feedID)

	return err
}

// UpsertFinding inserts a finding; returns (finding, created=true) if new, else (existing, false).
func (s *Store) UpsertFinding(ctx context.Context, p FindingUpsert) (*Finding, bool, error) {
	row := s.db.QueryRow(ctx, `
		INSERT INTO radar_findings (feed_id, external_id, url, title, summary, published_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (feed_id, external_id) DO NOTHING
		RETURNING id, feed_id, content_id, external_id, url, title, summary,
		          published_at, discovered_at, embedding IS NOT NULL
	`, p.FeedID, p.ExternalID, p.URL, p.Title, p.Summary, p.PublishedAt)

	var f Finding
	err := row.Scan(&f.ID, &f.FeedID, &f.ContentID, &f.ExternalID, &f.URL,
		&f.Title, &f.Summary, &f.PublishedAt, &f.DiscoveredAt, &f.HasEmbedding)
	if err == nil {
		return &f, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, fmt.Errorf("upsert finding: %w", err)
	}

	// Conflict path — fetch the existing row.
	existing, err := s.GetFindingByExternalID(ctx, p.FeedID, p.ExternalID)
	if err != nil {
		return nil, false, err
	}

	return existing, false, nil
}

func (s *Store) GetFindingByExternalID(ctx context.Context, feedID int64, externalID *string) (*Finding, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, feed_id, content_id, external_id, url, title, summary,
		       published_at, discovered_at, embedding IS NOT NULL
		FROM radar_findings
		WHERE feed_id = $1 AND external_id IS NOT DISTINCT FROM $2
	`, feedID, externalID)

	var f Finding
	if err := row.Scan(&f.ID, &f.FeedID, &f.ContentID, &f.ExternalID, &f.URL,
		&f.Title, &f.Summary, &f.PublishedAt, &f.DiscoveredAt, &f.HasEmbedding); err != nil {
		return nil, wrapPgError(err)
	}

	return &f, nil
}

func (s *Store) UpdateFindingEmbedding(ctx context.Context, findingID int64, vec pgvector.Vector) error {
	cmd, err := s.db.Exec(ctx,
		`UPDATE radar_findings SET embedding = $1 WHERE id = $2`, vec, findingID)

	if err != nil {
		return fmt.Errorf("update finding embedding: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// MatchFindingToTopics inserts matches for all subscribed+active topics of
// the finding's feed where cosine similarity ≥ topic.match_threshold.
// Returns the number of matches inserted (existing rows are not counted).
func (s *Store) MatchFindingToTopics(ctx context.Context, findingID int64) (int64, error) {
	cmd, err := s.db.Exec(ctx, `
		INSERT INTO radar_topic_matches (topic_id, finding_id, similarity, state)
		SELECT rt.id,
		       $1,
		       1 - (rt.embedding <=> f.embedding) AS similarity,
		       'new'
		FROM radar_topics rt
		JOIN radar_feed_subscriptions rfs ON rfs.user_id = rt.user_id
		JOIN radar_findings f ON f.id = $1
		WHERE rfs.feed_id = f.feed_id
		  AND rt.is_active
		  AND rt.embedding IS NOT NULL
		  AND f.embedding IS NOT NULL
		  AND 1 - (rt.embedding <=> f.embedding) >= rt.match_threshold
		ON CONFLICT (topic_id, finding_id) DO NOTHING
	`, findingID)

	if err != nil {
		return 0, fmt.Errorf("match finding: %w", err)
	}

	return cmd.RowsAffected(), nil
}

// PreviewFindings scores the user's reachable findings against a probe vector
// and returns the best `limit`, unfiltered by any threshold — the caller draws
// the cutoff line. The subscription join mirrors MatchFindingToTopics, so a
// preview can only promise findings the matcher would actually have seen.
func (s *Store) PreviewFindings(ctx context.Context, userID int64, vec pgvector.Vector, limit int) ([]PreviewMatch, error) {
	rows, err := s.db.Query(ctx, `
		SELECT 1 - (f.embedding <=> $2) AS similarity,
		       f.id, f.feed_id, fd.title AS feed_title,
		       f.url, f.title, f.summary,
		       f.published_at, f.discovered_at
		FROM radar_findings f
		JOIN radar_feeds fd ON fd.id = f.feed_id
		JOIN radar_feed_subscriptions rfs
		  ON rfs.feed_id = f.feed_id AND rfs.user_id = $1
		WHERE f.embedding IS NOT NULL
		ORDER BY f.embedding <=> $2
		LIMIT $3`, userID, vec, limit)
	if err != nil {
		return nil, fmt.Errorf("preview findings: %w", err)
	}
	defer rows.Close()

	items := []PreviewMatch{}
	for rows.Next() {
		var p PreviewMatch
		if err := rows.Scan(
			&p.Similarity,
			&p.Finding.ID, &p.Finding.FeedID, &p.Finding.FeedTitle,
			&p.Finding.URL, &p.Finding.Title, &p.Finding.Summary,
			&p.Finding.PublishedAt, &p.Finding.DiscoveredAt,
		); err != nil {
			return nil, fmt.Errorf("scan preview finding: %w", err)
		}
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return items, nil
}

type FindingForEmbed struct {
	ID           int64
	Title        *string
	Summary      *string
	HasEmbedding bool
}

func (s *Store) GetFindingForEmbed(ctx context.Context, findingID int64) (*FindingForEmbed, error) {
	row := s.db.QueryRow(ctx,
		`SELECT id, title, summary, embedding IS NOT NULL
		 FROM radar_findings WHERE id = $1`, findingID)

	var f FindingForEmbed
	if err := row.Scan(&f.ID, &f.Title, &f.Summary, &f.HasEmbedding); err != nil {
		return nil, wrapPgError(err)
	}

	return &f, nil
}

const topicsWithStatsSQL = `
SELECT
  t.id, t.user_id, t.name, t.description,
  t.match_threshold, t.is_active,
  t.embedding IS NOT NULL AS has_embedding,
  t.created_at, t.updated_at,
  COALESCE(m.new_count, 0)    AS new_count,
  COALESCE(m.total_count, 0)  AS total_count,
  COALESCE(m.source_count, 0) AS source_count,
  m.last_match_at
FROM radar_topics t
LEFT JOIN LATERAL (
  SELECT
    COUNT(*) FILTER (WHERE state = 'new') AS new_count,
    COUNT(*)                              AS total_count,
    COUNT(DISTINCT f.feed_id)             AS source_count,
    MAX(matched_at)                       AS last_match_at
  FROM radar_topic_matches m
  JOIN radar_findings f ON f.id = m.finding_id
  WHERE m.topic_id = t.id
) m ON true
WHERE t.user_id = $1`

func scanTopicWithStats(row pgx.Row) (*TopicWithStats, error) {
	var t TopicWithStats
	if err := row.Scan(
		&t.ID, &t.UserID, &t.Name, &t.Description,
		&t.MatchThreshold, &t.IsActive, &t.HasEmbedding,
		&t.CreatedAt, &t.UpdatedAt,
		&t.Stats.NewCount, &t.Stats.TotalCount, &t.Stats.SourceCount, &t.Stats.LastMatchAt,
	); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) ListTopicsWithStats(ctx context.Context, userID int64) ([]TopicWithStats, error) {
	rows, err := s.db.Query(ctx,
		topicsWithStatsSQL+` ORDER BY t.is_active DESC, t.created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list topics with stats: %w", err)
	}
	defer rows.Close()

	items := []TopicWithStats{}
	for rows.Next() {
		t, err := scanTopicWithStats(rows)
		if err != nil {
			return nil, fmt.Errorf("scan topic: %w", err)
		}
		items = append(items, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return items, nil
}

func (s *Store) GetTopicWithStats(ctx context.Context, userID, topicID int64) (*TopicWithStats, error) {
	row := s.db.QueryRow(ctx, topicsWithStatsSQL+` AND t.id = $2`, userID, topicID)
	t, err := scanTopicWithStats(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get topic with stats: %w", err)
	}
	return t, nil
}

func (s *Store) UpdateTopic(ctx context.Context, userID, topicID int64, p UpdateTopicParams) (*Topic, error) {
	setClauses := []string{}
	args := []any{}
	argIdx := 1

	if p.Name != nil {
		setClauses = append(setClauses, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *p.Name)
		argIdx++
	}
	if p.Description != nil {
		setClauses = append(setClauses, fmt.Sprintf("description = $%d", argIdx))
		args = append(args, *p.Description)
		argIdx++
	}
	if p.MatchThreshold != nil {
		setClauses = append(setClauses, fmt.Sprintf("match_threshold = $%d", argIdx))
		args = append(args, *p.MatchThreshold)
		argIdx++
	}
	if p.IsActive != nil {
		setClauses = append(setClauses, fmt.Sprintf("is_active = $%d", argIdx))
		args = append(args, *p.IsActive)
		argIdx++
	}

	if len(setClauses) == 0 {
		// Defensive: caller (service) is expected to validate non-empty patch.
		return nil, fmt.Errorf("%w: no fields to update", ErrInvalidInput)
	}

	setClauses = append(setClauses, "updated_at = now()")

	query := fmt.Sprintf(`UPDATE radar_topics
		SET %s
		WHERE id = $%d AND user_id = $%d
		RETURNING id, user_id, name, description, match_threshold, is_active,
		          embedding IS NOT NULL, created_at, updated_at`,
		strings.Join(setClauses, ", "), argIdx, argIdx+1)
	args = append(args, topicID, userID)

	var t Topic
	err := s.db.QueryRow(ctx, query, args...).Scan(
		&t.ID, &t.UserID, &t.Name, &t.Description,
		&t.MatchThreshold, &t.IsActive, &t.HasEmbedding,
		&t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("update topic: %w", err)
	}
	return &t, nil
}

func (s *Store) DeleteTopic(ctx context.Context, userID, topicID int64) error {
	tag, err := s.db.Exec(ctx,
		`DELETE FROM radar_topics WHERE id=$1 AND user_id=$2`, topicID, userID)
	if err != nil {
		return fmt.Errorf("delete topic: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListMatches(ctx context.Context, userID int64, p ListMatchesParams) ([]MatchView, int, error) {
	// total
	countQuery := `
		SELECT count(*)
		FROM radar_topic_matches m
		JOIN radar_topics t ON t.id = m.topic_id
		WHERE t.user_id = $1
		  AND ($2::bigint IS NULL OR m.topic_id = $2)
		  AND ($3::text   IS NULL OR m.state    = $3)`
	var total int
	if err := s.db.QueryRow(ctx, countQuery, userID, p.TopicID, p.State).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count matches: %w", err)
	}

	rows, err := s.db.Query(ctx, `
		SELECT
		  m.id, m.topic_id, t.name AS topic_name,
		  m.similarity, m.state, m.matched_at,
		  f.id, f.feed_id, fd.title AS feed_title,
		  f.url, f.title, f.summary,
		  f.published_at, f.discovered_at
		FROM radar_topic_matches m
		JOIN radar_topics t   ON t.id = m.topic_id
		JOIN radar_findings f ON f.id = m.finding_id
		JOIN radar_feeds fd   ON fd.id = f.feed_id
		WHERE t.user_id = $1
		  AND ($2::bigint IS NULL OR m.topic_id = $2)
		  AND ($3::text   IS NULL OR m.state    = $3)
		ORDER BY m.matched_at DESC
		LIMIT $4 OFFSET $5`,
		userID, p.TopicID, p.State, p.Limit, p.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list matches: %w", err)
	}
	defer rows.Close()

	items := []MatchView{}
	for rows.Next() {
		var m MatchView
		if err := rows.Scan(
			&m.ID, &m.TopicID, &m.TopicName,
			&m.Similarity, &m.State, &m.MatchedAt,
			&m.Finding.ID, &m.Finding.FeedID, &m.Finding.FeedTitle,
			&m.Finding.URL, &m.Finding.Title, &m.Finding.Summary,
			&m.Finding.PublishedAt, &m.Finding.DiscoveredAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan match: %w", err)
		}
		items = append(items, m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows: %w", err)
	}
	return items, total, nil
}

func (s *Store) GetMatch(ctx context.Context, userID, matchID int64) (*MatchView, error) {
	row := s.db.QueryRow(ctx, `
		SELECT
		  m.id, m.topic_id, t.name AS topic_name,
		  m.similarity, m.state, m.matched_at,
		  f.id, f.feed_id, fd.title AS feed_title,
		  f.url, f.title, f.summary,
		  f.published_at, f.discovered_at
		FROM radar_topic_matches m
		JOIN radar_topics t   ON t.id = m.topic_id
		JOIN radar_findings f ON f.id = m.finding_id
		JOIN radar_feeds fd   ON fd.id = f.feed_id
		WHERE m.id = $1 AND t.user_id = $2`,
		matchID, userID)

	var mv MatchView
	if err := row.Scan(
		&mv.ID, &mv.TopicID, &mv.TopicName,
		&mv.Similarity, &mv.State, &mv.MatchedAt,
		&mv.Finding.ID, &mv.Finding.FeedID, &mv.Finding.FeedTitle,
		&mv.Finding.URL, &mv.Finding.Title, &mv.Finding.Summary,
		&mv.Finding.PublishedAt, &mv.Finding.DiscoveredAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get match: %w", err)
	}
	return &mv, nil
}

func (s *Store) UpdateMatchState(ctx context.Context, userID, matchID int64, state string) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE radar_topic_matches
		SET state = $1
		WHERE id = $2
		  AND topic_id IN (SELECT id FROM radar_topics WHERE user_id = $3)`,
		state, matchID, userID)
	if err != nil {
		return fmt.Errorf("update match state: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) LastSweepAt(ctx context.Context, userID int64) (*time.Time, error) {
	var last *time.Time
	err := s.db.QueryRow(ctx, `
		SELECT MAX(f.last_fetched_at)
		FROM radar_feeds f
		JOIN radar_feed_subscriptions s ON s.feed_id = f.id
		WHERE s.user_id = $1 AND f.is_active`, userID).Scan(&last)
	if err != nil {
		return nil, fmt.Errorf("last sweep at: %w", err)
	}
	return last, nil
}

func (s *Store) ListFeeds(ctx context.Context, p ListFeedsParams) ([]Feed, int, error) {
	var total int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM radar_feeds`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count feeds: %w", err)
	}

	rows, err := s.db.Query(ctx, `
		SELECT id, url, kind, title, fetch_interval_seconds, is_active,
		       last_fetched_at, last_error, created_at
		FROM radar_feeds
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2`, p.Limit, p.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list feeds: %w", err)
	}
	defer rows.Close()

	items := []Feed{}
	for rows.Next() {
		var f Feed
		if err := rows.Scan(&f.ID, &f.URL, &f.Kind, &f.Title,
			&f.FetchIntervalSeconds, &f.IsActive,
			&f.LastFetchedAt, &f.LastError, &f.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan feed: %w", err)
		}
		items = append(items, f)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows: %w", err)
	}
	return items, total, nil
}
