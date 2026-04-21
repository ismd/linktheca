# Phase 2: Library Backend — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Library (read-it-later) backend module — users can save URLs, the system extracts article content via go-readability, and users manage their saved items through a full CRUD HTTP API with filtering and pagination.

**Architecture:** Two new migrations create `article_contents` (shared content cache) and `library_items` (per-user saved articles). A new `internal/core/content/` package wraps go-readability for article extraction. The `internal/library/` module follows the established `store → service → http` pattern. Content extraction happens synchronously when a user saves a URL; if the same URL was already parsed, the cached `article_contents` row is reused. All endpoints live behind `RequireUser` middleware.

**Tech Stack:** Go 1.26+, `go-chi/chi/v5`, `jackc/pgx/v5`, `go-shiori/go-readability`, `stretchr/testify`, `testcontainers-go`. Same stack as Phase 1 — no new infrastructure.

**Module path:** `github.com/ismd/linktheca`

**Working directory:** `/home/ismd/coding/linktheca`

---

## File structure created by this phase

```
linktheca/
├── migrations/
│   ├── 004_article_contents.sql        # shared content cache table
│   └── 005_library_items.sql           # per-user saved articles
│
├── internal/
│   ├── core/
│   │   └── content/
│   │       ├── extractor.go            # go-readability wrapper, Extractor interface
│   │       └── extractor_test.go       # unit test with HTTP test server
│   ├── library/
│   │   ├── types.go                    # Item, ArticleContent, DTOs
│   │   ├── store.go                    # SQL: article_contents + library_items
│   │   ├── store_test.go              # integration tests with testdb
│   │   ├── service.go                 # SaveURL, List, GetByID, Update, Delete
│   │   ├── service_test.go            # unit tests with mock store
│   │   ├── http.go                    # HTTP handlers
│   │   └── http_test.go              # HTTP-level integration tests
│   └── server/
│       └── server.go                   # Modified: wire library routes
```

**Not in this phase:** tags, full-text search endpoint, bulk operations, import/export, Radar module, frontend.

---

## Conventions for every task

- **TDD everywhere.** Every non-trivial function gets a failing test first, then minimal implementation, then verification.
- **Commit after each task.** Small, focused commits make review easy and rollback cheap.
- **Run from the repo root** (`/home/ismd/coding/linktheca`) unless otherwise noted.
- **Do not use `git add .`** — stage files explicitly to avoid accidentally including secrets or build artifacts.
- **Commit messages** follow `<type>(<scope>): <subject>` (e.g., `feat(library): add library_items migration`).
- **Go version:** Go 1.26 or later. Check with `go version`.

---

## Part A: Migrations

### Task 1: article_contents migration

**Files:**
- Create: `migrations/004_article_contents.sql`

- [x] **Step 1: Create migration file**

Create `migrations/004_article_contents.sql`:
```sql
-- +goose Up
CREATE TABLE article_contents (
    id                   BIGSERIAL PRIMARY KEY,
    url                  TEXT NOT NULL UNIQUE,
    canonical_url        TEXT,
    title                TEXT,
    byline               TEXT,
    excerpt              TEXT,
    text                 TEXT,
    html                 TEXT,
    lang                 TEXT,
    reading_time_seconds INT,
    fetched_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    fetch_error          TEXT
);

CREATE INDEX article_contents_fts_idx ON article_contents USING GIN (
    to_tsvector('simple', coalesce(title, '') || ' ' || coalesce(text, ''))
);

-- +goose Down
DROP TABLE article_contents;
```

- [x] **Step 2: Verify migration applies**

Start Postgres and run the backend to apply migrations:
```bash
make dev-db
sleep 3
LINKTHECA_DB_DSN="postgres://linktheca:linktheca@localhost:5432/linktheca?sslmode=disable" \
LINKTHECA_JWT_SECRET="dev-only-secret-that-is-at-least-32-bytes-long" \
go run ./cmd/linktheca &
sleep 2
```

Verify the table exists:
```bash
docker compose -f compose.dev.yaml exec postgres psql -U linktheca -d linktheca -c "\d article_contents"
```

Expected: prints column listing showing `id`, `url`, `title`, `text`, `html`, etc.

Stop:
```bash
kill %1
wait 2>/dev/null
```

- [x] **Step 3: Commit**

```bash
git add migrations/004_article_contents.sql
git commit -m "feat(db): add article_contents migration for shared content cache"
```

---

### Task 2: library_items migration

**Files:**
- Create: `migrations/005_library_items.sql`

- [x] **Step 1: Create migration file**

Create `migrations/005_library_items.sql`:
```sql
-- +goose Up
CREATE TABLE library_items (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content_id  BIGINT NOT NULL REFERENCES article_contents(id),
    state       TEXT NOT NULL DEFAULT 'unread'
                CHECK (state IN ('unread', 'read', 'archived')),
    is_favorite BOOLEAN NOT NULL DEFAULT FALSE,
    note        TEXT,
    saved_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    read_at     TIMESTAMPTZ,
    UNIQUE (user_id, content_id)
);

CREATE INDEX library_items_user_saved_idx ON library_items (user_id, saved_at DESC);
CREATE INDEX library_items_user_state_idx ON library_items (user_id, state);

-- +goose Down
DROP TABLE library_items;
```

- [x] **Step 2: Verify migration applies**

```bash
LINKTHECA_DB_DSN="postgres://linktheca:linktheca@localhost:5432/linktheca?sslmode=disable" \
LINKTHECA_JWT_SECRET="dev-only-secret-that-is-at-least-32-bytes-long" \
go run ./cmd/linktheca &
sleep 2
docker compose -f compose.dev.yaml exec postgres psql -U linktheca -d linktheca -c "\d library_items"
kill %1
wait 2>/dev/null
```

Expected: prints column listing with `id`, `user_id`, `content_id`, `state`, `is_favorite`, `note`, `saved_at`, `read_at`.

- [x] **Step 3: Stop dev DB and commit**

```bash
make dev-db-down
git add migrations/005_library_items.sql
git commit -m "feat(db): add library_items migration for per-user saved articles"
```

---

## Part B: Content extraction

### Task 3: Content extractor package (TDD)

**Files:**
- Create: `internal/core/content/extractor.go`
- Test: `internal/core/content/extractor_test.go`

- [x] **Step 1: Add go-readability dependency**

```bash
go get github.com/go-shiori/go-readability
```

Expected: package downloaded, `go.mod` and `go.sum` updated.

- [x] **Step 2: Write the failing test**

Create `internal/core/content/extractor_test.go`:
```go
package content_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ismd/linktheca/internal/core/content"
	"github.com/stretchr/testify/require"
)

const testHTML = `<!DOCTYPE html>
<html>
<head><title>Test Article</title></head>
<body>
<article>
<h1>Test Article</h1>
<p>By John Doe</p>
<p>This is the first paragraph of a test article. It contains enough text
to be recognized as real content by the readability algorithm. We need
several sentences to make this work properly.</p>
<p>This is the second paragraph. It also contains meaningful content that
should be extracted by the readability parser. The more text we have here,
the better the extraction will work.</p>
<p>And a third paragraph for good measure. Content extraction algorithms
typically need a reasonable amount of text to distinguish article content
from boilerplate navigation and footer elements.</p>
</article>
</body>
</html>`

func TestExtractFromURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(testHTML))
	}))
	defer srv.Close()

	ext := content.NewExtractor()
	result, err := ext.Extract(context.Background(), srv.URL)

	require.NoError(t, err)
	require.Equal(t, srv.URL, result.URL)
	require.Contains(t, result.Title, "Test Article")
	require.NotEmpty(t, result.Text)
	require.NotEmpty(t, result.HTML)
}

func TestExtractFromURLFetchError(t *testing.T) {
	ext := content.NewExtractor()
	_, err := ext.Extract(context.Background(), "http://127.0.0.1:1/nonexistent")

	require.Error(t, err)
}

func TestReadingTimeEstimation(t *testing.T) {
	require.Equal(t, 0, content.EstimateReadingTime(""))
	require.Equal(t, 1, content.EstimateReadingTime("short text"))

	// ~200 words → should be about 1 minute (200 WPM)
	long := ""
	for i := 0; i < 200; i++ {
		long += "word "
	}
	got := content.EstimateReadingTime(long)
	require.GreaterOrEqual(t, got, 55)
	require.LessOrEqual(t, got, 65)
}
```

- [x] **Step 3: Run test to verify it fails**

```bash
go test ./internal/core/content/... -v
```

Expected: FAIL — package does not exist.

- [x] **Step 4: Write the implementation**

Create `internal/core/content/extractor.go`:
```go
package content

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	readability "github.com/go-shiori/go-readability"
)

// Article holds extracted content from a URL.
type Article struct {
	URL              string
	CanonicalURL     string
	Title            string
	Byline           string
	Excerpt          string
	Text             string
	HTML             string
	Lang             string
	ReadingTimeSecs  int
}

// Extractor fetches a URL and extracts its readable content.
type Extractor interface {
	Extract(ctx context.Context, url string) (*Article, error)
}

type readabilityExtractor struct {
	client *http.Client
}

// NewExtractor creates an Extractor backed by go-readability.
func NewExtractor() Extractor {
	return &readabilityExtractor{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (e *readabilityExtractor) Extract(ctx context.Context, rawURL string) (*Article, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "Linktheca/1.0")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch: status %d", resp.StatusCode)
	}

	doc, err := readability.FromReader(resp.Body, resp.Request.URL)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	return &Article{
		URL:             rawURL,
		CanonicalURL:    "",
		Title:           doc.Title,
		Byline:          doc.Byline,
		Excerpt:         doc.Excerpt,
		Text:            doc.TextContent,
		HTML:            doc.Content,
		Lang:            doc.Language,
		ReadingTimeSecs: EstimateReadingTime(doc.TextContent),
	}, nil
}

// EstimateReadingTime returns estimated reading time in seconds.
// Average reading speed: ~200 words per minute.
func EstimateReadingTime(text string) int {
	words := len(strings.Fields(text))
	if words == 0 {
		return 0
	}
	secs := (words * 60) / 200
	if secs == 0 {
		secs = 1
	}
	return secs
}
```

- [x] **Step 5: Run tests to verify pass**

```bash
go test ./internal/core/content/... -v
```

Expected: PASS for all three tests.

- [x] **Step 6: Commit**

```bash
go mod tidy
git add go.mod go.sum internal/core/content/
git commit -m "feat(content): article extractor with go-readability"
```

---

## Part C: Library types

### Task 4: Library module types

**Files:**
- Create: `internal/library/types.go`

- [ ] **Step 1: Create types file**

Create `internal/library/types.go`:
```go
package library

import "time"

// ArticleContent is the shared content cache entry.
type ArticleContent struct {
	ID               int64      `json:"id"`
	URL              string     `json:"url"`
	CanonicalURL     *string    `json:"canonical_url,omitempty"`
	Title            *string    `json:"title,omitempty"`
	Byline           *string    `json:"byline,omitempty"`
	Excerpt          *string    `json:"excerpt,omitempty"`
	Text             *string    `json:"text,omitempty"`
	HTML             *string    `json:"html,omitempty"`
	Lang             *string    `json:"lang,omitempty"`
	ReadingTimeSecs  *int       `json:"reading_time_seconds,omitempty"`
	FetchedAt        time.Time  `json:"fetched_at"`
	FetchError       *string    `json:"fetch_error,omitempty"`
}

// Item is a user's saved article in the library.
type Item struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"user_id"`
	ContentID  int64      `json:"content_id"`
	State      string     `json:"state"`
	IsFavorite bool       `json:"is_favorite"`
	Note       *string    `json:"note,omitempty"`
	SavedAt    time.Time  `json:"saved_at"`
	ReadAt     *time.Time `json:"read_at,omitempty"`

	// Joined from article_contents (populated in list/get queries).
	URL          string  `json:"url"`
	Title        *string `json:"title,omitempty"`
	Excerpt      *string `json:"excerpt,omitempty"`
	ReadTimeSecs *int    `json:"reading_time_seconds,omitempty"`
}

// SaveRequest is the payload for POST /library.
type SaveRequest struct {
	URL string `json:"url"`
}

// UpdateRequest is the payload for PATCH /library/:id.
type UpdateRequest struct {
	State      *string `json:"state,omitempty"`
	IsFavorite *bool   `json:"is_favorite,omitempty"`
	Note       *string `json:"note,omitempty"`
}

// ListParams holds query parameters for GET /library.
type ListParams struct {
	UserID    int64
	State     string // empty = all states
	Favorite  *bool  // nil = don't filter
	Limit     int
	Offset    int
}

// ListResult holds the paginated response for GET /library.
type ListResult struct {
	Items []Item `json:"items"`
	Total int    `json:"total"`
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/library/...
```

Expected: no output, no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/library/types.go
git commit -m "feat(library): domain types and DTOs"
```

---

## Part D: Library store

### Task 5: Library store — UpsertContent and CreateItem (TDD)

**Files:**
- Create: `internal/library/store.go`
- Test: `internal/library/store_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/library/store_test.go`:
```go
package library_test

import (
	"context"
	"testing"

	"github.com/ismd/linktheca/internal/library"
	"github.com/ismd/linktheca/internal/testing/testdb"
	"github.com/stretchr/testify/require"
)

func TestIntegrationUpsertContent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	// First insert
	c1, err := store.UpsertContent(ctx, library.UpsertContentParams{
		URL:             "https://example.com/article-1",
		Title:           ptr("Test Article"),
		Text:            ptr("Some text content here."),
		HTML:            ptr("<p>Some text content here.</p>"),
		ReadingTimeSecs: intPtr(60),
	})
	require.NoError(t, err)
	require.Equal(t, "https://example.com/article-1", c1.URL)
	require.NotZero(t, c1.ID)

	// Second insert with same URL — returns existing row
	c2, err := store.UpsertContent(ctx, library.UpsertContentParams{
		URL:   "https://example.com/article-1",
		Title: ptr("Updated Title"),
	})
	require.NoError(t, err)
	require.Equal(t, c1.ID, c2.ID, "upsert must return the same row for the same URL")
}

func TestIntegrationUpsertContentWithFetchError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	c, err := store.UpsertContent(ctx, library.UpsertContentParams{
		URL:        "https://example.com/broken",
		FetchError: ptr("connection refused"),
	})
	require.NoError(t, err)
	require.NotZero(t, c.ID)
	require.NotNil(t, c.FetchError)
	require.Equal(t, "connection refused", *c.FetchError)
}

func TestIntegrationCreateItem(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	userID := createTestUser(t, pool)

	content, err := store.UpsertContent(ctx, library.UpsertContentParams{
		URL:   "https://example.com/save-me",
		Title: ptr("Saved Article"),
	})
	require.NoError(t, err)

	item, err := store.CreateItem(ctx, userID, content.ID)
	require.NoError(t, err)
	require.Equal(t, userID, item.UserID)
	require.Equal(t, content.ID, item.ContentID)
	require.Equal(t, "unread", item.State)
	require.False(t, item.IsFavorite)
}

func TestIntegrationCreateItemDuplicate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	userID := createTestUser(t, pool)

	content, err := store.UpsertContent(ctx, library.UpsertContentParams{
		URL:   "https://example.com/dup",
		Title: ptr("Dup Article"),
	})
	require.NoError(t, err)

	_, err = store.CreateItem(ctx, userID, content.ID)
	require.NoError(t, err)

	_, err = store.CreateItem(ctx, userID, content.ID)
	require.ErrorIs(t, err, library.ErrAlreadySaved)
}

// createTestUser inserts a user directly into the DB for test setup.
func createTestUser(t *testing.T, pool interface{ Exec(ctx context.Context, sql string, args ...any) (interface{ RowsAffected() int64 }, error) }) int64 {
	t.Helper()
	ctx := context.Background()
	var id int64
	// Use the pool directly with QueryRow
	type queryRower interface {
		QueryRow(ctx context.Context, sql string, args ...any) interface{ Scan(dest ...any) error }
	}
	// Simpler approach: cast to pgxpool.Pool
	err := pool.(*library.TestablePool).QueryRow(ctx,
		`INSERT INTO users (email, password_hash, display_name) VALUES ($1, $2, $3) RETURNING id`,
		"test@example.com", "fakehash", "Test User",
	).Scan(&id)
	require.NoError(t, err)
	return id
}

func ptr(s string) *string { return &s }
func intPtr(n int) *int    { return &n }
```

Wait — the `createTestUser` helper above has a problem: we can't cast to a custom type. Let me fix the test to use `pgxpool.Pool` directly.

Replace the entire `store_test.go` with this corrected version:

Create `internal/library/store_test.go`:
```go
package library_test

import (
	"context"
	"testing"

	"github.com/ismd/linktheca/internal/library"
	"github.com/ismd/linktheca/internal/testing/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestIntegrationUpsertContent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	// First insert
	c1, err := store.UpsertContent(ctx, library.UpsertContentParams{
		URL:             "https://example.com/article-1",
		Title:           ptr("Test Article"),
		Text:            ptr("Some text content here."),
		HTML:            ptr("<p>Some text content here.</p>"),
		ReadingTimeSecs: intPtr(60),
	})
	require.NoError(t, err)
	require.Equal(t, "https://example.com/article-1", c1.URL)
	require.NotZero(t, c1.ID)

	// Second insert with same URL — returns existing row without updating
	c2, err := store.UpsertContent(ctx, library.UpsertContentParams{
		URL:   "https://example.com/article-1",
		Title: ptr("Updated Title"),
	})
	require.NoError(t, err)
	require.Equal(t, c1.ID, c2.ID, "upsert must return the same row for the same URL")
}

func TestIntegrationUpsertContentWithFetchError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	c, err := store.UpsertContent(ctx, library.UpsertContentParams{
		URL:        "https://example.com/broken",
		FetchError: ptr("connection refused"),
	})
	require.NoError(t, err)
	require.NotZero(t, c.ID)
	require.NotNil(t, c.FetchError)
	require.Equal(t, "connection refused", *c.FetchError)
}

func TestIntegrationCreateItem(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	userID := createTestUser(t, pool)

	content, err := store.UpsertContent(ctx, library.UpsertContentParams{
		URL:   "https://example.com/save-me",
		Title: ptr("Saved Article"),
	})
	require.NoError(t, err)

	item, err := store.CreateItem(ctx, userID, content.ID)
	require.NoError(t, err)
	require.Equal(t, userID, item.UserID)
	require.Equal(t, content.ID, item.ContentID)
	require.Equal(t, "unread", item.State)
	require.False(t, item.IsFavorite)
}

func TestIntegrationCreateItemDuplicate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	userID := createTestUser(t, pool)

	content, err := store.UpsertContent(ctx, library.UpsertContentParams{
		URL:   "https://example.com/dup",
		Title: ptr("Dup Article"),
	})
	require.NoError(t, err)

	_, err = store.CreateItem(ctx, userID, content.ID)
	require.NoError(t, err)

	_, err = store.CreateItem(ctx, userID, content.ID)
	require.ErrorIs(t, err, library.ErrAlreadySaved)
}

func createTestUser(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email, password_hash, display_name) VALUES ($1, $2, $3) RETURNING id`,
		"test@example.com", "fakehash", "Test User",
	).Scan(&id)
	require.NoError(t, err)
	return id
}

func ptr(s string) *string { return &s }
func intPtr(n int) *int    { return &n }
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/library/... -v
```

Expected: FAIL — package `library` has no Go files / types not found.

- [ ] **Step 3: Write the store implementation**

Create `internal/library/store.go`:
```go
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

// UpsertContent inserts a new article_contents row or returns the existing one if the URL already exists.
func (s *Store) UpsertContent(ctx context.Context, p UpsertContentParams) (*ArticleContent, error) {
	row := s.db.QueryRow(ctx, `
		INSERT INTO article_contents (url, canonical_url, title, byline, excerpt, text, html, lang, reading_time_seconds, fetch_error)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (url) DO UPDATE SET url = EXCLUDED.url
		RETURNING id, url, canonical_url, title, byline, excerpt, text, html, lang, reading_time_seconds, fetched_at, fetch_error
	`, p.URL, p.CanonicalURL, p.Title, p.Byline, p.Excerpt, p.Text, p.HTML, p.Lang, p.ReadingTimeSecs, p.FetchError)

	return scanContent(row)
}

// GetContentByURL returns the article_contents row for a URL, or ErrNotFound.
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
		RETURNING id, user_id, content_id, state, is_favorite, note, saved_at, read_at
	`, userID, contentID)

	var item Item
	err := row.Scan(&item.ID, &item.UserID, &item.ContentID, &item.State,
		&item.IsFavorite, &item.Note, &item.SavedAt, &item.ReadAt)
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
```

- [ ] **Step 4: Run tests to verify pass**

```bash
go test ./internal/library/... -v -count=1
```

Expected: PASS for all four tests.

- [ ] **Step 5: Commit**

```bash
git add internal/library/store.go internal/library/store_test.go
git commit -m "feat(library): store with UpsertContent and CreateItem"
```

---

### Task 6: Library store — List, GetByID, Update, Delete (TDD)

**Files:**
- Modify: `internal/library/store.go`
- Modify: `internal/library/store_test.go`

- [ ] **Step 1: Add integration tests for List, GetByID, Update, Delete**

Append to `internal/library/store_test.go`:
```go
func TestIntegrationListItems(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	userID := createTestUser(t, pool)

	// Save 3 articles
	for i := 0; i < 3; i++ {
		c, err := store.UpsertContent(ctx, library.UpsertContentParams{
			URL:   fmt.Sprintf("https://example.com/list-%d", i),
			Title: ptr(fmt.Sprintf("Article %d", i)),
		})
		require.NoError(t, err)
		_, err = store.CreateItem(ctx, userID, c.ID)
		require.NoError(t, err)
	}

	result, err := store.ListItems(ctx, library.ListParams{
		UserID: userID,
		Limit:  10,
		Offset: 0,
	})
	require.NoError(t, err)
	require.Equal(t, 3, result.Total)
	require.Len(t, result.Items, 3)
	// Items come with joined content fields
	require.NotEmpty(t, result.Items[0].URL)
}

func TestIntegrationListItemsByState(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	userID := createTestUser(t, pool)

	c1, _ := store.UpsertContent(ctx, library.UpsertContentParams{URL: "https://example.com/s1", Title: ptr("S1")})
	c2, _ := store.UpsertContent(ctx, library.UpsertContentParams{URL: "https://example.com/s2", Title: ptr("S2")})

	item1, _ := store.CreateItem(ctx, userID, c1.ID)
	_, _ = store.CreateItem(ctx, userID, c2.ID)

	// Mark first as read
	state := "read"
	_, err := store.UpdateItem(ctx, userID, item1.ID, library.UpdateParams{State: &state})
	require.NoError(t, err)

	// Filter by state=unread
	result, err := store.ListItems(ctx, library.ListParams{
		UserID: userID,
		State:  "unread",
		Limit:  10,
	})
	require.NoError(t, err)
	require.Equal(t, 1, result.Total)
}

func TestIntegrationListItemsPagination(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	userID := createTestUser(t, pool)

	for i := 0; i < 5; i++ {
		c, _ := store.UpsertContent(ctx, library.UpsertContentParams{
			URL:   fmt.Sprintf("https://example.com/page-%d", i),
			Title: ptr(fmt.Sprintf("Page %d", i)),
		})
		_, _ = store.CreateItem(ctx, userID, c.ID)
	}

	result, err := store.ListItems(ctx, library.ListParams{
		UserID: userID,
		Limit:  2,
		Offset: 0,
	})
	require.NoError(t, err)
	require.Equal(t, 5, result.Total)
	require.Len(t, result.Items, 2)

	result2, err := store.ListItems(ctx, library.ListParams{
		UserID: userID,
		Limit:  2,
		Offset: 2,
	})
	require.NoError(t, err)
	require.Equal(t, 5, result2.Total)
	require.Len(t, result2.Items, 2)
}

func TestIntegrationGetItemByID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	userID := createTestUser(t, pool)

	content, _ := store.UpsertContent(ctx, library.UpsertContentParams{
		URL:   "https://example.com/get-me",
		Title: ptr("Get Me"),
		Text:  ptr("Full article text here."),
		HTML:  ptr("<p>Full article text here.</p>"),
	})
	item, _ := store.CreateItem(ctx, userID, content.ID)

	got, err := store.GetItemByID(ctx, userID, item.ID)
	require.NoError(t, err)
	require.Equal(t, item.ID, got.ID)
	require.Equal(t, "https://example.com/get-me", got.URL)
	require.NotNil(t, got.Title)
	require.Equal(t, "Get Me", *got.Title)
}

func TestIntegrationGetItemByIDNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)

	_, err := store.GetItemByID(context.Background(), 999, 999)
	require.ErrorIs(t, err, library.ErrNotFound)
}

func TestIntegrationGetItemByIDAnotherUser(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	user1 := createTestUser(t, pool)
	user2 := createTestUserWithEmail(t, pool, "user2@example.com")

	content, _ := store.UpsertContent(ctx, library.UpsertContentParams{URL: "https://example.com/private", Title: ptr("Private")})
	item, _ := store.CreateItem(ctx, user1, content.ID)

	// user2 should not see user1's item
	_, err := store.GetItemByID(ctx, user2, item.ID)
	require.ErrorIs(t, err, library.ErrNotFound)
}

func TestIntegrationUpdateItem(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	userID := createTestUser(t, pool)

	content, _ := store.UpsertContent(ctx, library.UpsertContentParams{URL: "https://example.com/update-me", Title: ptr("Update Me")})
	item, _ := store.CreateItem(ctx, userID, content.ID)

	state := "read"
	fav := true
	note := "Great article!"
	updated, err := store.UpdateItem(ctx, userID, item.ID, library.UpdateParams{
		State:      &state,
		IsFavorite: &fav,
		Note:       &note,
	})
	require.NoError(t, err)
	require.Equal(t, "read", updated.State)
	require.True(t, updated.IsFavorite)
	require.NotNil(t, updated.Note)
	require.Equal(t, "Great article!", *updated.Note)
	require.NotNil(t, updated.ReadAt, "read_at should be set when state becomes read")
}

func TestIntegrationUpdateItemPartial(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	userID := createTestUser(t, pool)

	content, _ := store.UpsertContent(ctx, library.UpsertContentParams{URL: "https://example.com/partial", Title: ptr("Partial")})
	item, _ := store.CreateItem(ctx, userID, content.ID)

	// Only update favorite, leave state and note unchanged
	fav := true
	updated, err := store.UpdateItem(ctx, userID, item.ID, library.UpdateParams{
		IsFavorite: &fav,
	})
	require.NoError(t, err)
	require.True(t, updated.IsFavorite)
	require.Equal(t, "unread", updated.State, "state must not change")
}

func TestIntegrationDeleteItem(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	userID := createTestUser(t, pool)

	content, _ := store.UpsertContent(ctx, library.UpsertContentParams{URL: "https://example.com/delete-me", Title: ptr("Delete Me")})
	item, _ := store.CreateItem(ctx, userID, content.ID)

	err := store.DeleteItem(ctx, userID, item.ID)
	require.NoError(t, err)

	_, err = store.GetItemByID(ctx, userID, item.ID)
	require.ErrorIs(t, err, library.ErrNotFound)
}

func TestIntegrationDeleteItemNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)

	err := store.DeleteItem(context.Background(), 999, 999)
	require.ErrorIs(t, err, library.ErrNotFound)
}

func createTestUserWithEmail(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email, password_hash, display_name) VALUES ($1, $2, $3) RETURNING id`,
		email, "fakehash", "Test User",
	).Scan(&id)
	require.NoError(t, err)
	return id
}
```

Also add `"fmt"` to the imports at the top of the test file (needed by `TestIntegrationListItems`).

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/library/... -v -count=1
```

Expected: FAIL — `ListItems`, `GetItemByID`, `UpdateItem`, `DeleteItem` not defined.

- [ ] **Step 3: Add UpdateParams type to types.go**

Append to `internal/library/types.go`:
```go
// UpdateParams holds the fields that can be changed via store.UpdateItem.
type UpdateParams struct {
	State      *string
	IsFavorite *bool
	Note       *string
}
```

- [ ] **Step 4: Implement ListItems, GetItemByID, UpdateItem, DeleteItem**

Append to `internal/library/store.go`:
```go
// GetItemByID returns a single library item with joined content fields.
// Only returns items belonging to the given user.
func (s *Store) GetItemByID(ctx context.Context, userID, itemID int64) (*Item, error) {
	row := s.db.QueryRow(ctx, `
		SELECT li.id, li.user_id, li.content_id, li.state, li.is_favorite, li.note, li.saved_at, li.read_at,
		       ac.url, ac.title, ac.excerpt, ac.reading_time_seconds
		FROM library_items li
		JOIN article_contents ac ON ac.id = li.content_id
		WHERE li.id = $1 AND li.user_id = $2
	`, itemID, userID)

	return scanItem(row)
}

// ListItems returns paginated library items for a user with optional state filter.
func (s *Store) ListItems(ctx context.Context, p ListParams) (*ListResult, error) {
	// Count total matching rows.
	countQuery := `SELECT count(*) FROM library_items WHERE user_id = $1`
	countArgs := []any{p.UserID}
	argIdx := 2

	if p.State != "" {
		countQuery += fmt.Sprintf(` AND state = $%d`, argIdx)
		countArgs = append(countArgs, p.State)
		argIdx++
	}
	if p.Favorite != nil {
		countQuery += fmt.Sprintf(` AND is_favorite = $%d`, argIdx)
		countArgs = append(countArgs, *p.Favorite)
		argIdx++
	}

	var total int
	if err := s.db.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count items: %w", err)
	}

	// Fetch items page.
	query := `
		SELECT li.id, li.user_id, li.content_id, li.state, li.is_favorite, li.note, li.saved_at, li.read_at,
		       ac.url, ac.title, ac.excerpt, ac.reading_time_seconds
		FROM library_items li
		JOIN article_contents ac ON ac.id = li.content_id
		WHERE li.user_id = $1`
	args := []any{p.UserID}
	argIdx = 2

	if p.State != "" {
		query += fmt.Sprintf(` AND li.state = $%d`, argIdx)
		args = append(args, p.State)
		argIdx++
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

// UpdateItem partially updates a library item. Only non-nil fields are changed.
// When state changes to "read", read_at is set to now(). When state changes away from "read", read_at is cleared.
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
	if p.Note != nil {
		setClauses = append(setClauses, fmt.Sprintf("note = $%d", argIdx))
		args = append(args, *p.Note)
		argIdx++
	}

	if len(setClauses) == 0 {
		return s.GetItemByID(ctx, userID, itemID)
	}

	query := fmt.Sprintf(`UPDATE library_items SET %s WHERE id = $%d AND user_id = $%d
		RETURNING id, user_id, content_id, state, is_favorite, note, saved_at, read_at`,
		strings.Join(setClauses, ", "), argIdx, argIdx+1)
	args = append(args, itemID, userID)

	var item Item
	err := s.db.QueryRow(ctx, query, args...).Scan(
		&item.ID, &item.UserID, &item.ContentID, &item.State,
		&item.IsFavorite, &item.Note, &item.SavedAt, &item.ReadAt)
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
		&item.IsFavorite, &item.Note, &item.SavedAt, &item.ReadAt,
		&item.URL, &item.Title, &item.Excerpt, &item.ReadTimeSecs)
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
		&item.IsFavorite, &item.Note, &item.SavedAt, &item.ReadAt,
		&item.URL, &item.Title, &item.Excerpt, &item.ReadTimeSecs)
	if err != nil {
		return nil, err
	}
	return &item, nil
}
```

Also add `"strings"` to the imports in `store.go`.

- [ ] **Step 5: Run tests to verify pass**

```bash
go test ./internal/library/... -v -count=1
```

Expected: PASS for all tests.

- [ ] **Step 6: Commit**

```bash
git add internal/library/store.go internal/library/store_test.go internal/library/types.go
git commit -m "feat(library): store CRUD — List, GetByID, Update, Delete with integration tests"
```

---

## Part E: Library service

### Task 7: Library service — SaveURL (TDD, unit with mock store)

**Files:**
- Create: `internal/library/service.go`
- Create: `internal/library/service_test.go`

- [ ] **Step 1: Write the mock store and failing tests**

Create `internal/library/service_test.go`:
```go
package library_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ismd/linktheca/internal/core/content"
	"github.com/ismd/linktheca/internal/library"
	"github.com/stretchr/testify/require"
)

// --- mock store ---

type mockStore struct {
	contents   map[string]*library.ArticleContent
	items      map[int64]*library.Item
	nextCID    int64
	nextItemID int64
}

func newMockStore() *mockStore {
	return &mockStore{
		contents: make(map[string]*library.ArticleContent),
		items:    make(map[int64]*library.Item),
	}
}

func (m *mockStore) UpsertContent(_ context.Context, p library.UpsertContentParams) (*library.ArticleContent, error) {
	if c, ok := m.contents[p.URL]; ok {
		return c, nil
	}
	m.nextCID++
	c := &library.ArticleContent{
		ID:    m.nextCID,
		URL:   p.URL,
		Title: p.Title,
	}
	m.contents[p.URL] = c
	return c, nil
}

func (m *mockStore) GetContentByURL(_ context.Context, url string) (*library.ArticleContent, error) {
	c, ok := m.contents[url]
	if !ok {
		return nil, library.ErrNotFound
	}
	return c, nil
}

func (m *mockStore) CreateItem(_ context.Context, userID, contentID int64) (*library.Item, error) {
	for _, item := range m.items {
		if item.UserID == userID && item.ContentID == contentID {
			return nil, library.ErrAlreadySaved
		}
	}
	m.nextItemID++
	item := &library.Item{
		ID:        m.nextItemID,
		UserID:    userID,
		ContentID: contentID,
		State:     "unread",
		URL:       m.contentURL(contentID),
		Title:     m.contentTitle(contentID),
	}
	m.items[item.ID] = item
	return item, nil
}

func (m *mockStore) GetItemByID(_ context.Context, userID, itemID int64) (*library.Item, error) {
	item, ok := m.items[itemID]
	if !ok || item.UserID != userID {
		return nil, library.ErrNotFound
	}
	return item, nil
}

func (m *mockStore) ListItems(_ context.Context, p library.ListParams) (*library.ListResult, error) {
	var items []library.Item
	for _, item := range m.items {
		if item.UserID != p.UserID {
			continue
		}
		if p.State != "" && item.State != p.State {
			continue
		}
		items = append(items, *item)
	}
	return &library.ListResult{Items: items, Total: len(items)}, nil
}

func (m *mockStore) UpdateItem(_ context.Context, userID, itemID int64, p library.UpdateParams) (*library.Item, error) {
	item, ok := m.items[itemID]
	if !ok || item.UserID != userID {
		return nil, library.ErrNotFound
	}
	if p.State != nil {
		item.State = *p.State
	}
	if p.IsFavorite != nil {
		item.IsFavorite = *p.IsFavorite
	}
	if p.Note != nil {
		item.Note = p.Note
	}
	return item, nil
}

func (m *mockStore) DeleteItem(_ context.Context, userID, itemID int64) error {
	item, ok := m.items[itemID]
	if !ok || item.UserID != userID {
		return library.ErrNotFound
	}
	delete(m.items, itemID)
	return nil
}

func (m *mockStore) contentURL(contentID int64) string {
	for _, c := range m.contents {
		if c.ID == contentID {
			return c.URL
		}
	}
	return ""
}

func (m *mockStore) contentTitle(contentID int64) *string {
	for _, c := range m.contents {
		if c.ID == contentID {
			return c.Title
		}
	}
	return nil
}

// --- mock extractor ---

type mockExtractor struct {
	results map[string]*content.Article
	err     error
}

func newMockExtractor() *mockExtractor {
	return &mockExtractor{results: make(map[string]*content.Article)}
}

func (m *mockExtractor) Extract(_ context.Context, url string) (*content.Article, error) {
	if m.err != nil {
		return nil, m.err
	}
	if a, ok := m.results[url]; ok {
		return a, nil
	}
	return &content.Article{
		URL:   url,
		Title: "Extracted: " + url,
		Text:  "Some extracted text for " + url,
		HTML:  "<p>Some extracted text for " + url + "</p>",
	}, nil
}

// --- tests ---

func TestServiceSaveURL(t *testing.T) {
	store := newMockStore()
	ext := newMockExtractor()
	svc := library.NewService(store, ext)

	item, err := svc.SaveURL(context.Background(), 1, "https://example.com/article")
	require.NoError(t, err)
	require.Equal(t, int64(1), item.UserID)
	require.Equal(t, "https://example.com/article", item.URL)
	require.Equal(t, "unread", item.State)
}

func TestServiceSaveURLDuplicate(t *testing.T) {
	store := newMockStore()
	ext := newMockExtractor()
	svc := library.NewService(store, ext)

	_, err := svc.SaveURL(context.Background(), 1, "https://example.com/dup")
	require.NoError(t, err)

	_, err = svc.SaveURL(context.Background(), 1, "https://example.com/dup")
	require.ErrorIs(t, err, library.ErrAlreadySaved)
}

func TestServiceSaveURLExtractionFailure(t *testing.T) {
	store := newMockStore()
	ext := newMockExtractor()
	ext.err = errors.New("network error")
	svc := library.NewService(store, ext)

	// Even if extraction fails, we still save the item with whatever we got (URL-only record with fetch_error)
	item, err := svc.SaveURL(context.Background(), 1, "https://example.com/broken")
	require.NoError(t, err)
	require.Equal(t, "https://example.com/broken", item.URL)
}

// compile-time interface check
var _ library.StoreAPI = (*mockStore)(nil)
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/library/... -v -count=1 -run TestService
```

Expected: FAIL — `library.NewService`, `library.StoreAPI` not defined.

- [ ] **Step 3: Write the service implementation**

Create `internal/library/service.go`:
```go
package library

import (
	"context"
	"fmt"

	"github.com/ismd/linktheca/internal/core/content"
)

type StoreAPI interface {
	UpsertContent(ctx context.Context, p UpsertContentParams) (*ArticleContent, error)
	GetContentByURL(ctx context.Context, url string) (*ArticleContent, error)
	CreateItem(ctx context.Context, userID, contentID int64) (*Item, error)
	GetItemByID(ctx context.Context, userID, itemID int64) (*Item, error)
	ListItems(ctx context.Context, p ListParams) (*ListResult, error)
	UpdateItem(ctx context.Context, userID, itemID int64, p UpdateParams) (*Item, error)
	DeleteItem(ctx context.Context, userID, itemID int64) error
}

type Service struct {
	store     StoreAPI
	extractor content.Extractor
}

func NewService(store StoreAPI, extractor content.Extractor) *Service {
	return &Service{store: store, extractor: extractor}
}

// SaveURL extracts content from the URL and saves it to the user's library.
// If extraction fails, we still create a record with the URL and the fetch error.
func (s *Service) SaveURL(ctx context.Context, userID int64, rawURL string) (*Item, error) {
	var params UpsertContentParams

	article, extractErr := s.extractor.Extract(ctx, rawURL)
	if extractErr != nil {
		errMsg := extractErr.Error()
		params = UpsertContentParams{
			URL:        rawURL,
			FetchError: &errMsg,
		}
	} else {
		params = UpsertContentParams{
			URL:             article.URL,
			CanonicalURL:    nilIfEmpty(article.CanonicalURL),
			Title:           nilIfEmpty(article.Title),
			Byline:          nilIfEmpty(article.Byline),
			Excerpt:         nilIfEmpty(article.Excerpt),
			Text:            nilIfEmpty(article.Text),
			HTML:            nilIfEmpty(article.HTML),
			Lang:            nilIfEmpty(article.Lang),
			ReadingTimeSecs: nilIfZero(article.ReadingTimeSecs),
		}
	}

	ac, err := s.store.UpsertContent(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("upsert content: %w", err)
	}

	item, err := s.store.CreateItem(ctx, userID, ac.ID)
	if err != nil {
		return nil, err
	}

	return item, nil
}

// List returns paginated library items for a user.
func (s *Service) List(ctx context.Context, p ListParams) (*ListResult, error) {
	return s.store.ListItems(ctx, p)
}

// GetByID returns a single library item with full content.
func (s *Service) GetByID(ctx context.Context, userID, itemID int64) (*Item, error) {
	return s.store.GetItemByID(ctx, userID, itemID)
}

// Update partially updates a library item.
func (s *Service) Update(ctx context.Context, userID, itemID int64, req UpdateRequest) (*Item, error) {
	p := UpdateParams{
		State:      req.State,
		IsFavorite: req.IsFavorite,
		Note:       req.Note,
	}

	if p.State != nil {
		switch *p.State {
		case "unread", "read", "archived":
			// valid
		default:
			return nil, fmt.Errorf("invalid state: %s", *p.State)
		}
	}

	return s.store.UpdateItem(ctx, userID, itemID, p)
}

// Delete removes a library item.
func (s *Service) Delete(ctx context.Context, userID, itemID int64) error {
	return s.store.DeleteItem(ctx, userID, itemID)
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
```

- [ ] **Step 4: Run tests to verify pass**

```bash
go test ./internal/library/... -v -count=1 -run TestService
```

Expected: PASS for `TestServiceSaveURL`, `TestServiceSaveURLDuplicate`, `TestServiceSaveURLExtractionFailure`.

- [ ] **Step 5: Commit**

```bash
git add internal/library/service.go internal/library/service_test.go
git commit -m "feat(library): service with SaveURL, List, GetByID, Update, Delete"
```

---

### Task 8: Library service — List, GetByID, Update, Delete unit tests

**Files:**
- Modify: `internal/library/service_test.go`

- [ ] **Step 1: Add unit tests for remaining service methods**

Append to `internal/library/service_test.go`:
```go
func TestServiceList(t *testing.T) {
	store := newMockStore()
	ext := newMockExtractor()
	svc := library.NewService(store, ext)

	_, _ = svc.SaveURL(context.Background(), 1, "https://example.com/a")
	_, _ = svc.SaveURL(context.Background(), 1, "https://example.com/b")

	result, err := svc.List(context.Background(), library.ListParams{
		UserID: 1,
		Limit:  10,
	})
	require.NoError(t, err)
	require.Equal(t, 2, result.Total)
}

func TestServiceGetByID(t *testing.T) {
	store := newMockStore()
	ext := newMockExtractor()
	svc := library.NewService(store, ext)

	item, _ := svc.SaveURL(context.Background(), 1, "https://example.com/get")

	got, err := svc.GetByID(context.Background(), 1, item.ID)
	require.NoError(t, err)
	require.Equal(t, item.ID, got.ID)
}

func TestServiceGetByIDNotFound(t *testing.T) {
	store := newMockStore()
	ext := newMockExtractor()
	svc := library.NewService(store, ext)

	_, err := svc.GetByID(context.Background(), 1, 999)
	require.ErrorIs(t, err, library.ErrNotFound)
}

func TestServiceUpdate(t *testing.T) {
	store := newMockStore()
	ext := newMockExtractor()
	svc := library.NewService(store, ext)

	item, _ := svc.SaveURL(context.Background(), 1, "https://example.com/upd")

	state := "read"
	updated, err := svc.Update(context.Background(), 1, item.ID, library.UpdateRequest{State: &state})
	require.NoError(t, err)
	require.Equal(t, "read", updated.State)
}

func TestServiceUpdateInvalidState(t *testing.T) {
	store := newMockStore()
	ext := newMockExtractor()
	svc := library.NewService(store, ext)

	item, _ := svc.SaveURL(context.Background(), 1, "https://example.com/bad")

	bad := "invalid"
	_, err := svc.Update(context.Background(), 1, item.ID, library.UpdateRequest{State: &bad})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid state")
}

func TestServiceDelete(t *testing.T) {
	store := newMockStore()
	ext := newMockExtractor()
	svc := library.NewService(store, ext)

	item, _ := svc.SaveURL(context.Background(), 1, "https://example.com/del")

	err := svc.Delete(context.Background(), 1, item.ID)
	require.NoError(t, err)

	_, err = svc.GetByID(context.Background(), 1, item.ID)
	require.ErrorIs(t, err, library.ErrNotFound)
}

func TestServiceDeleteNotFound(t *testing.T) {
	store := newMockStore()
	ext := newMockExtractor()
	svc := library.NewService(store, ext)

	err := svc.Delete(context.Background(), 1, 999)
	require.ErrorIs(t, err, library.ErrNotFound)
}
```

- [ ] **Step 2: Run all service tests**

```bash
go test ./internal/library/... -v -count=1 -run TestService
```

Expected: PASS for all service tests.

- [ ] **Step 3: Commit**

```bash
git add internal/library/service_test.go
git commit -m "test(library): complete unit tests for service methods"
```

---

## Part F: HTTP handlers

### Task 9: Library HTTP handlers (TDD)

**Files:**
- Create: `internal/library/http.go`
- Create: `internal/library/http_test.go`

- [ ] **Step 1: Write the failing HTTP integration tests**

Create `internal/library/http_test.go`:
```go
package library_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	coreauth "github.com/ismd/linktheca/internal/core/auth"
	"github.com/ismd/linktheca/internal/library"
	"github.com/stretchr/testify/require"
)

func setupHTTPTest(t *testing.T) (*chi.Mux, *coreauth.JWTIssuer) {
	t.Helper()

	store := newMockStore()
	ext := newMockExtractor()
	svc := library.NewService(store, ext)

	issuer := coreauth.NewJWTIssuer("test-secret-at-least-32-bytes-long-for-hmac", 15*time.Minute)
	h := library.NewHTTP(svc)

	r := chi.NewRouter()
	r.Route("/library", func(r chi.Router) {
		r.Use(coreauth.RequireUser(issuer))
		r.Post("/", h.SaveHandler())
		r.Get("/", h.ListHandler())
		r.Get("/{id}", h.GetHandler())
		r.Patch("/{id}", h.UpdateHandler())
		r.Delete("/{id}", h.DeleteHandler())
	})

	return r, issuer
}

func authHeader(t *testing.T, issuer *coreauth.JWTIssuer, userID int64) string {
	t.Helper()
	token, err := issuer.Issue(userID, false)
	require.NoError(t, err)
	return "Bearer " + token
}

func TestHTTPSaveAndGet(t *testing.T) {
	r, issuer := setupHTTPTest(t)

	body := `{"url":"https://example.com/http-test"}`
	req := httptest.NewRequest(http.MethodPost, "/library", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader(t, issuer, 1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)

	var item library.Item
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&item))
	require.Equal(t, "https://example.com/http-test", item.URL)
	require.Equal(t, "unread", item.State)

	// GET the created item
	req2 := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/library/%d", item.ID), nil)
	req2.Header.Set("Authorization", authHeader(t, issuer, 1))
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)

	require.Equal(t, http.StatusOK, rec2.Code)
}

func TestHTTPList(t *testing.T) {
	r, issuer := setupHTTPTest(t)

	// Save 2 items
	for _, url := range []string{"https://a.com", "https://b.com"} {
		body, _ := json.Marshal(library.SaveRequest{URL: url})
		req := httptest.NewRequest(http.MethodPost, "/library", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authHeader(t, issuer, 1))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusCreated, rec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/library?limit=10", nil)
	req.Header.Set("Authorization", authHeader(t, issuer, 1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var result library.ListResult
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&result))
	require.Equal(t, 2, result.Total)
	require.Len(t, result.Items, 2)
}

func TestHTTPUpdate(t *testing.T) {
	r, issuer := setupHTTPTest(t)

	// Save
	body := `{"url":"https://example.com/update-http"}`
	req := httptest.NewRequest(http.MethodPost, "/library", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader(t, issuer, 1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var item library.Item
	json.NewDecoder(rec.Body).Decode(&item)

	// Update
	updateBody := `{"state":"read","is_favorite":true}`
	req2 := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/library/%d", item.ID), bytes.NewBufferString(updateBody))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", authHeader(t, issuer, 1))
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)

	require.Equal(t, http.StatusOK, rec2.Code)

	var updated library.Item
	json.NewDecoder(rec2.Body).Decode(&updated)
	require.Equal(t, "read", updated.State)
	require.True(t, updated.IsFavorite)
}

func TestHTTPDelete(t *testing.T) {
	r, issuer := setupHTTPTest(t)

	body := `{"url":"https://example.com/delete-http"}`
	req := httptest.NewRequest(http.MethodPost, "/library", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader(t, issuer, 1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var item library.Item
	json.NewDecoder(rec.Body).Decode(&item)

	// Delete
	req2 := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/library/%d", item.ID), nil)
	req2.Header.Set("Authorization", authHeader(t, issuer, 1))
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)

	require.Equal(t, http.StatusNoContent, rec2.Code)
}

func TestHTTPSaveNoAuth(t *testing.T) {
	r, _ := setupHTTPTest(t)

	body := `{"url":"https://example.com/no-auth"}`
	req := httptest.NewRequest(http.MethodPost, "/library", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHTTPSaveDuplicate(t *testing.T) {
	r, issuer := setupHTTPTest(t)

	body := `{"url":"https://example.com/dup-http"}`
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/library", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authHeader(t, issuer, 1))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if i == 0 {
			require.Equal(t, http.StatusCreated, rec.Code)
		} else {
			require.Equal(t, http.StatusConflict, rec.Code)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/library/... -v -count=1 -run TestHTTP
```

Expected: FAIL — `library.NewHTTP`, handler methods not defined.

- [ ] **Step 3: Write the HTTP handlers**

Create `internal/library/http.go`:
```go
package library

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	coreauth "github.com/ismd/linktheca/internal/core/auth"
	"github.com/ismd/linktheca/internal/core/httpx"
)

type HTTP struct {
	svc *Service
}

func NewHTTP(svc *Service) *HTTP {
	return &HTTP{svc: svc}
}

// SaveHandler returns the http.HandlerFunc for POST /library.
func (h *HTTP) SaveHandler() http.HandlerFunc { return h.save }

// ListHandler returns the http.HandlerFunc for GET /library.
func (h *HTTP) ListHandler() http.HandlerFunc { return h.list }

// GetHandler returns the http.HandlerFunc for GET /library/:id.
func (h *HTTP) GetHandler() http.HandlerFunc { return h.get }

// UpdateHandler returns the http.HandlerFunc for PATCH /library/:id.
func (h *HTTP) UpdateHandler() http.HandlerFunc { return h.update }

// DeleteHandler returns the http.HandlerFunc for DELETE /library/:id.
func (h *HTTP) DeleteHandler() http.HandlerFunc { return h.delete }

func (h *HTTP) save(w http.ResponseWriter, r *http.Request) {
	var req SaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}
	if req.URL == "" {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "url is required")
		return
	}

	userID := coreauth.UserID(r.Context())
	item, err := h.svc.SaveURL(r.Context(), userID, req.URL)
	if err != nil {
		writeLibraryError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, item)
}

func (h *HTTP) list(w http.ResponseWriter, r *http.Request) {
	userID := coreauth.UserID(r.Context())

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}

	params := ListParams{
		UserID: userID,
		State:  r.URL.Query().Get("state"),
		Limit:  limit,
		Offset: offset,
	}

	if fav := r.URL.Query().Get("favorite"); fav != "" {
		v := fav == "true"
		params.Favorite = &v
	}

	result, err := h.svc.List(r.Context(), params)
	if err != nil {
		writeLibraryError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *HTTP) get(w http.ResponseWriter, r *http.Request) {
	userID := coreauth.UserID(r.Context())
	itemID, err := parseID(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}

	item, err := h.svc.GetByID(r.Context(), userID, itemID)
	if err != nil {
		writeLibraryError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, item)
}

func (h *HTTP) update(w http.ResponseWriter, r *http.Request) {
	userID := coreauth.UserID(r.Context())
	itemID, err := parseID(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}

	var req UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}

	item, err := h.svc.Update(r.Context(), userID, itemID, req)
	if err != nil {
		writeLibraryError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, item)
}

func (h *HTTP) delete(w http.ResponseWriter, r *http.Request) {
	userID := coreauth.UserID(r.Context())
	itemID, err := parseID(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}

	if err := h.svc.Delete(r.Context(), userID, itemID); err != nil {
		writeLibraryError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func parseID(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

func writeLibraryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "item not found")
	case errors.Is(err, ErrAlreadySaved):
		httpx.WriteError(w, http.StatusConflict, "already_saved", "article already in library")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "")
	}
}
```

- [ ] **Step 4: Run HTTP tests to verify pass**

```bash
go test ./internal/library/... -v -count=1 -run TestHTTP
```

Expected: PASS for all HTTP tests.

- [ ] **Step 5: Run ALL library tests together**

```bash
go test ./internal/library/... -v -count=1 -short
```

Expected: PASS for all unit and HTTP tests. Integration tests skipped due to `-short`.

- [ ] **Step 6: Commit**

```bash
git add internal/library/http.go internal/library/http_test.go
git commit -m "feat(library): HTTP handlers for save, list, get, update, delete"
```

---

## Part G: Server wiring

### Task 10: Wire library routes into server.go

**Files:**
- Modify: `internal/server/server.go`

- [ ] **Step 1: Update server.go to include library routes**

Add the library import and wiring. The full updated `server.go`:

```go
package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chicors "github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"github.com/ismd/linktheca/internal/auth"
	coreauth "github.com/ismd/linktheca/internal/core/auth"
	"github.com/ismd/linktheca/internal/core/config"
	"github.com/ismd/linktheca/internal/core/content"
	"github.com/ismd/linktheca/internal/core/httpx"
	"github.com/ismd/linktheca/internal/library"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Deps struct {
	Config *config.Config
	Logger *slog.Logger
	DB     *pgxpool.Pool
}

func New(deps Deps) *http.Server {
	logger := deps.Logger
	cfg := deps.Config

	issuer := coreauth.NewJWTIssuer(cfg.JWTSecret, cfg.JWTAccessTTL)

	// Auth module
	authStore := auth.NewStore(deps.DB)
	authSvc := auth.NewService(authStore, issuer, auth.ServiceConfig{
		RefreshTTL:          cfg.JWTRefreshTTL,
		RegistrationEnabled: cfg.RegistrationEnabled,
	})
	authHTTP := auth.NewHTTP(authSvc, issuer)

	// Library module
	libStore := library.NewStore(deps.DB)
	extractor := content.NewExtractor()
	libSvc := library.NewService(libStore, extractor)
	libHTTP := library.NewHTTP(libSvc)

	r := chi.NewRouter()

	r.Use(httpx.RequestID)
	r.Use(httpx.RequestLogger(logger))
	r.Use(httpx.Recover(logger))

	if len(cfg.CORSOrigins) > 0 {
		r.Use(chicors.Handler(chicors.Options{
			AllowedOrigins:   cfg.CORSOrigins,
			AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Authorization", "Content-Type"},
			AllowCredentials: false,
			MaxAge:           300,
		}))
	}

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	// Auth — public (rate-limited)
	r.Group(func(r chi.Router) {
		r.Use(httprate.LimitByIP(10, 10*time.Minute))
		r.Post("/auth/register", authHTTP.RegisterHandler())
		r.Post("/auth/login", authHTTP.LoginHandler())
		r.Post("/auth/refresh", authHTTP.RefreshHandler())
	})

	// Auth — protected
	r.Group(func(r chi.Router) {
		r.Use(coreauth.RequireUser(issuer))
		r.Post("/auth/logout", authHTTP.LogoutHandler())
		r.Get("/auth/me", authHTTP.MeHandler())
	})

	// Library — all routes require auth
	r.Route("/library", func(r chi.Router) {
		r.Use(coreauth.RequireUser(issuer))
		r.Post("/", libHTTP.SaveHandler())
		r.Get("/", libHTTP.ListHandler())
		r.Get("/{id}", libHTTP.GetHandler())
		r.Patch("/{id}", libHTTP.UpdateHandler())
		r.Delete("/{id}", libHTTP.DeleteHandler())
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return srv
}
```

- [ ] **Step 2: Verify everything compiles**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 3: Run all tests (unit)**

```bash
go test ./... -short -count=1
```

Expected: PASS for all packages.

- [ ] **Step 4: Commit**

```bash
git add internal/server/server.go
git commit -m "feat(server): wire library module into HTTP router"
```

---

## Part H: Full integration test

### Task 11: End-to-end library flow against real Postgres

**Files:**
- Create: `internal/library/integration_test.go`

- [ ] **Step 1: Write the end-to-end integration test**

Create `internal/library/integration_test.go`:
```go
package library_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	coreauth "github.com/ismd/linktheca/internal/core/auth"
	"github.com/ismd/linktheca/internal/core/content"
	"github.com/ismd/linktheca/internal/library"
	"github.com/ismd/linktheca/internal/testing/testdb"
	"github.com/stretchr/testify/require"
)

func TestIntegrationFullLibraryFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)

	// Set up a fake HTTP server serving article content
	articleSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		html := `<!DOCTYPE html><html><head><title>Integration Test Article</title></head>
		<body><article><h1>Integration Test Article</h1>
		<p>This is a long enough paragraph to be recognized by the readability algorithm.
		It contains multiple sentences and enough words to make the extraction work properly.
		The content extraction should identify this as the main article text.</p>
		<p>Second paragraph with more content to ensure reliable extraction.
		We need enough text for the readability heuristics to work correctly.</p>
		</article></body></html>`
		_, _ = w.Write([]byte(html))
	}))
	defer articleSrv.Close()

	// Create test user
	userID := createTestUser(t, pool)

	// Build the library stack with real store and real extractor
	store := library.NewStore(pool)
	extractor := content.NewExtractor()
	svc := library.NewService(store, extractor)

	issuer := coreauth.NewJWTIssuer("test-secret-at-least-32-bytes-long-for-hmac", 15*time.Minute)
	h := library.NewHTTP(svc)

	r := chi.NewRouter()
	r.Route("/library", func(r chi.Router) {
		r.Use(coreauth.RequireUser(issuer))
		r.Post("/", h.SaveHandler())
		r.Get("/", h.ListHandler())
		r.Get("/{id}", h.GetHandler())
		r.Patch("/{id}", h.UpdateHandler())
		r.Delete("/{id}", h.DeleteHandler())
	})

	token, err := issuer.Issue(userID, false)
	require.NoError(t, err)
	auth := "Bearer " + token

	// 1. Save a URL
	body, _ := json.Marshal(library.SaveRequest{URL: articleSrv.URL})
	req := httptest.NewRequest(http.MethodPost, "/library", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, "save response: %s", rec.Body.String())

	var saved library.Item
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&saved))
	require.Equal(t, "unread", saved.State)
	require.NotZero(t, saved.ID)

	// 2. List items
	req = httptest.NewRequest(http.MethodGet, "/library?limit=10", nil)
	req.Header.Set("Authorization", auth)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var listResult library.ListResult
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&listResult))
	require.Equal(t, 1, listResult.Total)

	// 3. Mark as read
	updateBody := `{"state":"read"}`
	req = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/library/%d", saved.ID), bytes.NewBufferString(updateBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var updated library.Item
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&updated))
	require.Equal(t, "read", updated.State)

	// 4. Filter by state=unread — should return 0
	req = httptest.NewRequest(http.MethodGet, "/library?state=unread&limit=10", nil)
	req.Header.Set("Authorization", auth)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var filtered library.ListResult
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&filtered))
	require.Equal(t, 0, filtered.Total)

	// 5. Delete
	req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/library/%d", saved.ID), nil)
	req.Header.Set("Authorization", auth)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)

	// 6. Verify deleted
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/library/%d", saved.ID), nil)
	req.Header.Set("Authorization", auth)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestIntegrationSaveDuplicateURL(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)

	articleSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>Dup</title></head><body><article><p>Content for duplicate test article with enough text.</p><p>More text here.</p></article></body></html>`))
	}))
	defer articleSrv.Close()

	userID := createTestUser(t, pool)
	store := library.NewStore(pool)
	extractor := content.NewExtractor()
	svc := library.NewService(store, extractor)
	issuer := coreauth.NewJWTIssuer("test-secret-at-least-32-bytes-long-for-hmac", 15*time.Minute)
	h := library.NewHTTP(svc)

	r := chi.NewRouter()
	r.Route("/library", func(r chi.Router) {
		r.Use(coreauth.RequireUser(issuer))
		r.Post("/", h.SaveHandler())
	})

	token, _ := issuer.Issue(userID, false)
	auth := "Bearer " + token

	body, _ := json.Marshal(library.SaveRequest{URL: articleSrv.URL})

	// First save
	req := httptest.NewRequest(http.MethodPost, "/library", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	// Second save — should return 409 Conflict
	req = httptest.NewRequest(http.MethodPost, "/library", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusConflict, rec.Code)
}
```

- [ ] **Step 2: Run the integration tests**

```bash
go test ./internal/library/... -v -count=1 -run TestIntegration
```

Expected: PASS for all integration tests (testcontainers spins up a real Postgres).

- [ ] **Step 3: Run the entire test suite**

```bash
go test ./... -count=1 -race
```

Expected: PASS for all packages — auth, library, core, server.

- [ ] **Step 4: Commit**

```bash
git add internal/library/integration_test.go
git commit -m "test(library): end-to-end integration test for full library CRUD flow"
```

---

## Part I: GetItemWithContent for reader view

### Task 12: Store and service method for full article content (TDD)

The `GetByID` from Task 6 joins only summary fields (URL, title, excerpt, reading_time). For the article reader view, we need the full `article_contents` row (text, HTML, byline, lang).

**Files:**
- Modify: `internal/library/store.go`
- Modify: `internal/library/store_test.go`
- Modify: `internal/library/types.go`
- Modify: `internal/library/service.go`
- Modify: `internal/library/http.go`

- [ ] **Step 1: Add ItemDetail type to types.go**

Append to `internal/library/types.go`:
```go
// ItemDetail is a library item with the full article content for reader view.
type ItemDetail struct {
	Item
	Content ArticleContent `json:"content"`
}
```

- [ ] **Step 2: Write the failing integration test**

Append to `internal/library/store_test.go`:
```go
func TestIntegrationGetItemDetail(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := library.NewStore(pool)
	ctx := context.Background()

	userID := createTestUser(t, pool)

	content, _ := store.UpsertContent(ctx, library.UpsertContentParams{
		URL:   "https://example.com/detail",
		Title: ptr("Detail Article"),
		Text:  ptr("Full article text for reader view."),
		HTML:  ptr("<p>Full article text for reader view.</p>"),
	})
	item, _ := store.CreateItem(ctx, userID, content.ID)

	detail, err := store.GetItemDetail(ctx, userID, item.ID)
	require.NoError(t, err)
	require.Equal(t, item.ID, detail.ID)
	require.Equal(t, "https://example.com/detail", detail.Content.URL)
	require.NotNil(t, detail.Content.Text)
	require.Equal(t, "Full article text for reader view.", *detail.Content.Text)
	require.NotNil(t, detail.Content.HTML)
}
```

- [ ] **Step 3: Run test to verify it fails**

```bash
go test ./internal/library/... -v -count=1 -run TestIntegrationGetItemDetail
```

Expected: FAIL — `GetItemDetail` not defined.

- [ ] **Step 4: Implement GetItemDetail in store**

Append to `internal/library/store.go`:
```go
// GetItemDetail returns a library item with the full article_contents record.
func (s *Store) GetItemDetail(ctx context.Context, userID, itemID int64) (*ItemDetail, error) {
	row := s.db.QueryRow(ctx, `
		SELECT li.id, li.user_id, li.content_id, li.state, li.is_favorite, li.note, li.saved_at, li.read_at,
		       ac.url, ac.title, ac.excerpt, ac.reading_time_seconds,
		       ac.id, ac.url, ac.canonical_url, ac.title, ac.byline, ac.excerpt, ac.text, ac.html,
		       ac.lang, ac.reading_time_seconds, ac.fetched_at, ac.fetch_error
		FROM library_items li
		JOIN article_contents ac ON ac.id = li.content_id
		WHERE li.id = $1 AND li.user_id = $2
	`, itemID, userID)

	var d ItemDetail
	err := row.Scan(
		&d.ID, &d.UserID, &d.ContentID, &d.State, &d.IsFavorite, &d.Note, &d.SavedAt, &d.ReadAt,
		&d.URL, &d.Title, &d.Excerpt, &d.ReadTimeSecs,
		&d.Content.ID, &d.Content.URL, &d.Content.CanonicalURL, &d.Content.Title,
		&d.Content.Byline, &d.Content.Excerpt, &d.Content.Text, &d.Content.HTML,
		&d.Content.Lang, &d.Content.ReadingTimeSecs, &d.Content.FetchedAt, &d.Content.FetchError,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get item detail: %w", err)
	}
	return &d, nil
}
```

- [ ] **Step 5: Add GetItemDetail to StoreAPI interface and service**

Add to the `StoreAPI` interface in `service.go`:
```go
GetItemDetail(ctx context.Context, userID, itemID int64) (*ItemDetail, error)
```

Add method to service:
```go
// GetDetail returns a single library item with full article content for reader view.
func (s *Service) GetDetail(ctx context.Context, userID, itemID int64) (*ItemDetail, error) {
	return s.store.GetItemDetail(ctx, userID, itemID)
}
```

Add a `GET /library/{id}/content` endpoint in `http.go`:

Add to `HTTP` struct methods:
```go
// GetDetailHandler returns the http.HandlerFunc for GET /library/:id/content.
func (h *HTTP) GetDetailHandler() http.HandlerFunc { return h.getDetail }

func (h *HTTP) getDetail(w http.ResponseWriter, r *http.Request) {
	userID := coreauth.UserID(r.Context())
	itemID, err := parseID(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}

	detail, err := h.svc.GetDetail(r.Context(), userID, itemID)
	if err != nil {
		writeLibraryError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, detail)
}
```

- [ ] **Step 6: Update mock store in service_test.go**

Add to `mockStore`:
```go
func (m *mockStore) GetItemDetail(_ context.Context, userID, itemID int64) (*library.ItemDetail, error) {
	item, ok := m.items[itemID]
	if !ok || item.UserID != userID {
		return nil, library.ErrNotFound
	}
	return &library.ItemDetail{Item: *item}, nil
}
```

- [ ] **Step 7: Run all tests**

```bash
go test ./internal/library/... -v -count=1
```

Expected: PASS for all tests.

- [ ] **Step 8: Commit**

```bash
git add internal/library/
git commit -m "feat(library): add GetDetail for full article content in reader view"
```

---

### Task 13: Wire the /content route into server.go

**Files:**
- Modify: `internal/server/server.go`

- [ ] **Step 1: Add the content detail route**

In `server.go`, inside the `/library` route group, add:
```go
r.Get("/{id}/content", libHTTP.GetDetailHandler())
```

So the library route block becomes:
```go
r.Route("/library", func(r chi.Router) {
	r.Use(coreauth.RequireUser(issuer))
	r.Post("/", libHTTP.SaveHandler())
	r.Get("/", libHTTP.ListHandler())
	r.Get("/{id}", libHTTP.GetHandler())
	r.Get("/{id}/content", libHTTP.GetDetailHandler())
	r.Patch("/{id}", libHTTP.UpdateHandler())
	r.Delete("/{id}", libHTTP.DeleteHandler())
})
```

- [ ] **Step 2: Verify build and tests**

```bash
go build ./...
go test ./... -short -count=1
```

Expected: builds clean, all unit tests pass.

- [ ] **Step 3: Commit**

```bash
git add internal/server/server.go
git commit -m "feat(server): add library content detail route"
```

---

## Part J: Manual smoke test

### Task 14: Manual smoke test against running backend

**Files:** (none — manual verification)

- [ ] **Step 1: Start Postgres**

```bash
make dev-db
sleep 3
```

- [ ] **Step 2: Start backend**

```bash
LINKTHECA_DB_DSN="postgres://linktheca:linktheca@localhost:5432/linktheca?sslmode=disable" \
LINKTHECA_JWT_SECRET="dev-only-secret-that-is-at-least-32-bytes-long" \
go run ./cmd/linktheca &
sleep 2
```

- [ ] **Step 3: Register and get token (or login if user already exists)**

```bash
curl -s -X POST http://localhost:8080/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@linktheca.local","password":"initial-admin-password","display_name":"Admin"}' | tee /tmp/register.json

# If registration fails because user exists:
# curl -s -X POST http://localhost:8080/auth/login \
#   -H 'Content-Type: application/json' \
#   -d '{"email":"admin@linktheca.local","password":"initial-admin-password"}' | tee /tmp/register.json

ACCESS=$(jq -r '.tokens.access_token' /tmp/register.json)
echo "Token: $ACCESS"
```

- [ ] **Step 4: Save a URL to the library**

```bash
curl -s -X POST http://localhost:8080/library \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $ACCESS" \
  -d '{"url":"https://go.dev/blog/go1.22"}' | tee /tmp/saved.json | jq .
```

Expected: 201 with JSON containing `id`, `state: "unread"`, `url`, and extracted `title`.

- [ ] **Step 5: List library items**

```bash
curl -s http://localhost:8080/library \
  -H "Authorization: Bearer $ACCESS" | jq .
```

Expected: JSON with `total: 1` and the saved item in `items`.

- [ ] **Step 6: Get item detail (full content)**

```bash
ITEM_ID=$(jq -r '.id' /tmp/saved.json)
curl -s http://localhost:8080/library/${ITEM_ID}/content \
  -H "Authorization: Bearer $ACCESS" | jq .
```

Expected: JSON with `content` object containing `text` and `html` fields.

- [ ] **Step 7: Mark as read**

```bash
curl -s -X PATCH http://localhost:8080/library/${ITEM_ID} \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $ACCESS" \
  -d '{"state":"read"}' | jq .
```

Expected: JSON with `state: "read"` and `read_at` set.

- [ ] **Step 8: Delete**

```bash
curl -s -X DELETE http://localhost:8080/library/${ITEM_ID} \
  -H "Authorization: Bearer $ACCESS" -w "\n%{http_code}\n"
```

Expected: 204 with no body.

- [ ] **Step 9: Stop backend and DB**

```bash
kill %1
wait 2>/dev/null
make dev-db-down
```

- [ ] **Step 10: No commit needed — manual verification only**

If any step failed, fix the issue, commit the fix, and re-run.

---

## Phase 2 complete

At this point the Linktheca backend has everything from Phase 1, plus:

- **`article_contents`** table — shared content cache with full-text search index.
- **`library_items`** table — per-user saved articles with state (`unread`/`read`/`archived`), favorites, notes.
- **Content extractor** — `go-readability`-based article extraction with reading time estimation.
- **Library module** (`store → service → http`) — full CRUD: save URL, list (with filtering/pagination), get, get detail (reader view), update (state/favorite/note), delete.
- **Multi-user isolation** — all queries scoped by `user_id`, tested with different users.
- **Integration tests** — end-to-end flow against real Postgres via testcontainers.

**Next phase:** `Phase 3 — Radar backend` (RSS feeds, Ollama embeddings, River jobs, semantic matching). Will be planned in its own document.

## Handoff notes for the executing agent

- Content extraction via `go-readability` makes a real HTTP request. In integration tests, use `httptest.NewServer` to serve fake HTML.
- The `canonicalOrEmpty` function in `extractor.go` is intentionally simple (returns empty string). Canonical URL detection from HTML `<link rel="canonical">` can be added later.
- The `UpsertContent` uses `ON CONFLICT (url) DO UPDATE SET url = EXCLUDED.url` — this is a no-op update that lets us get the RETURNING clause for existing rows. This is intentional: we don't overwrite parsed content.
- `UpdateItem` builds a dynamic SQL query with positional parameters. When testing manually, check that partial updates (e.g., only `is_favorite`) don't reset other fields.
- The `fmt` import is needed in `store_test.go` (for `TestIntegrationListItems`) and `http_test.go` (for `fmt.Sprintf` in URL construction). Make sure it's in the imports.
- If `go-readability` extraction produces empty title/text for your test HTML, increase the paragraph length — the algorithm needs enough content to distinguish it from boilerplate.
