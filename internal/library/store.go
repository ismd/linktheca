package library

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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
	ImageURL        *string
	FaviconURL      *string
	SiteName        *string
	PublishedTime   *time.Time
	ModifiedTime    *time.Time
	ReadingTimeSecs *int
	Image           *string
	Favicon         *string
	FetchError      *string
}

// UpsertContent inserts a new article_contents row or returns the existing one if the URL already exists
func (s *Store) UpsertContent(ctx context.Context, p UpsertContentParams) (*ArticleContent, error) {
	row := s.db.QueryRow(ctx, `
		INSERT INTO article_contents (url, canonical_url, title, byline, excerpt, text, html, lang, image_url, favicon_url, site_name, published_time, modified_time, reading_time_seconds, image, favicon, fetch_error)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		ON CONFLICT (url) DO UPDATE SET
			image_url      = COALESCE(EXCLUDED.image_url, article_contents.image_url),
			favicon_url    = COALESCE(EXCLUDED.favicon_url, article_contents.favicon_url),
			site_name      = COALESCE(EXCLUDED.site_name, article_contents.site_name),
			published_time = COALESCE(EXCLUDED.published_time, article_contents.published_time),
			modified_time  = COALESCE(EXCLUDED.modified_time, article_contents.modified_time),
			image          = COALESCE(EXCLUDED.image, article_contents.image),
			favicon        = COALESCE(EXCLUDED.favicon, article_contents.favicon)
		RETURNING id, url, canonical_url, title, byline, excerpt, text, html, lang, image_url, favicon_url, site_name, published_time, modified_time, reading_time_seconds, image, favicon, fetched_at, fetch_error
	`, p.URL, p.CanonicalURL, p.Title, p.Byline, p.Excerpt, p.Text, p.HTML, p.Lang, p.ImageURL, p.FaviconURL, p.SiteName, p.PublishedTime, p.ModifiedTime, p.ReadingTimeSecs, p.Image, p.Favicon, p.FetchError)

	return scanContent(row)
}

// GetContentByURL returns the article_contents row for a URL, or ErrNotFound
func (s *Store) GetContentByURL(ctx context.Context, url string) (*ArticleContent, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, url, canonical_url, title, byline, excerpt, text, html, lang, image_url, favicon_url, site_name, published_time, modified_time, reading_time_seconds, image, favicon, fetched_at, fetch_error
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
		&c.Excerpt, &c.Text, &c.HTML, &c.Lang, &c.ImageURL, &c.FaviconURL, &c.SiteName,
		&c.PublishedTime, &c.ModifiedTime, &c.ReadingTimeSecs, &c.Image, &c.Favicon,
		&c.FetchedAt, &c.FetchError)
	if err != nil {
		return nil, err
	}

	return &c, nil
}

// GetItemByID returns a single library item with joined content fields
// Only returns items belonging to the given user
func (s *Store) GetItemByID(ctx context.Context, userID, itemID int64) (*Item, error) {
	row := s.db.QueryRow(ctx, `
		SELECT li.id, li.user_id, li.content_id, li.state, li.is_favorite, li.saved_at, li.read_at,
		       ac.url, ac.title, ac.excerpt, ac.reading_time_seconds, ac.image
		FROM library_items li
		JOIN article_contents ac ON ac.id = li.content_id
		WHERE li.id = $1 AND li.user_id = $2
	`, itemID, userID)

	return scanItem(row)
}

// ListItems returns paginated library items for a user with optional state filter
func (s *Store) ListItems(ctx context.Context, p ListParams) (*ListResult, error) {
	// Count total matching rows.
	countQuery := `SELECT count(*) FROM library_items WHERE user_id = $1`
	countArgs := []any{p.UserID}
	argIdx := 2

	if p.State != "" {
		countQuery += fmt.Sprintf(` AND state = $%d`, argIdx)
		countArgs = append(countArgs, p.State)
		argIdx++
	} else {
		countQuery += ` AND state <> 'archived'`
	}
	if p.Favorite != nil {
		countQuery += fmt.Sprintf(` AND is_favorite = $%d`, argIdx)
		countArgs = append(countArgs, *p.Favorite)
	}

	var total int
	if err := s.db.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count items: %w", err)
	}

	// Fetch items page.
	query := `
		SELECT li.id, li.user_id, li.content_id, li.state, li.is_favorite, li.saved_at, li.read_at,
		       ac.url, ac.title, ac.excerpt, ac.reading_time_seconds, ac.image
		FROM library_items li
		JOIN article_contents ac ON ac.id = li.content_id
		WHERE li.user_id = $1`
	args := []any{p.UserID}
	argIdx = 2

	if p.State != "" {
		query += fmt.Sprintf(` AND li.state = $%d`, argIdx)
		args = append(args, p.State)
		argIdx++
	} else {
		query += ` AND li.state <> 'archived'`
	}
	if p.Favorite != nil {
		query += fmt.Sprintf(` AND li.is_favorite = $%d`, argIdx)
		args = append(args, *p.Favorite)
		argIdx++
	}

	query += ` ORDER BY li.saved_at DESC`
	query += fmt.Sprintf(` LIMIT $%d OFFSET $%d`, argIdx, argIdx+1)
	args = append(args, p.Limit, p.Offset)

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	defer rows.Close()

	var items []Item
	for rows.Next() {
		item, err := scanItemFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("scan item: %w", err)
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	if items == nil {
		items = []Item{}
	}

	return &ListResult{Items: items, Total: total}, nil
}

// UpdateItem partially updates a library item. Only non-nil fields are changed
// When state changes to "read", read_at is set to now(). When state changes away from "read", read_at is cleared
func (s *Store) UpdateItem(ctx context.Context, userID, itemID int64, p UpdateParams) (*Item, error) {
	setClauses := []string{}
	args := []any{}
	argIdx := 1

	if p.State != nil {
		setClauses = append(setClauses, fmt.Sprintf("state = $%d", argIdx))
		args = append(args, *p.State)
		argIdx++

		if *p.State == "read" {
			setClauses = append(setClauses, "read_at = now()")
		} else {
			setClauses = append(setClauses, "read_at = NULL")
		}
	}

	if p.IsFavorite != nil {
		setClauses = append(setClauses, fmt.Sprintf("is_favorite = $%d", argIdx))
		args = append(args, *p.IsFavorite)
		argIdx++
	}

	if len(setClauses) == 0 {
		return s.GetItemByID(ctx, userID, itemID)
	}

	query := fmt.Sprintf(`UPDATE library_items SET %s WHERE id = $%d AND user_id = $%d
		RETURNING id, user_id, content_id, state, is_favorite, saved_at, read_at`,
		strings.Join(setClauses, ", "), argIdx, argIdx+1)
	args = append(args, itemID, userID)

	var item Item
	err := s.db.QueryRow(ctx, query, args...).Scan(
		&item.ID, &item.UserID, &item.ContentID, &item.State,
		&item.IsFavorite, &item.SavedAt, &item.ReadAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("update item: %w", err)
	}

	return &item, nil
}

// DeleteItem removes a library item. Returns ErrNotFound if the item doesn't exist or doesn't belong to the user.
func (s *Store) DeleteItem(ctx context.Context, userID, itemID int64) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM library_items WHERE id = $1 AND user_id = $2`, itemID, userID)

	if err != nil {
		return fmt.Errorf("delete item: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

func scanItem(row pgx.Row) (*Item, error) {
	var item Item

	err := row.Scan(&item.ID, &item.UserID, &item.ContentID, &item.State,
		&item.IsFavorite, &item.SavedAt, &item.ReadAt,
		&item.URL, &item.Title, &item.Excerpt, &item.ReadTimeSecs, &item.Image)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	return &item, nil
}

func scanItemFromRows(rows pgx.Rows) (*Item, error) {
	var item Item

	err := rows.Scan(&item.ID, &item.UserID, &item.ContentID, &item.State,
		&item.IsFavorite, &item.SavedAt, &item.ReadAt,
		&item.URL, &item.Title, &item.Excerpt, &item.ReadTimeSecs, &item.Image)

	if err != nil {
		return nil, err
	}

	return &item, nil
}

// GetItemDetail returns a library item with the full article_contents record
func (s *Store) GetItemDetail(ctx context.Context, userID, itemID int64) (*ItemDetail, error) {
	row := s.db.QueryRow(ctx, `
		SELECT li.id, li.user_id, li.content_id, li.state, li.is_favorite, li.saved_at, li.read_at,
		       ac.url, ac.title, ac.excerpt, ac.reading_time_seconds, ac.image,
		       ac.id, ac.url, ac.canonical_url, ac.title, ac.byline, ac.excerpt, ac.text, ac.html,
		       ac.lang, ac.image_url, ac.favicon_url, ac.site_name, ac.published_time, ac.modified_time,
		       ac.reading_time_seconds, ac.image, ac.favicon, ac.fetched_at, ac.fetch_error
		FROM library_items li
		JOIN article_contents ac ON ac.id = li.content_id
		WHERE li.id = $1 AND li.user_id = $2
	`, itemID, userID)

	var d ItemDetail
	err := row.Scan(
		&d.ID, &d.UserID, &d.ContentID, &d.State, &d.IsFavorite, &d.SavedAt, &d.ReadAt,
		&d.URL, &d.Title, &d.Excerpt, &d.ReadTimeSecs, &d.Image,
		&d.Content.ID, &d.Content.URL, &d.Content.CanonicalURL, &d.Content.Title,
		&d.Content.Byline, &d.Content.Excerpt, &d.Content.Text, &d.Content.HTML,
		&d.Content.Lang, &d.Content.ImageURL, &d.Content.FaviconURL, &d.Content.SiteName,
		&d.Content.PublishedTime, &d.Content.ModifiedTime, &d.Content.ReadingTimeSecs,
		&d.Content.Image, &d.Content.Favicon, &d.Content.FetchedAt, &d.Content.FetchError,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get item detail: %w", err)
	}

	return &d, nil
}
