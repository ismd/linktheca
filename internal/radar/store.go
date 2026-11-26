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

// UpdateTopicEmbedding stores a topic's embedding together with the vector it
// is matched with (see Reference.TopicMatchVector).
func (s *Store) UpdateTopicEmbedding(ctx context.Context, topicID int64, vec, matchVec pgvector.Vector) error {
	cmd, err := s.db.Exec(ctx,
		`UPDATE radar_topics SET embedding=$1, match_embedding=$2, updated_at=now() WHERE id=$3`,
		vec, matchVec, topicID)
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
		INSERT INTO radar_feeds (url, kind, fetch_interval_seconds, owner_user_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id, url, kind, title, fetch_interval_seconds, is_active,
		          last_fetched_at, last_error, created_at
	`, p.URL, p.Kind, p.FetchIntervalSeconds, p.OwnerUserID)

	var f Feed
	if err := row.Scan(&f.ID, &f.URL, &f.Kind, &f.Title,
		&f.FetchIntervalSeconds, &f.IsActive,
		&f.LastFetchedAt, &f.LastError, &f.CreatedAt); err != nil {
		return nil, wrapPgError(err)
	}

	return &f, nil
}

// GetGlobalFeedByURL finds a catalog feed by URL. Personal feeds are never
// returned: a user adding a URL that only another account holds privately must
// get their own row, not a subscription to someone else's feed.
func (s *Store) GetGlobalFeedByURL(ctx context.Context, url string) (*Feed, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, url, kind, title, fetch_interval_seconds, is_active,
		       last_fetched_at, last_error, created_at
		FROM radar_feeds
		WHERE url = $1 AND owner_user_id IS NULL
	`, url)

	var f Feed
	if err := row.Scan(&f.ID, &f.URL, &f.Kind, &f.Title,
		&f.FetchIntervalSeconds, &f.IsActive,
		&f.LastFetchedAt, &f.LastError, &f.CreatedAt); err != nil {
		return nil, wrapPgError(err)
	}

	return &f, nil
}

// CountUserFeeds counts the personal feeds one account owns, for the quota.
func (s *Store) CountUserFeeds(ctx context.Context, userID int64) (int, error) {
	var n int
	if err := s.db.QueryRow(ctx,
		`SELECT count(*) FROM radar_feeds WHERE owner_user_id = $1`, userID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count user feeds: %w", err)
	}

	return n, nil
}

func (s *Store) Subscribe(ctx context.Context, userID, feedID int64) (*Subscription, error) {
	row := s.db.QueryRow(ctx, `
		INSERT INTO radar_feed_subscriptions (user_id, feed_id)
		SELECT $1, f.id FROM radar_feeds f
		WHERE f.id = $2 AND (f.owner_user_id IS NULL OR f.owner_user_id = $1)
		ON CONFLICT (user_id, feed_id)
		  DO UPDATE SET created_at = radar_feed_subscriptions.created_at
		RETURNING user_id, feed_id, created_at
	`, userID, feedID)

	var sub Subscription
	if err := row.Scan(&sub.UserID, &sub.FeedID, &sub.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrFeedNotFound
		}
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

// MarkFeedFetched records a successful fetch. The channel title is written
// only when the column is still empty, so an admin's manual name always wins.
func (s *Store) MarkFeedFetched(ctx context.Context, feedID int64, etag, lastModified, title *string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE radar_feeds
		SET last_fetched_at = now(), etag = $1, last_modified = $2, last_error = NULL,
		    title = coalesce(title, nullif(btrim($4), ''))
		WHERE id = $3
	`, etag, lastModified, feedID, title)

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

// UpdateFindingEmbedding stores a finding's article embedding together with
// the vector it is matched with (see FindingMatchVector).
func (s *Store) UpdateFindingEmbedding(ctx context.Context, findingID int64, vec, matchVec pgvector.Vector) error {
	cmd, err := s.db.Exec(ctx,
		`UPDATE radar_findings SET embedding = $1, match_embedding = $2 WHERE id = $3`,
		vec, matchVec, findingID)

	if err != nil {
		return fmt.Errorf("update finding embedding: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// MatchFindingToTopics inserts matches for all subscribed+active topics of
// the finding's feed whose match score reaches topic.match_threshold. The
// score is the inner product of the two match vectors (see Reference); it is
// stored as the match's similarity.
// Returns the number of matches inserted (existing rows are not counted).
func (s *Store) MatchFindingToTopics(ctx context.Context, findingID int64) (int64, error) {
	cmd, err := s.db.Exec(ctx, `
		INSERT INTO radar_topic_matches (topic_id, finding_id, similarity, state)
		SELECT rt.id,
		       $1,
		       -(rt.match_embedding <#> f.match_embedding) AS score,
		       'new'
		FROM radar_topics rt
		JOIN radar_feed_subscriptions rfs ON rfs.user_id = rt.user_id
		JOIN radar_findings f ON f.id = $1
		WHERE rfs.feed_id = f.feed_id
		  AND rt.is_active
		  AND rt.match_embedding IS NOT NULL
		  AND f.match_embedding IS NOT NULL
		  AND -(rt.match_embedding <#> f.match_embedding) >= rt.match_threshold
		ON CONFLICT (topic_id, finding_id) DO NOTHING
	`, findingID)

	if err != nil {
		return 0, fmt.Errorf("match finding: %w", err)
	}

	return cmd.RowsAffected(), nil
}

// PreviewFindings ranks the user's subscribed findings by their match score
// against a topic match vector, the way MatchFindingToTopics would score them.
func (s *Store) PreviewFindings(ctx context.Context, userID int64, matchVec pgvector.Vector, limit int) ([]PreviewMatch, error) {
	rows, err := s.db.Query(ctx, `
		SELECT -(f.match_embedding <#> $2) AS score,
		       f.id, f.feed_id, fd.title AS feed_title,
		       f.url, f.title, COALESCE(f.summary, ac.excerpt),
		       f.published_at, f.discovered_at
		FROM radar_findings f
		JOIN radar_feeds fd ON fd.id = f.feed_id
		JOIN radar_feed_subscriptions rfs
		  ON rfs.feed_id = f.feed_id AND rfs.user_id = $1
		LEFT JOIN article_contents ac ON ac.id = f.content_id
		WHERE f.match_embedding IS NOT NULL
		ORDER BY f.match_embedding <#> $2
		LIMIT $3`, userID, matchVec, limit)
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

// FillTopicMatchEmbeddings derives the match vector of every embedded topic
// that has none yet, q − c, from the stored embedding q and the reference
// centroid c. Topics embedded before match vectors existed need it once.
func (s *Store) FillTopicMatchEmbeddings(ctx context.Context, centroid pgvector.Vector) (int64, error) {
	cmd, err := s.db.Exec(ctx, `
		UPDATE radar_topics
		SET match_embedding = embedding - $1
		WHERE embedding IS NOT NULL AND match_embedding IS NULL`, centroid)
	if err != nil {
		return 0, fmt.Errorf("fill topic match embeddings: %w", err)
	}

	return cmd.RowsAffected(), nil
}

// ListFindingsWithoutMatchEmbedding returns up to limit embedded findings that
// have no match vector yet, newest first.
func (s *Store) ListFindingsWithoutMatchEmbedding(ctx context.Context, limit int) ([]int64, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id FROM radar_findings
		WHERE embedding IS NOT NULL AND match_embedding IS NULL
		ORDER BY discovered_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list findings without match embedding: %w", err)
	}

	ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return nil, fmt.Errorf("collect findings without match embedding: %w", err)
	}

	return ids, nil
}

// FindingForTitleEmbed is what completing a finding's match vector needs: the
// title to embed and the article embedding to combine it with.
type FindingForTitleEmbed struct {
	ID                int64
	Title             *string
	Embedding         *pgvector.Vector
	HasMatchEmbedding bool
}

func (s *Store) GetFindingForTitleEmbed(ctx context.Context, findingID int64) (*FindingForTitleEmbed, error) {
	row := s.db.QueryRow(ctx,
		`SELECT id, title, embedding, match_embedding IS NOT NULL
		 FROM radar_findings WHERE id = $1`, findingID)

	var f FindingForTitleEmbed
	if err := row.Scan(&f.ID, &f.Title, &f.Embedding, &f.HasMatchEmbedding); err != nil {
		return nil, wrapPgError(err)
	}

	return &f, nil
}

// FillFindingMatchEmbedding stores the match vector of a finding that has
// none. A finding re-embedded in the meantime already has one that fits its
// new embedding, and keeps it.
func (s *Store) FillFindingMatchEmbedding(ctx context.Context, findingID int64, matchVec pgvector.Vector) error {
	if _, err := s.db.Exec(ctx,
		`UPDATE radar_findings SET match_embedding = $2 WHERE id = $1 AND match_embedding IS NULL`,
		findingID, matchVec); err != nil {
		return fmt.Errorf("fill finding match embedding: %w", err)
	}

	return nil
}

type FindingForEmbed struct {
	ID      int64
	Title   *string
	Summary *string
	// Text is the article text downloaded for the finding, nil when the page
	// was not fetched or has been pruned since.
	Text         *string
	HasEmbedding bool
}

func (s *Store) GetFindingForEmbed(ctx context.Context, findingID int64) (*FindingForEmbed, error) {
	row := s.db.QueryRow(ctx,
		`SELECT f.id, f.title, f.summary, ac.text, f.embedding IS NOT NULL
		 FROM radar_findings f
		 LEFT JOIN article_contents ac ON ac.id = f.content_id
		 WHERE f.id = $1`, findingID)

	var f FindingForEmbed
	if err := row.Scan(&f.ID, &f.Title, &f.Summary, &f.Text, &f.HasEmbedding); err != nil {
		return nil, wrapPgError(err)
	}

	return &f, nil
}

type FindingForFetch struct {
	ID        int64
	URL       string
	ContentID *int64
}

func (s *Store) GetFindingForFetch(ctx context.Context, findingID int64) (*FindingForFetch, error) {
	row := s.db.QueryRow(ctx,
		`SELECT id, url, content_id FROM radar_findings WHERE id = $1`, findingID)

	var f FindingForFetch
	if err := row.Scan(&f.ID, &f.URL, &f.ContentID); err != nil {
		return nil, wrapPgError(err)
	}

	return &f, nil
}

// ListFindingsWithoutPage returns findings discovered since the given time
// whose page has not been downloaded, newest first.
func (s *Store) ListFindingsWithoutPage(ctx context.Context, since time.Time) ([]int64, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id FROM radar_findings
		WHERE content_id IS NULL AND discovered_at >= $1
		ORDER BY discovered_at DESC`, since)
	if err != nil {
		return nil, fmt.Errorf("list findings without page: %w", err)
	}

	ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return nil, fmt.Errorf("collect findings without page: %w", err)
	}

	return ids, nil
}

// SetFindingContent links a finding to the downloaded text of its page.
func (s *Store) SetFindingContent(ctx context.Context, findingID, contentID int64) error {
	cmd, err := s.db.Exec(ctx,
		`UPDATE radar_findings SET content_id = $2 WHERE id = $1`, findingID, contentID)
	if err != nil {
		return fmt.Errorf("set finding content: %w", err)
	}

	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// PruneFindingContent deletes pages that Radar downloaded and no longer
// needs: fetched before cutoff, linked only to findings that matched no
// topic, and not saved to anyone's Library. Their findings keep everything
// but the text, embeddings included, so they still show up in topic previews
// and stay usable for learning from what users do with matches later.
func (s *Store) PruneFindingContent(ctx context.Context, cutoff time.Time) (int64, error) {
	cmd, err := s.db.Exec(ctx, `
		DELETE FROM article_contents ac
		WHERE ac.fetched_at < $1
		  AND EXISTS (SELECT 1 FROM radar_findings f WHERE f.content_id = ac.id)
		  AND NOT EXISTS (SELECT 1 FROM library_items li WHERE li.content_id = ac.id)
		  AND NOT EXISTS (
		        SELECT 1
		        FROM radar_findings f
		        JOIN radar_topic_matches m ON m.finding_id = f.id
		        WHERE f.content_id = ac.id)`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("prune finding content: %w", err)
	}

	return cmd.RowsAffected(), nil
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
		  f.url, f.title, COALESCE(f.summary, ac.excerpt),
		  f.published_at, f.discovered_at
		FROM radar_topic_matches m
		JOIN radar_topics t   ON t.id = m.topic_id
		JOIN radar_findings f ON f.id = m.finding_id
		JOIN radar_feeds fd   ON fd.id = f.feed_id
		LEFT JOIN article_contents ac ON ac.id = f.content_id
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
		  f.url, f.title, COALESCE(f.summary, ac.excerpt),
		  f.published_at, f.discovered_at,
		  ac.id, ac.html, ac.text, ac.byline, ac.site_name, ac.reading_time_seconds
		FROM radar_topic_matches m
		JOIN radar_topics t   ON t.id = m.topic_id
		JOIN radar_findings f ON f.id = m.finding_id
		JOIN radar_feeds fd   ON fd.id = f.feed_id
		LEFT JOIN article_contents ac ON ac.id = f.content_id
		WHERE m.id = $1 AND t.user_id = $2`,
		matchID, userID)

	var (
		mv        MatchView
		contentID *int64
		c         FindingContent
	)
	if err := row.Scan(
		&mv.ID, &mv.TopicID, &mv.TopicName,
		&mv.Similarity, &mv.State, &mv.MatchedAt,
		&mv.Finding.ID, &mv.Finding.FeedID, &mv.Finding.FeedTitle,
		&mv.Finding.URL, &mv.Finding.Title, &mv.Finding.Summary,
		&mv.Finding.PublishedAt, &mv.Finding.DiscoveredAt,
		&contentID, &c.HTML, &c.Text, &c.Byline, &c.SiteName, &c.ReadingTimeSecs,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get match: %w", err)
	}

	if contentID != nil {
		mv.Finding.Content = &c
	}

	return &mv, nil
}

// GetFinding returns one finding with its downloaded article, provided the
// user subscribes to its feed: the same findings a topic preview can surface,
// so a preview row can be opened even though it is not a match yet.
func (s *Store) GetFinding(ctx context.Context, userID, findingID int64) (*MatchFinding, error) {
	row := s.db.QueryRow(ctx, `
		SELECT
		  f.id, f.feed_id, fd.title AS feed_title,
		  f.url, f.title, COALESCE(f.summary, ac.excerpt),
		  f.published_at, f.discovered_at,
		  ac.id, ac.html, ac.text, ac.byline, ac.site_name, ac.reading_time_seconds
		FROM radar_findings f
		JOIN radar_feeds fd ON fd.id = f.feed_id
		JOIN radar_feed_subscriptions rfs
		  ON rfs.feed_id = f.feed_id AND rfs.user_id = $2
		LEFT JOIN article_contents ac ON ac.id = f.content_id
		WHERE f.id = $1`,
		findingID, userID)

	var (
		mf        MatchFinding
		contentID *int64
		c         FindingContent
	)
	if err := row.Scan(
		&mf.ID, &mf.FeedID, &mf.FeedTitle,
		&mf.URL, &mf.Title, &mf.Summary,
		&mf.PublishedAt, &mf.DiscoveredAt,
		&contentID, &c.HTML, &c.Text, &c.Byline, &c.SiteName, &c.ReadingTimeSecs,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get finding: %w", err)
	}

	if contentID != nil {
		mf.Content = &c
	}

	return &mf, nil
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

func (s *Store) ListFeeds(ctx context.Context, userID int64, p ListFeedsParams) ([]FeedListItem, int, error) {
	// The predicate is shared by the count and the page so the two agree. The
	// global scope needs no user id, so the count's argument list follows the
	// predicate; the page query always binds $1 through its subscribed subquery.
	where := `(f.owner_user_id IS NULL OR f.owner_user_id = $1)`
	countArgs := []any{userID}
	if p.Scope == FeedScopeGlobal {
		where = `f.owner_user_id IS NULL`
		countArgs = nil
	}

	var total int
	if err := s.db.QueryRow(ctx,
		`SELECT count(*) FROM radar_feeds f WHERE `+where, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count feeds: %w", err)
	}

	rows, err := s.db.Query(ctx, `
		SELECT f.id, f.url, f.kind, f.title, f.fetch_interval_seconds, f.is_active,
		       f.last_fetched_at, f.last_error, f.created_at,
		       EXISTS (SELECT 1 FROM radar_feed_subscriptions s
		               WHERE s.feed_id = f.id AND s.user_id = $1) AS subscribed,
		       coalesce(fc.n, 0) AS finding_count,
		       f.owner_user_id IS NOT NULL AS is_own
		FROM radar_feeds f
		LEFT JOIN (
			SELECT feed_id, count(*) AS n FROM radar_findings GROUP BY feed_id
		) fc ON fc.feed_id = f.id
		WHERE `+where+`
		ORDER BY (f.owner_user_id IS NULL) ASC, lower(coalesce(f.title, f.url)) ASC
		LIMIT $2 OFFSET $3`, userID, p.Limit, p.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list feeds: %w", err)
	}
	defer rows.Close()

	items := []FeedListItem{}
	for rows.Next() {
		var it FeedListItem
		if err := rows.Scan(&it.ID, &it.URL, &it.Kind, &it.Title,
			&it.FetchIntervalSeconds, &it.IsActive,
			&it.LastFetchedAt, &it.LastError, &it.CreatedAt,
			&it.Subscribed, &it.FindingCount, &it.IsOwn); err != nil {
			return nil, 0, fmt.Errorf("scan feed: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows: %w", err)
	}
	return items, total, nil
}

// UpdateFeed applies a partial patch. An empty title clears the column so the
// crawler can fill it from the channel again.
func (s *Store) UpdateFeed(ctx context.Context, feedID int64, owner *int64, p UpdateFeedParams) (*Feed, error) {
	setClauses := []string{}
	args := []any{}
	argIdx := 1

	if p.Title != nil {
		setClauses = append(setClauses, fmt.Sprintf("title = nullif($%d, '')", argIdx))
		args = append(args, *p.Title)
		argIdx++
	}
	if p.FetchIntervalSeconds != nil {
		setClauses = append(setClauses, fmt.Sprintf("fetch_interval_seconds = $%d", argIdx))
		args = append(args, *p.FetchIntervalSeconds)
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

	args = append(args, feedID, owner)
	query := fmt.Sprintf(`
		UPDATE radar_feeds SET %s
		WHERE id = $%d AND owner_user_id IS NOT DISTINCT FROM $%d
		RETURNING id, url, kind, title, fetch_interval_seconds, is_active,
		          last_fetched_at, last_error, created_at`,
		strings.Join(setClauses, ", "), argIdx, argIdx+1)

	var f Feed
	if err := s.db.QueryRow(ctx, query, args...).Scan(&f.ID, &f.URL, &f.Kind, &f.Title,
		&f.FetchIntervalSeconds, &f.IsActive,
		&f.LastFetchedAt, &f.LastError, &f.CreatedAt); err != nil {
		// wrapPgError already turns pgx.ErrNoRows into ErrNotFound.
		return nil, wrapPgError(err)
	}

	return &f, nil
}

// SeedSubscriptions subscribes a fresh user to every active catalog feed and
// returns how many subscriptions were inserted. Re-running it inserts nothing.
func (s *Store) SeedSubscriptions(ctx context.Context, userID int64) (int, error) {
	cmd, err := s.db.Exec(ctx, `
		INSERT INTO radar_feed_subscriptions (user_id, feed_id)
		SELECT $1, id FROM radar_feeds WHERE is_active AND owner_user_id IS NULL
		ON CONFLICT DO NOTHING`, userID)
	if err != nil {
		return 0, fmt.Errorf("seed subscriptions: %w", err)
	}

	return int(cmd.RowsAffected()), nil
}

func (s *Store) DeleteFeed(ctx context.Context, feedID int64, owner *int64) error {
	cmd, err := s.db.Exec(ctx,
		`DELETE FROM radar_feeds
		 WHERE id = $1 AND owner_user_id IS NOT DISTINCT FROM $2`, feedID, owner)
	if err != nil {
		return fmt.Errorf("delete feed: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) Unsubscribe(ctx context.Context, userID, feedID int64) error {
	if _, err := s.db.Exec(ctx,
		`DELETE FROM radar_feed_subscriptions WHERE user_id = $1 AND feed_id = $2`,
		userID, feedID); err != nil {
		return fmt.Errorf("unsubscribe: %w", err)
	}

	return nil
}
