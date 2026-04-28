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
