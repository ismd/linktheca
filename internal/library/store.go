package library

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrAlreadySaved = errors.New("article already in library")
)

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

// UpsertContentParams holds parameters for inserting or finding existing content.
type UpsertContentParams struct {
	URL             string
	CanonicalURL    *string
	Title           *string
	Byline          *string
	Excerpt         *string
	Text            *string
	HTML            *string
	Lang            *string
	ReadingTimeSecs *int
	FetchError      *string
}

// UpsertContent inserts a new article_contents row or returns the existing one if the URL already exists
func (s *Store) UpsertContent(ctx context.Context, p UpsertContentParams) (*ArticleContent, error) {
	row := s.db.QueryRow(ctx, `
		INSERT INTO article_contents (url, canonical_url, title, byline, excerpt, text, html, lang, reading_time_seconds, fetch_error)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (url) DO UPDATE SET url = EXCLUDED.url
		RETURNING id, url, canonical_url, title, byline, excerpt, text, html, lang, reading_time_seconds, fetched_at, fetch_error
	`, p.URL, p.CanonicalURL, p.Title, p.Byline, p.Excerpt, p.Text, p.HTML, p.Lang, p.ReadingTimeSecs, p.FetchError)

	return scanContent(row)
}

// GetContentByURL returns the article_contents row for a URL, or ErrNotFound
func (s *Store) GetContentByURL(ctx context.Context, url string) (*ArticleContent, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, url, canonical_url, title, byline, excerpt, text, html, lang, reading_time_seconds, fetched_at, fetch_error
		FROM article_contents
		WHERE url = $1
	`, url)

	c, err := scanContent(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get content by url: %w", err)
	}

	return c, nil
}

// CreateItem creates a new library_items row. Returns ErrAlreadySaved if the user already saved this content.
func (s *Store) CreateItem(ctx context.Context, userID, contentID int64) (*Item, error) {
	row := s.db.QueryRow(ctx, `
		INSERT INTO library_items (user_id, content_id)
		VALUES ($1, $2)
		RETURNING id, user_id, content_id, state, is_favorite, saved_at, read_at
	`, userID, contentID)

	var item Item
	err := row.Scan(&item.ID, &item.UserID, &item.ContentID, &item.State,
		&item.IsFavorite, &item.SavedAt, &item.ReadAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrAlreadySaved
		}
		return nil, fmt.Errorf("create item: %w", err)
	}

	return &item, nil
}

func scanContent(row pgx.Row) (*ArticleContent, error) {
	var c ArticleContent

	err := row.Scan(&c.ID, &c.URL, &c.CanonicalURL, &c.Title, &c.Byline,
		&c.Excerpt, &c.Text, &c.HTML, &c.Lang, &c.ReadingTimeSecs,
		&c.FetchedAt, &c.FetchError)
	if err != nil {
		return nil, err
	}

	return &c, nil
}
