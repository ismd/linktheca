package content

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when no article_contents row exists for a URL.
var ErrNotFound = errors.New("content not found")

// Record is a row of article_contents: one fetched page, shared by every
// module that needs it. The Library links saved items to it and Radar links
// findings to it, so a page fetched by one is reused by the other.
type Record struct {
	ID              int64      `json:"id"`
	URL             string     `json:"url"`
	CanonicalURL    *string    `json:"canonical_url,omitempty"`
	Title           *string    `json:"title,omitempty"`
	Byline          *string    `json:"byline,omitempty"`
	Excerpt         *string    `json:"excerpt,omitempty"`
	Text            *string    `json:"text,omitempty"`
	HTML            *string    `json:"html,omitempty"`
	Lang            *string    `json:"lang,omitempty"`
	ImageURL        *string    `json:"image_url,omitempty"`
	FaviconURL      *string    `json:"favicon_url,omitempty"`
	SiteName        *string    `json:"site_name,omitempty"`
	PublishedTime   *time.Time `json:"published_time,omitempty"`
	ModifiedTime    *time.Time `json:"modified_time,omitempty"`
	ReadingTimeSecs *int       `json:"reading_time_seconds,omitempty"`
	Image           *string    `json:"image,omitempty"`
	Favicon         *string    `json:"favicon,omitempty"`
	FetchedAt       time.Time  `json:"fetched_at"`
	FetchError      *string    `json:"fetch_error,omitempty"`
}

// UpsertParams is one fetch of a page: either what was extracted from it or,
// with FetchError set, why that failed.
type UpsertParams struct {
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

// ParamsFromArticle turns an extracted article into upsert params, leaving
// out empty fields so they do not overwrite what an earlier fetch stored.
func ParamsFromArticle(a *Article) UpsertParams {
	return UpsertParams{
		URL:             a.URL,
		CanonicalURL:    nilIfEmpty(a.CanonicalURL),
		Title:           nilIfEmpty(a.Title),
		Byline:          nilIfEmpty(a.Byline),
		Excerpt:         nilIfEmpty(a.Excerpt),
		Text:            nilIfEmpty(a.Text),
		HTML:            nilIfEmpty(a.HTML),
		Lang:            nilIfEmpty(a.Lang),
		ImageURL:        nilIfEmpty(a.ImageURL),
		FaviconURL:      nilIfEmpty(a.Favicon),
		SiteName:        nilIfEmpty(a.SiteName),
		PublishedTime:   nilIfZeroTime(a.PublishedTime),
		ModifiedTime:    nilIfZeroTime(a.ModifiedTime),
		ReadingTimeSecs: nilIfZero(a.ReadingTimeSecs),
	}
}

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

const recordColumns = `id, url, canonical_url, title, byline, excerpt, text, html, lang,
	image_url, favicon_url, site_name, published_time, modified_time,
	reading_time_seconds, image, favicon, fetched_at, fetch_error`

// Upsert stores one fetch of a page, keyed by URL.
//
// A second fetch of the same URL fills in what it found and keeps whatever it
// did not: a Radar fetch that downloads no images leaves the Library's images
// in place, and a Library save fills them into a row Radar created.
//
// A failed fetch never hides content that an earlier fetch extracted: the
// error is recorded only on a row that has no text. A successful fetch clears
// an earlier error and moves fetched_at forward.
func (s *Store) Upsert(ctx context.Context, p UpsertParams) (*Record, error) {
	row := s.db.QueryRow(ctx, `
		INSERT INTO article_contents (url, canonical_url, title, byline, excerpt, text, html, lang,
			image_url, favicon_url, site_name, published_time, modified_time,
			reading_time_seconds, image, favicon, fetch_error)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		ON CONFLICT (url) DO UPDATE SET
			canonical_url        = COALESCE(EXCLUDED.canonical_url, article_contents.canonical_url),
			title                = COALESCE(EXCLUDED.title, article_contents.title),
			byline               = COALESCE(EXCLUDED.byline, article_contents.byline),
			excerpt              = COALESCE(EXCLUDED.excerpt, article_contents.excerpt),
			text                 = COALESCE(EXCLUDED.text, article_contents.text),
			html                 = COALESCE(EXCLUDED.html, article_contents.html),
			lang                 = COALESCE(EXCLUDED.lang, article_contents.lang),
			reading_time_seconds = COALESCE(EXCLUDED.reading_time_seconds, article_contents.reading_time_seconds),
			image_url            = COALESCE(EXCLUDED.image_url, article_contents.image_url),
			favicon_url          = COALESCE(EXCLUDED.favicon_url, article_contents.favicon_url),
			site_name            = COALESCE(EXCLUDED.site_name, article_contents.site_name),
			published_time       = COALESCE(EXCLUDED.published_time, article_contents.published_time),
			modified_time        = COALESCE(EXCLUDED.modified_time, article_contents.modified_time),
			image                = COALESCE(EXCLUDED.image, article_contents.image),
			favicon              = COALESCE(EXCLUDED.favicon, article_contents.favicon),
			fetch_error          = CASE
			                         WHEN EXCLUDED.fetch_error IS NULL OR article_contents.text IS NOT NULL THEN NULL
			                         ELSE EXCLUDED.fetch_error
			                       END,
			fetched_at           = CASE
			                         WHEN EXCLUDED.fetch_error IS NULL THEN now()
			                         ELSE article_contents.fetched_at
			                       END
		RETURNING `+recordColumns,
		p.URL, p.CanonicalURL, p.Title, p.Byline, p.Excerpt, p.Text, p.HTML, p.Lang,
		p.ImageURL, p.FaviconURL, p.SiteName, p.PublishedTime, p.ModifiedTime,
		p.ReadingTimeSecs, p.Image, p.Favicon, p.FetchError)

	r, err := scanRecord(row)
	if err != nil {
		return nil, fmt.Errorf("upsert content: %w", err)
	}

	return r, nil
}

// GetByURL returns the row for a URL, or ErrNotFound.
func (s *Store) GetByURL(ctx context.Context, url string) (*Record, error) {
	row := s.db.QueryRow(ctx, `SELECT `+recordColumns+` FROM article_contents WHERE url = $1`, url)

	r, err := scanRecord(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get content by url: %w", err)
	}

	return r, nil
}

func scanRecord(row pgx.Row) (*Record, error) {
	var r Record

	err := row.Scan(&r.ID, &r.URL, &r.CanonicalURL, &r.Title, &r.Byline,
		&r.Excerpt, &r.Text, &r.HTML, &r.Lang, &r.ImageURL, &r.FaviconURL, &r.SiteName,
		&r.PublishedTime, &r.ModifiedTime, &r.ReadingTimeSecs, &r.Image, &r.Favicon,
		&r.FetchedAt, &r.FetchError)
	if err != nil {
		return nil, err
	}

	return &r, nil
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}

func nilIfZero(n int) *int {
	if n == 0 {
		return nil
	}

	return &n
}

func nilIfZeroTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}

	return &t
}
