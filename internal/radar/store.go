package radar

import (
	"context"
	"errors"
	"fmt"

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
