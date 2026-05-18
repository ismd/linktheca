# Radar Read-API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the existing `internal/radar` module with the read-side HTTP API needed by the upcoming Radar UI: topic CRUD with aggregated stats, denormalized match list, status, admin-only feed list. No new migrations.

**Architecture:** Pure extension of the `store → service → http` pattern already used by `radar` and `library`. New SQL queries against existing tables. Embedding behavior on `PATCH /radar/topics/{id}` mirrors `CreateTopic` (non-atomic; embedder failure leaves fields updated and embedding stale — caller can retry).

**Tech Stack:** Go 1.22+, `chi/v5`, `pgx/v5`, `pgvector/pgvector-go`, `testify`, `testcontainers-go` (via `internal/testing/testdb`).

**Spec:** `docs/superpowers/specs/2026-05-14-radar-read-api-design.md`

---

## API contract (locked here so engineer doesn't need to re-derive)

All routes mount under `/radar`. Snake_case JSON. Existing `DisabledHandler` wildcard covers them when `LINKTHECA_RADAR_ENABLED=false`.

| Method | Path | Auth |
|---|---|---|
| `GET` | `/radar/topics` | user |
| `GET` | `/radar/topics/{id}` | user |
| `PATCH` | `/radar/topics/{id}` | user |
| `DELETE` | `/radar/topics/{id}` | user |
| `GET` | `/radar/matches` | user |
| `PATCH` | `/radar/matches/{id}` | user |
| `GET` | `/radar/status` | user |
| `GET` | `/radar/feeds` | **admin** |

Response shapes:

**`TopicWithStats`** — embedded `Topic` + `stats: {new_count, total_count, source_count, last_match_at}`.
**`MatchView`** — `{id, topic_id, topic_name, similarity, state, matched_at, finding: {id, feed_id, feed_title, url, title, summary, published_at, discovered_at}}`.
**`MatchList`/`FeedList`** — `{items, total}`.
**`RadarStatus`** — `{last_sweep_at}` (nullable).
**`UpdateTopicRequest`** — `{name?, description?, match_threshold?, is_active?}`.
**`UpdateMatchRequest`** — `{state: "new"|"seen"}`.

Errors: existing `writeRadarError` is sufficient. `400 bad_request`, `404 not_found`, `503 embedder_unavailable`, `500 internal`. Topic/match belonging to another user → `404` (don't leak existence).

---

## Files touched

| Path | Change |
|---|---|
| `internal/radar/types.go` | extend — new DTOs, params, response shapes |
| `internal/radar/store.go` | extend — 8 new methods |
| `internal/radar/service.go` | extend — update `StoreAPI`, add 8 new methods |
| `internal/radar/http.go` | extend — 8 new handler getters + private handlers |
| `internal/radar/store_test.go` | extend — seed helpers + new test cases |
| `internal/radar/service_test.go` | extend — mockStore methods + new test cases |
| `internal/radar/http_test.go` | extend — new handler tests |
| `internal/radar/integration_test.go` | extend — new end-to-end scenario |
| `internal/server/server.go` | extend — wire 8 new routes inside existing `r.Route("/radar", …)` |

No migrations. No new packages.

---

## Task 1: Add new types to `types.go`

**Files:**
- Modify: `internal/radar/types.go`

Types are inert — no tests for them directly; they're exercised by every subsequent task. Single commit at the end.

- [x] **Step 1: Append new types to `internal/radar/types.go`**

Add the following at the end of the file (after the existing `FindingUpsert` type):

```go
// --- Read-API types -------------------------------------------------------

// TopicStats holds aggregated match counts for a topic.
type TopicStats struct {
	NewCount    int        `json:"new_count"`
	TotalCount  int        `json:"total_count"`
	SourceCount int        `json:"source_count"`
	LastMatchAt *time.Time `json:"last_match_at"`
}

// TopicWithStats is a Topic enriched with aggregate match stats.
// Returned by GET /radar/topics and GET /radar/topics/{id}.
type TopicWithStats struct {
	Topic
	Stats TopicStats `json:"stats"`
}

// MatchFinding is the finding portion of a denormalized MatchView.
type MatchFinding struct {
	ID           int64      `json:"id"`
	FeedID       int64      `json:"feed_id"`
	FeedTitle    *string    `json:"feed_title"`
	URL          string     `json:"url"`
	Title        *string    `json:"title"`
	Summary      *string    `json:"summary"`
	PublishedAt  *time.Time `json:"published_at"`
	DiscoveredAt time.Time  `json:"discovered_at"`
}

// MatchView is a Match denormalized with topic name and finding metadata.
// Returned by GET /radar/matches.
type MatchView struct {
	ID         int64        `json:"id"`
	TopicID    int64        `json:"topic_id"`
	TopicName  string       `json:"topic_name"`
	Similarity float32      `json:"similarity"`
	State      string       `json:"state"`
	MatchedAt  time.Time    `json:"matched_at"`
	Finding    MatchFinding `json:"finding"`
}

// ListMatchesParams holds query parameters for GET /radar/matches.
type ListMatchesParams struct {
	UserID  int64
	TopicID *int64  // nil = any topic owned by UserID
	State   *string // nil = any state
	Limit   int
	Offset  int
}

// MatchList holds the paginated response for GET /radar/matches.
type MatchList struct {
	Items []MatchView `json:"items"`
	Total int         `json:"total"`
}

// ListFeedsParams holds query parameters for GET /radar/feeds (admin).
type ListFeedsParams struct {
	Limit  int
	Offset int
}

// FeedList holds the paginated response for GET /radar/feeds.
type FeedList struct {
	Items []Feed `json:"items"`
	Total int    `json:"total"`
}

// UpdateTopicRequest is the payload for PATCH /radar/topics/{id}.
// All fields are optional; only non-nil fields are updated.
type UpdateTopicRequest struct {
	Name           *string  `json:"name,omitempty"`
	Description    *string  `json:"description,omitempty"`
	MatchThreshold *float32 `json:"match_threshold,omitempty"`
	IsActive       *bool    `json:"is_active,omitempty"`
}

// UpdateTopicParams is the store-level analogue of UpdateTopicRequest.
type UpdateTopicParams struct {
	Name           *string
	Description    *string
	MatchThreshold *float32
	IsActive       *bool
}

// UpdateMatchRequest is the payload for PATCH /radar/matches/{id}.
type UpdateMatchRequest struct {
	State string `json:"state"`
}

// RadarStatus is the response for GET /radar/status.
type RadarStatus struct {
	LastSweepAt *time.Time `json:"last_sweep_at"`
}
```

- [x] **Step 2: Verify compilation**

Run: `go build ./internal/radar/...`
Expected: PASS (types compile in isolation).

- [x] **Step 3: Commit**

```bash
git add internal/radar/types.go
git commit -m "feat(radar): add read-API types and DTOs"
```

---

## Task 2: Store — `ListTopicsWithStats` and `GetTopicWithStats`

**Files:**
- Modify: `internal/radar/store.go`
- Modify: `internal/radar/store_test.go`

Adds the topic-list query with one-shot aggregation via `LEFT JOIN LATERAL`. Single-topic getter reuses the same SQL with an extra `AND t.id = $2`.

- [x] **Step 1: Add seed helpers to `store_test.go`**

Add the following helpers right after the existing `seedUser`:

```go
func seedTopic(t *testing.T, pool *pgxpool.Pool, userID int64, name, desc string, threshold float32, active bool) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO radar_topics (user_id, name, description, match_threshold, is_active)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		userID, name, desc, threshold, active).Scan(&id)
	require.NoError(t, err)
	return id
}

func seedFeed(t *testing.T, pool *pgxpool.Pool, url, title string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO radar_feeds (url, kind, title) VALUES ($1, 'rss', $2) RETURNING id`,
		url, title).Scan(&id)
	require.NoError(t, err)
	return id
}

func seedFinding(t *testing.T, pool *pgxpool.Pool, feedID int64, url, title string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO radar_findings (feed_id, url, title) VALUES ($1, $2, $3) RETURNING id`,
		feedID, url, title).Scan(&id)
	require.NoError(t, err)
	return id
}

func seedMatch(t *testing.T, pool *pgxpool.Pool, topicID, findingID int64, state string, similarity float32) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO radar_topic_matches (topic_id, finding_id, similarity, state)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		topicID, findingID, similarity, state).Scan(&id)
	require.NoError(t, err)
	return id
}

func seedSubscription(t *testing.T, pool *pgxpool.Pool, userID, feedID int64) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO radar_feed_subscriptions (user_id, feed_id) VALUES ($1, $2)`,
		userID, feedID)
	require.NoError(t, err)
}
```

- [x] **Step 2: Write failing test `TestStore_ListTopicsWithStats_empty`**

Append to `internal/radar/store_test.go`:

```go
func TestStore_ListTopicsWithStats_empty(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)

	items, err := store.ListTopicsWithStats(ctx, userID)
	require.NoError(t, err)
	require.Empty(t, items)
}
```

- [x] **Step 3: Write failing test `TestStore_ListTopicsWithStats_aggregates`**

Append:

```go
func TestStore_ListTopicsWithStats_aggregates(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)
	topicA := seedTopic(t, pool, userID, "A", "desc-a", 0.55, true)
	topicB := seedTopic(t, pool, userID, "B", "desc-b", 0.6, true)

	feed1 := seedFeed(t, pool, "https://f1.example/rss", "Feed1")
	feed2 := seedFeed(t, pool, "https://f2.example/rss", "Feed2")
	f1a := seedFinding(t, pool, feed1, "https://x.example/1", "t1")
	f1b := seedFinding(t, pool, feed1, "https://x.example/2", "t2")
	f2a := seedFinding(t, pool, feed2, "https://x.example/3", "t3")

	// topicA: 2 new + 1 seen across 2 feeds
	seedMatch(t, pool, topicA, f1a, "new", 0.7)
	seedMatch(t, pool, topicA, f1b, "new", 0.71)
	seedMatch(t, pool, topicA, f2a, "seen", 0.65)
	// topicB: no matches

	items, err := store.ListTopicsWithStats(ctx, userID)
	require.NoError(t, err)
	require.Len(t, items, 2)

	byID := make(map[int64]radar.TopicWithStats, len(items))
	for _, it := range items {
		byID[it.ID] = it
	}

	a := byID[topicA]
	require.Equal(t, 2, a.Stats.NewCount)
	require.Equal(t, 3, a.Stats.TotalCount)
	require.Equal(t, 2, a.Stats.SourceCount)
	require.NotNil(t, a.Stats.LastMatchAt)

	b := byID[topicB]
	require.Equal(t, 0, b.Stats.NewCount)
	require.Equal(t, 0, b.Stats.TotalCount)
	require.Equal(t, 0, b.Stats.SourceCount)
	require.Nil(t, b.Stats.LastMatchAt)
}
```

- [x] **Step 4: Write failing test `TestStore_ListTopicsWithStats_isolation`**

Append:

```go
func TestStore_ListTopicsWithStats_isolation(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userA := seedUser(t, pool)
	userB := seedUser(t, pool)
	seedTopic(t, pool, userB, "OtherB", "other-desc", 0.55, true)

	items, err := store.ListTopicsWithStats(ctx, userA)
	require.NoError(t, err)
	require.Empty(t, items)
}
```

- [x] **Step 5: Write failing test `TestStore_GetTopicWithStats_notFound`**

Append:

```go
func TestStore_GetTopicWithStats_notFound(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userA := seedUser(t, pool)
	userB := seedUser(t, pool)
	otherTopic := seedTopic(t, pool, userB, "OtherB", "other-desc", 0.55, true)

	_, err := store.GetTopicWithStats(ctx, userA, otherTopic)
	require.ErrorIs(t, err, radar.ErrNotFound)
}
```

- [x] **Step 6: Run tests; verify failure**

Run: `go test ./internal/radar/ -run 'TestStore_(ListTopicsWithStats|GetTopicWithStats)' -v`
Expected: FAIL (methods undefined).

- [x] **Step 7: Implement `ListTopicsWithStats` and `GetTopicWithStats` in `store.go`**

Append to `internal/radar/store.go`:

```go
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
```

- [x] **Step 8: Run tests; verify pass**

Run: `go test ./internal/radar/ -run 'TestStore_(ListTopicsWithStats|GetTopicWithStats)' -v`
Expected: PASS for all four tests.

- [x] **Step 9: Commit**

```bash
git add internal/radar/store.go internal/radar/store_test.go
git commit -m "feat(radar): add ListTopicsWithStats and GetTopicWithStats store methods"
```

---

## Task 3: Store — `UpdateTopic` and `DeleteTopic`

**Files:**
- Modify: `internal/radar/store.go`
- Modify: `internal/radar/store_test.go`

`UpdateTopic` dynamically builds the SET clause from non-nil fields (pattern from `library.UpdateItem`). `DeleteTopic` is a one-shot DELETE with ownership.

- [x] **Step 1: Write failing test `TestStore_UpdateTopic_partial`**

Append to `store_test.go`:

```go
func TestStore_UpdateTopic_partial(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)
	topicID := seedTopic(t, pool, userID, "orig", "orig description", 0.55, true)

	newName := "renamed"
	updated, err := store.UpdateTopic(ctx, userID, topicID, radar.UpdateTopicParams{
		Name: &newName,
	})
	require.NoError(t, err)
	require.Equal(t, "renamed", updated.Name)
	require.Equal(t, "orig description", updated.Description) // unchanged
	require.Equal(t, float32(0.55), updated.MatchThreshold)
	require.True(t, updated.IsActive)
}
```

- [x] **Step 2: Write failing test `TestStore_UpdateTopic_allFields`**

```go
func TestStore_UpdateTopic_allFields(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)
	topicID := seedTopic(t, pool, userID, "orig", "orig description", 0.55, true)

	name := "new-name"
	desc := "new description"
	threshold := float32(0.7)
	active := false
	updated, err := store.UpdateTopic(ctx, userID, topicID, radar.UpdateTopicParams{
		Name: &name, Description: &desc, MatchThreshold: &threshold, IsActive: &active,
	})
	require.NoError(t, err)
	require.Equal(t, name, updated.Name)
	require.Equal(t, desc, updated.Description)
	require.Equal(t, threshold, updated.MatchThreshold)
	require.False(t, updated.IsActive)
}
```

- [x] **Step 3: Write failing test `TestStore_UpdateTopic_otherUser`**

```go
func TestStore_UpdateTopic_otherUser(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userA := seedUser(t, pool)
	userB := seedUser(t, pool)
	topicID := seedTopic(t, pool, userB, "B's topic", "B's description", 0.55, true)

	name := "stolen"
	_, err := store.UpdateTopic(ctx, userA, topicID, radar.UpdateTopicParams{Name: &name})
	require.ErrorIs(t, err, radar.ErrNotFound)
}
```

- [x] **Step 4: Write failing test `TestStore_DeleteTopic_cascades`**

```go
func TestStore_DeleteTopic_cascades(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)
	topicID := seedTopic(t, pool, userID, "doomed", "to be deleted", 0.55, true)
	feedID := seedFeed(t, pool, "https://x.example/rss", "X")
	findingID := seedFinding(t, pool, feedID, "https://x.example/a", "a")
	matchID := seedMatch(t, pool, topicID, findingID, "new", 0.7)

	require.NoError(t, store.DeleteTopic(ctx, userID, topicID))

	// match is gone via CASCADE
	var count int
	err := pool.QueryRow(ctx,
		`SELECT count(*) FROM radar_topic_matches WHERE id=$1`, matchID).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)
}
```

- [x] **Step 5: Write failing test `TestStore_DeleteTopic_otherUser`**

```go
func TestStore_DeleteTopic_otherUser(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userA := seedUser(t, pool)
	userB := seedUser(t, pool)
	topicID := seedTopic(t, pool, userB, "B's", "B's desc", 0.55, true)

	err := store.DeleteTopic(ctx, userA, topicID)
	require.ErrorIs(t, err, radar.ErrNotFound)
}
```

- [x] **Step 6: Run tests; verify failure**

Run: `go test ./internal/radar/ -run 'TestStore_(UpdateTopic|DeleteTopic)' -v`
Expected: FAIL — methods undefined.

- [x] **Step 7: Implement `UpdateTopic` and `DeleteTopic` in `store.go`**

Append to `internal/radar/store.go`. The `strings` package must be imported (it isn't currently — add to the existing import block):

```go
import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)
```

Then add the methods:

```go
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
```

- [x] **Step 8: Run tests; verify pass**

Run: `go test ./internal/radar/ -run 'TestStore_(UpdateTopic|DeleteTopic)' -v`
Expected: PASS for all five tests.

- [x] **Step 9: Commit**

```bash
git add internal/radar/store.go internal/radar/store_test.go
git commit -m "feat(radar): add UpdateTopic and DeleteTopic store methods"
```

---

## Task 4: Store — `ListMatches` and `UpdateMatchState`

**Files:**
- Modify: `internal/radar/store.go`
- Modify: `internal/radar/store_test.go`

`ListMatches` joins topics+findings+feeds; ownership through `t.user_id`. `UpdateMatchState` uses subselect for ownership.

- [x] **Step 1: Write failing test `TestStore_ListMatches_filters`**

Append to `store_test.go`:

```go
func TestStore_ListMatches_filters(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)
	topicA := seedTopic(t, pool, userID, "A", "desc-a", 0.55, true)
	topicB := seedTopic(t, pool, userID, "B", "desc-b", 0.55, true)
	feedID := seedFeed(t, pool, "https://x.example/rss", "X-feed")
	f1 := seedFinding(t, pool, feedID, "https://x.example/1", "title-1")
	f2 := seedFinding(t, pool, feedID, "https://x.example/2", "title-2")
	f3 := seedFinding(t, pool, feedID, "https://x.example/3", "title-3")
	seedMatch(t, pool, topicA, f1, "new", 0.7)
	seedMatch(t, pool, topicA, f2, "seen", 0.71)
	seedMatch(t, pool, topicB, f3, "new", 0.72)

	// No filters → all 3
	items, total, err := store.ListMatches(ctx, userID, radar.ListMatchesParams{Limit: 50})
	require.NoError(t, err)
	require.Len(t, items, 3)
	require.Equal(t, 3, total)

	// Filter by topic A → 2
	items, total, err = store.ListMatches(ctx, userID,
		radar.ListMatchesParams{TopicID: &topicA, Limit: 50})
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, 2, total)

	// Filter by state=new → 2 (topicA-f1 + topicB-f3)
	stateNew := "new"
	items, total, err = store.ListMatches(ctx, userID,
		radar.ListMatchesParams{State: &stateNew, Limit: 50})
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, 2, total)

	// Combined: topic A + state=seen → 1
	stateSeen := "seen"
	items, total, err = store.ListMatches(ctx, userID,
		radar.ListMatchesParams{TopicID: &topicA, State: &stateSeen, Limit: 50})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, 1, total)
}
```

- [x] **Step 2: Write failing test `TestStore_ListMatches_denormalization`**

```go
func TestStore_ListMatches_denormalization(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)
	topicID := seedTopic(t, pool, userID, "Local-first", "local-first software", 0.55, true)
	feedID := seedFeed(t, pool, "https://hn.example/rss", "Hacker News")
	findingID := seedFinding(t, pool, feedID, "https://hn.example/a", "article-title")
	seedMatch(t, pool, topicID, findingID, "new", 0.73)

	items, _, err := store.ListMatches(ctx, userID, radar.ListMatchesParams{Limit: 50})
	require.NoError(t, err)
	require.Len(t, items, 1)
	m := items[0]
	require.Equal(t, "Local-first", m.TopicName)
	require.Equal(t, findingID, m.Finding.ID)
	require.Equal(t, feedID, m.Finding.FeedID)
	require.NotNil(t, m.Finding.FeedTitle)
	require.Equal(t, "Hacker News", *m.Finding.FeedTitle)
	require.NotNil(t, m.Finding.Title)
	require.Equal(t, "article-title", *m.Finding.Title)
}
```

- [x] **Step 3: Write failing test `TestStore_ListMatches_isolation`**

```go
func TestStore_ListMatches_isolation(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userA := seedUser(t, pool)
	userB := seedUser(t, pool)
	topicB := seedTopic(t, pool, userB, "B's topic", "B's desc", 0.55, true)
	feedID := seedFeed(t, pool, "https://x.example/rss", "X")
	findingID := seedFinding(t, pool, feedID, "https://x.example/a", "a")
	seedMatch(t, pool, topicB, findingID, "new", 0.7)

	// userA sees nothing globally
	items, total, err := store.ListMatches(ctx, userA, radar.ListMatchesParams{Limit: 50})
	require.NoError(t, err)
	require.Empty(t, items)
	require.Equal(t, 0, total)

	// userA cannot peek into B's topic by passing topic_id
	items, total, err = store.ListMatches(ctx, userA,
		radar.ListMatchesParams{TopicID: &topicB, Limit: 50})
	require.NoError(t, err)
	require.Empty(t, items)
	require.Equal(t, 0, total)
}
```

- [x] **Step 4: Write failing test `TestStore_ListMatches_pagination`**

```go
func TestStore_ListMatches_pagination(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)
	topicID := seedTopic(t, pool, userID, "T", "desc-t", 0.55, true)
	feedID := seedFeed(t, pool, "https://x.example/rss", "X")
	for i := 0; i < 5; i++ {
		fid := seedFinding(t, pool, feedID,
			fmt.Sprintf("https://x.example/a%d", i),
			fmt.Sprintf("title-%d", i))
		seedMatch(t, pool, topicID, fid, "new", 0.7)
	}

	items, total, err := store.ListMatches(ctx, userID,
		radar.ListMatchesParams{Limit: 2, Offset: 0})
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, 5, total)

	items, _, err = store.ListMatches(ctx, userID,
		radar.ListMatchesParams{Limit: 2, Offset: 4})
	require.NoError(t, err)
	require.Len(t, items, 1)
}
```

If `"fmt"` is not yet in `store_test.go`'s import block, add it.

- [x] **Step 5: Write failing test `TestStore_UpdateMatchState_ownership`**

```go
func TestStore_UpdateMatchState_ownership(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userA := seedUser(t, pool)
	userB := seedUser(t, pool)
	topicB := seedTopic(t, pool, userB, "B", "B's desc", 0.55, true)
	feedID := seedFeed(t, pool, "https://x.example/rss", "X")
	findingID := seedFinding(t, pool, feedID, "https://x.example/a", "a")
	matchID := seedMatch(t, pool, topicB, findingID, "new", 0.7)

	err := store.UpdateMatchState(ctx, userA, matchID, "seen")
	require.ErrorIs(t, err, radar.ErrNotFound)

	// B can update
	err = store.UpdateMatchState(ctx, userB, matchID, "seen")
	require.NoError(t, err)
}
```

- [x] **Step 6: Write failing test `TestStore_UpdateMatchState_idempotent`**

```go
func TestStore_UpdateMatchState_idempotent(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)
	topicID := seedTopic(t, pool, userID, "T", "desc", 0.55, true)
	feedID := seedFeed(t, pool, "https://x.example/rss", "X")
	findingID := seedFinding(t, pool, feedID, "https://x.example/a", "a")
	matchID := seedMatch(t, pool, topicID, findingID, "seen", 0.7)

	require.NoError(t, store.UpdateMatchState(ctx, userID, matchID, "seen"))
	require.NoError(t, store.UpdateMatchState(ctx, userID, matchID, "seen"))
}
```

- [x] **Step 7: Run tests; verify failure**

Run: `go test ./internal/radar/ -run 'TestStore_(ListMatches|UpdateMatchState)' -v`
Expected: FAIL — methods undefined.

- [x] **Step 8: Implement `ListMatches` and `UpdateMatchState` in `store.go`**

Append to `internal/radar/store.go`:

```go
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
```

- [x] **Step 9: Run tests; verify pass**

Run: `go test ./internal/radar/ -run 'TestStore_(ListMatches|UpdateMatchState)' -v`
Expected: PASS for all six tests.

- [x] **Step 10: Commit**

```bash
git add internal/radar/store.go internal/radar/store_test.go
git commit -m "feat(radar): add ListMatches and UpdateMatchState store methods"
```

---

## Task 5: Store — `LastSweepAt` and `ListFeeds`

**Files:**
- Modify: `internal/radar/store.go`
- Modify: `internal/radar/store_test.go`

Two small, independent queries.

- [x] **Step 1: Write failing tests for `LastSweepAt` and `ListFeeds`**

Append to `store_test.go`:

```go
func TestStore_LastSweepAt_noSubs(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)

	last, err := store.LastSweepAt(ctx, userID)
	require.NoError(t, err)
	require.Nil(t, last)
}

func TestStore_LastSweepAt_picksMax(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)
	feed1 := seedFeed(t, pool, "https://f1.example/rss", "F1")
	feed2 := seedFeed(t, pool, "https://f2.example/rss", "F2")
	seedSubscription(t, pool, userID, feed1)
	seedSubscription(t, pool, userID, feed2)

	// Set distinct fetch timestamps; feed2 is most recent.
	_, err := pool.Exec(ctx,
		`UPDATE radar_feeds SET last_fetched_at = $1 WHERE id = $2`,
		time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC), feed1)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`UPDATE radar_feeds SET last_fetched_at = $1 WHERE id = $2`,
		time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC), feed2)
	require.NoError(t, err)

	last, err := store.LastSweepAt(ctx, userID)
	require.NoError(t, err)
	require.NotNil(t, last)
	require.Equal(t, 12, last.UTC().Hour())
}

func TestStore_ListFeeds_pagination(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	for i := 0; i < 4; i++ {
		seedFeed(t, pool,
			fmt.Sprintf("https://feed-%d.example/rss", i),
			fmt.Sprintf("Feed %d", i))
	}

	items, total, err := store.ListFeeds(ctx, radar.ListFeedsParams{Limit: 2, Offset: 0})
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, 4, total)

	items, _, err = store.ListFeeds(ctx, radar.ListFeedsParams{Limit: 2, Offset: 3})
	require.NoError(t, err)
	require.Len(t, items, 1)
}
```

Add `"fmt"` and `"time"` to `store_test.go`'s import block if not already present.

- [x] **Step 2: Run tests; verify failure**

Run: `go test ./internal/radar/ -run 'TestStore_(LastSweepAt|ListFeeds)' -v`
Expected: FAIL — methods undefined.

- [x] **Step 3: Implement `LastSweepAt` and `ListFeeds`**

Append to `internal/radar/store.go`. Add `time` import if not yet present (it is, indirectly, but make sure the line compiles).

```go
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
```

Add to the import block in `store.go` if not already there: `"time"`.

- [x] **Step 4: Run tests; verify pass**

Run: `go test ./internal/radar/ -run 'TestStore_(LastSweepAt|ListFeeds)' -v`
Expected: PASS for all three tests.

- [x] **Step 5: Commit**

```bash
git add internal/radar/store.go internal/radar/store_test.go
git commit -m "feat(radar): add LastSweepAt and ListFeeds store methods"
```

---

## Task 6: Extend `StoreAPI` interface and `mockStore`

**Files:**
- Modify: `internal/radar/service.go` (interface only)
- Modify: `internal/radar/service_test.go` (mockStore stubs)

Service tests use a mockStore that satisfies `StoreAPI`. Before adding service methods we must extend the interface and add stubs (returning sentinel errors) so the package keeps compiling.

- [x] **Step 1: Extend `StoreAPI` in `service.go`**

Replace the existing `StoreAPI` interface block:

```go
type StoreAPI interface {
	CreateTopic(ctx context.Context, p CreateTopicParams) (*Topic, error)
	UpdateTopicEmbedding(ctx context.Context, topicID int64, vec pgvector.Vector) error
	AddFeed(ctx context.Context, p AddFeedParams) (*Feed, error)
	Subscribe(ctx context.Context, userID, feedID int64) (*Subscription, error)

	// Read-API extensions:
	ListTopicsWithStats(ctx context.Context, userID int64) ([]TopicWithStats, error)
	GetTopicWithStats(ctx context.Context, userID, topicID int64) (*TopicWithStats, error)
	UpdateTopic(ctx context.Context, userID, topicID int64, p UpdateTopicParams) (*Topic, error)
	DeleteTopic(ctx context.Context, userID, topicID int64) error
	ListMatches(ctx context.Context, userID int64, p ListMatchesParams) ([]MatchView, int, error)
	UpdateMatchState(ctx context.Context, userID, matchID int64, state string) error
	LastSweepAt(ctx context.Context, userID int64) (*time.Time, error)
	ListFeeds(ctx context.Context, p ListFeedsParams) ([]Feed, int, error)
}
```

Add `"time"` to `service.go`'s import block if not present.

- [x] **Step 2: Add mockStore stubs in `service_test.go`**

Append to the mockStore in `service_test.go` (after the existing methods). The stubs return zero values + sentinel errors that tests can override per scenario via fields. For simplicity start with simple "return nil" or "return errStub". Add the following fields to the `mockStore` struct definition (find it and extend):

```go
type mockStore struct {
	topics         map[int64]*radar.Topic
	topicEmb       map[int64]pgvector.Vector
	feeds          map[int64]*radar.Feed
	feedsByURL     map[string]*radar.Feed
	subs           map[string]*radar.Subscription
	nextTopicID    int64
	nextFeedID     int64
	createTopicErr error
	addFeedErr     error
	subscribeErr   error
	updateEmbErr   error

	// Read-API recording / overrides:
	listTopicsResult   []radar.TopicWithStats
	listTopicsErr      error
	getTopicResult     *radar.TopicWithStats
	getTopicErr        error
	updateTopicResult  *radar.Topic
	updateTopicErr     error
	updateTopicCalled  bool
	updateTopicParams  radar.UpdateTopicParams
	deleteTopicErr     error
	deleteTopicCalled  bool
	listMatchesResult  []radar.MatchView
	listMatchesTotal   int
	listMatchesErr     error
	listMatchesCalled  bool
	listMatchesParams  radar.ListMatchesParams
	updateMatchErr     error
	updateMatchCalled  bool
	updateMatchState   string
	lastSweepResult    *time.Time
	lastSweepErr       error
	listFeedsResult    []radar.Feed
	listFeedsTotal     int
	listFeedsErr       error
}
```

Append the new methods:

```go
func (m *mockStore) ListTopicsWithStats(_ context.Context, _ int64) ([]radar.TopicWithStats, error) {
	return m.listTopicsResult, m.listTopicsErr
}

func (m *mockStore) GetTopicWithStats(_ context.Context, _, _ int64) (*radar.TopicWithStats, error) {
	if m.getTopicErr != nil {
		return nil, m.getTopicErr
	}
	if m.getTopicResult == nil {
		return nil, radar.ErrNotFound
	}
	return m.getTopicResult, nil
}

func (m *mockStore) UpdateTopic(_ context.Context, _, _ int64, p radar.UpdateTopicParams) (*radar.Topic, error) {
	m.updateTopicCalled = true
	m.updateTopicParams = p
	if m.updateTopicErr != nil {
		return nil, m.updateTopicErr
	}
	if m.updateTopicResult != nil {
		return m.updateTopicResult, nil
	}
	// Default: synthesize a Topic reflecting params.
	t := radar.Topic{ID: 1, Name: "default"}
	if p.Name != nil {
		t.Name = *p.Name
	}
	if p.Description != nil {
		t.Description = *p.Description
	}
	if p.MatchThreshold != nil {
		t.MatchThreshold = *p.MatchThreshold
	}
	if p.IsActive != nil {
		t.IsActive = *p.IsActive
	}
	return &t, nil
}

func (m *mockStore) DeleteTopic(_ context.Context, _, _ int64) error {
	m.deleteTopicCalled = true
	return m.deleteTopicErr
}

func (m *mockStore) ListMatches(_ context.Context, _ int64, p radar.ListMatchesParams) ([]radar.MatchView, int, error) {
	m.listMatchesCalled = true
	m.listMatchesParams = p
	return m.listMatchesResult, m.listMatchesTotal, m.listMatchesErr
}

func (m *mockStore) UpdateMatchState(_ context.Context, _, _ int64, state string) error {
	m.updateMatchCalled = true
	m.updateMatchState = state
	return m.updateMatchErr
}

func (m *mockStore) LastSweepAt(_ context.Context, _ int64) (*time.Time, error) {
	return m.lastSweepResult, m.lastSweepErr
}

func (m *mockStore) ListFeeds(_ context.Context, _ radar.ListFeedsParams) ([]radar.Feed, int, error) {
	return m.listFeedsResult, m.listFeedsTotal, m.listFeedsErr
}
```

Make sure `service_test.go` imports `"time"` (it already does — used by existing mock methods).

- [x] **Step 3: Compile to verify interface satisfaction**

Run: `go build ./internal/radar/...`
Expected: PASS — both the real `Store` and `mockStore` now satisfy `StoreAPI`.

- [x] **Step 4: Re-run existing tests; verify nothing broke**

Run: `go test ./internal/radar/... -v -short`
Expected: PASS for all pre-existing tests.

- [x] **Step 5: Commit**

```bash
git add internal/radar/service.go internal/radar/service_test.go
git commit -m "refactor(radar): extend StoreAPI and mockStore with read-API stubs"
```

---

## Task 7: Service — `ListTopics`, `GetTopic`, `DeleteTopic`

**Files:**
- Modify: `internal/radar/service.go`
- Modify: `internal/radar/service_test.go`

Thin wrappers; validation lives at HTTP layer only for empty/path errors. These three don't touch the embedder.

- [x] **Step 1: Write failing test `TestService_ListTopics_passesThrough`**

Append to `service_test.go`:

```go
func TestService_ListTopics_passesThrough(t *testing.T) {
	store := newMockStore()
	store.listTopicsResult = []radar.TopicWithStats{
		{Topic: radar.Topic{ID: 1, Name: "A"}},
		{Topic: radar.Topic{ID: 2, Name: "B"}},
	}
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	got, err := svc.ListTopics(context.Background(), 42)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, int64(1), got[0].ID)
}
```

- [x] **Step 2: Write failing test `TestService_GetTopic_notFound`**

```go
func TestService_GetTopic_notFound(t *testing.T) {
	store := newMockStore()
	store.getTopicErr = radar.ErrNotFound
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	_, err := svc.GetTopic(context.Background(), 1, 999)
	require.ErrorIs(t, err, radar.ErrNotFound)
}
```

- [x] **Step 3: Write failing test `TestService_DeleteTopic_passesThrough`**

```go
func TestService_DeleteTopic_passesThrough(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	require.NoError(t, svc.DeleteTopic(context.Background(), 1, 7))
	require.True(t, store.deleteTopicCalled)

	store.deleteTopicErr = radar.ErrNotFound
	store.deleteTopicCalled = false
	require.ErrorIs(t, svc.DeleteTopic(context.Background(), 1, 7), radar.ErrNotFound)
	require.True(t, store.deleteTopicCalled)
}
```

- [x] **Step 4: Run tests; verify failure**

Run: `go test ./internal/radar/ -run 'TestService_(ListTopics|GetTopic|DeleteTopic)' -v -short`
Expected: FAIL — methods undefined.

- [x] **Step 5: Implement the three service methods**

Append to `internal/radar/service.go`:

```go
// ListTopics returns all topics of a user with aggregate match stats.
func (s *Service) ListTopics(ctx context.Context, userID int64) ([]TopicWithStats, error) {
	return s.store.ListTopicsWithStats(ctx, userID)
}

// GetTopic returns a single topic with aggregate match stats, or ErrNotFound.
func (s *Service) GetTopic(ctx context.Context, userID, topicID int64) (*TopicWithStats, error) {
	return s.store.GetTopicWithStats(ctx, userID, topicID)
}

// DeleteTopic removes a topic and CASCADEs its matches.
func (s *Service) DeleteTopic(ctx context.Context, userID, topicID int64) error {
	return s.store.DeleteTopic(ctx, userID, topicID)
}
```

- [x] **Step 6: Run tests; verify pass**

Run: `go test ./internal/radar/ -run 'TestService_(ListTopics|GetTopic|DeleteTopic)' -v -short`
Expected: PASS.

- [x] **Step 7: Commit**

```bash
git add internal/radar/service.go internal/radar/service_test.go
git commit -m "feat(radar): add ListTopics, GetTopic, DeleteTopic service methods"
```

---

## Task 8: Service — `UpdateTopic` (embedder-aware)

**Files:**
- Modify: `internal/radar/service.go`
- Modify: `internal/radar/service_test.go`

This is the most subtle method: validates fields, calls `store.UpdateTopic`, and if `description` was in the patch, calls `embedder.Embed` and `store.UpdateTopicEmbedding`. Mirrors `CreateTopic` behavior on embedder failure (fields persisted, embedding stale, `ErrEmbedderUnavailable` returned).

- [x] **Step 1: Write failing test `TestService_UpdateTopic_noFields`**

Append to `service_test.go`:

```go
func TestService_UpdateTopic_noFields(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	_, err := svc.UpdateTopic(context.Background(), 1, 7, radar.UpdateTopicRequest{})
	require.ErrorIs(t, err, radar.ErrInvalidInput)
	require.False(t, store.updateTopicCalled)
}
```

- [x] **Step 2: Write failing test `TestService_UpdateTopic_validation`**

```go
func TestService_UpdateTopic_validation(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	emptyName := ""
	_, err := svc.UpdateTopic(context.Background(), 1, 7,
		radar.UpdateTopicRequest{Name: &emptyName})
	require.ErrorIs(t, err, radar.ErrInvalidInput)

	bad := float32(1.5)
	_, err = svc.UpdateTopic(context.Background(), 1, 7,
		radar.UpdateTopicRequest{MatchThreshold: &bad})
	require.ErrorIs(t, err, radar.ErrInvalidInput)

	shortDesc := "tiny"
	_, err = svc.UpdateTopic(context.Background(), 1, 7,
		radar.UpdateTopicRequest{Description: &shortDesc})
	require.ErrorIs(t, err, radar.ErrInvalidInput)

	require.False(t, store.updateTopicCalled)
}
```

- [x] **Step 3: Write failing test `TestService_UpdateTopic_nameOnly_noEmbed`**

```go
func TestService_UpdateTopic_nameOnly_noEmbed(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	name := "new name"
	got, err := svc.UpdateTopic(context.Background(), 1, 7,
		radar.UpdateTopicRequest{Name: &name})
	require.NoError(t, err)
	require.Equal(t, "new name", got.Name)
	require.True(t, store.updateTopicCalled)
	require.NotNil(t, store.updateTopicParams.Name)
	require.Equal(t, "new name", *store.updateTopicParams.Name)
	require.Nil(t, store.updateTopicParams.Description)
	// embedder was not invoked: UpdateTopicEmbedding stores nothing in mock.
	// Confirm topicEmb is empty.
	require.Empty(t, store.topicEmb)
}
```

- [x] **Step 4: Write failing test `TestService_UpdateTopic_descriptionTriggersEmbed`**

```go
func TestService_UpdateTopic_descriptionTriggersEmbed(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	// updateTopicResult dictates what UpdateTopic returns (used to derive
	// embedder input "name: description").
	store.updateTopicResult = &radar.Topic{
		ID: 7, Name: "name", Description: "new long description here",
	}

	desc := "new long description here"
	got, err := svc.UpdateTopic(context.Background(), 1, 7,
		radar.UpdateTopicRequest{Description: &desc})
	require.NoError(t, err)
	require.Equal(t, "new long description here", got.Description)
	require.True(t, store.updateTopicCalled)
	require.NotEmpty(t, store.topicEmb, "embedder should have written via UpdateTopicEmbedding")
}
```

- [x] **Step 5: Write failing test `TestService_UpdateTopic_embedderUnavailable`**

```go
func TestService_UpdateTopic_embedderUnavailable(t *testing.T) {
	store := newMockStore()
	store.updateTopicResult = &radar.Topic{
		ID: 7, Name: "name", Description: "new long description here",
	}
	svc := radar.NewService(store, &errEmbedder{err: errors.New("conn refused")})

	desc := "new long description here"
	_, err := svc.UpdateTopic(context.Background(), 1, 7,
		radar.UpdateTopicRequest{Description: &desc})
	require.ErrorIs(t, err, radar.ErrEmbedderUnavailable)
	require.True(t, store.updateTopicCalled, "fields are persisted before embed attempt")
}
```

`errEmbedder` already exists in `service_test.go` (used by existing CreateTopic tests).

- [x] **Step 6: Run tests; verify failure**

Run: `go test ./internal/radar/ -run 'TestService_UpdateTopic' -v -short`
Expected: FAIL — `UpdateTopic` undefined.

- [x] **Step 7: Implement `Service.UpdateTopic`**

Append to `internal/radar/service.go`:

```go
// UpdateTopic validates the patch, persists changed fields, and — if
// `description` was in the patch — re-embeds the topic. Mirrors CreateTopic:
// embedder failure leaves the topic's fields updated and embedding stale, and
// returns ErrEmbedderUnavailable. The caller can retry with the same payload.
func (s *Service) UpdateTopic(ctx context.Context, userID, topicID int64, req UpdateTopicRequest) (*Topic, error) {
	p := UpdateTopicParams{
		Name:           req.Name,
		Description:    req.Description,
		MatchThreshold: req.MatchThreshold,
		IsActive:       req.IsActive,
	}

	if p.Name == nil && p.Description == nil && p.MatchThreshold == nil && p.IsActive == nil {
		return nil, fmt.Errorf("%w: no fields to update", ErrInvalidInput)
	}

	if p.Name != nil {
		n := strings.TrimSpace(*p.Name)
		if n == "" || len(n) > 200 {
			return nil, fmt.Errorf("%w: name must be 1..200 chars", ErrInvalidInput)
		}
		p.Name = &n
	}
	if p.Description != nil {
		d := strings.TrimSpace(*p.Description)
		if len(d) < 10 || len(d) > 2000 {
			return nil, fmt.Errorf("%w: description must be 10..2000 chars", ErrInvalidInput)
		}
		p.Description = &d
	}
	if p.MatchThreshold != nil {
		if *p.MatchThreshold < 0 || *p.MatchThreshold > 1 {
			return nil, fmt.Errorf("%w: match_threshold must be in [0,1]", ErrInvalidInput)
		}
	}

	topic, err := s.store.UpdateTopic(ctx, userID, topicID, p)
	if err != nil {
		return nil, err
	}

	if p.Description != nil {
		vec, err := s.embedder.Embed(ctx, topic.Name+": "+topic.Description)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrEmbedderUnavailable, err)
		}
		if err := s.store.UpdateTopicEmbedding(ctx, topic.ID, pgvector.NewVector(vec)); err != nil {
			if errors.Is(err, ErrNotFound) {
				return nil, err
			}
			return nil, fmt.Errorf("save embedding: %w", err)
		}
		topic.HasEmbedding = true
	}

	return topic, nil
}
```

- [x] **Step 8: Run tests; verify pass**

Run: `go test ./internal/radar/ -run 'TestService_UpdateTopic' -v -short`
Expected: PASS for all five tests.

- [x] **Step 9: Commit**

```bash
git add internal/radar/service.go internal/radar/service_test.go
git commit -m "feat(radar): add UpdateTopic service method with embedder re-compute"
```

---

## Task 9: Service — `ListMatches` and `SetMatchState`

**Files:**
- Modify: `internal/radar/service.go`
- Modify: `internal/radar/service_test.go`

`ListMatches` clamps `limit` and defaults to 50 / 0. `SetMatchState` validates the enum.

- [x] **Step 1: Write failing tests**

Append to `service_test.go`:

```go
func TestService_ListMatches_clampLimit(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	_, err := svc.ListMatches(context.Background(), radar.ListMatchesParams{UserID: 1, Limit: 200})
	require.NoError(t, err)
	require.Equal(t, 100, store.listMatchesParams.Limit)

	store.listMatchesCalled = false
	_, err = svc.ListMatches(context.Background(), radar.ListMatchesParams{UserID: 1, Limit: 0})
	require.NoError(t, err)
	require.Equal(t, 50, store.listMatchesParams.Limit)

	store.listMatchesCalled = false
	_, err = svc.ListMatches(context.Background(), radar.ListMatchesParams{UserID: 1, Limit: 25, Offset: -3})
	require.NoError(t, err)
	require.Equal(t, 25, store.listMatchesParams.Limit)
	require.Equal(t, 0, store.listMatchesParams.Offset)
}

func TestService_ListMatches_returnsResult(t *testing.T) {
	store := newMockStore()
	store.listMatchesResult = []radar.MatchView{{ID: 1}, {ID: 2}}
	store.listMatchesTotal = 2
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	got, err := svc.ListMatches(context.Background(), radar.ListMatchesParams{UserID: 1, Limit: 10})
	require.NoError(t, err)
	require.Len(t, got.Items, 2)
	require.Equal(t, 2, got.Total)
}

func TestService_SetMatchState_validation(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	err := svc.SetMatchState(context.Background(), 1, 9, "foo")
	require.ErrorIs(t, err, radar.ErrInvalidInput)
	require.False(t, store.updateMatchCalled)

	require.NoError(t, svc.SetMatchState(context.Background(), 1, 9, "new"))
	require.NoError(t, svc.SetMatchState(context.Background(), 1, 9, "seen"))
	require.True(t, store.updateMatchCalled)
}

func TestService_SetMatchState_propagatesNotFound(t *testing.T) {
	store := newMockStore()
	store.updateMatchErr = radar.ErrNotFound
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	err := svc.SetMatchState(context.Background(), 1, 9, "seen")
	require.ErrorIs(t, err, radar.ErrNotFound)
}
```

- [x] **Step 2: Run tests; verify failure**

Run: `go test ./internal/radar/ -run 'TestService_(ListMatches|SetMatchState)' -v -short`
Expected: FAIL.

- [x] **Step 3: Implement `ListMatches` and `SetMatchState`**

Append to `internal/radar/service.go`:

```go
// ListMatches returns paginated matches for a user, optionally filtered by
// topic and/or state. Clamps Limit to [1,100] (default 50) and Offset to >=0.
func (s *Service) ListMatches(ctx context.Context, p ListMatchesParams) (*MatchList, error) {
	if p.Limit <= 0 || p.Limit > 100 {
		if p.Limit > 100 {
			p.Limit = 100
		} else {
			p.Limit = 50
		}
	}
	if p.Offset < 0 {
		p.Offset = 0
	}

	items, total, err := s.store.ListMatches(ctx, p.UserID, p)
	if err != nil {
		return nil, err
	}
	return &MatchList{Items: items, Total: total}, nil
}

// SetMatchState updates a match's state. Valid states: "new", "seen".
func (s *Service) SetMatchState(ctx context.Context, userID, matchID int64, state string) error {
	if state != "new" && state != "seen" {
		return fmt.Errorf("%w: state must be new|seen", ErrInvalidInput)
	}
	return s.store.UpdateMatchState(ctx, userID, matchID, state)
}
```

- [x] **Step 4: Run tests; verify pass**

Run: `go test ./internal/radar/ -run 'TestService_(ListMatches|SetMatchState)' -v -short`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add internal/radar/service.go internal/radar/service_test.go
git commit -m "feat(radar): add ListMatches and SetMatchState service methods"
```

---

## Task 10: Service — `LastSweep` and `ListFeeds`

**Files:**
- Modify: `internal/radar/service.go`
- Modify: `internal/radar/service_test.go`

Two thin wrappers; `ListFeeds` clamps pagination.

- [x] **Step 1: Write failing tests**

Append to `service_test.go`:

```go
func TestService_LastSweep_passesThrough(t *testing.T) {
	store := newMockStore()
	when := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	store.lastSweepResult = &when
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	got, err := svc.LastSweep(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, when, *got)
}

func TestService_ListFeeds_clampsPagination(t *testing.T) {
	store := newMockStore()
	store.listFeedsResult = []radar.Feed{{ID: 1}}
	store.listFeedsTotal = 1
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	got, err := svc.ListFeeds(context.Background(), radar.ListFeedsParams{Limit: 0, Offset: -1})
	require.NoError(t, err)
	require.Len(t, got.Items, 1)
	require.Equal(t, 1, got.Total)
}
```

- [x] **Step 2: Run tests; verify failure**

Run: `go test ./internal/radar/ -run 'TestService_(LastSweep|ListFeeds)' -v -short`
Expected: FAIL.

- [x] **Step 3: Implement `LastSweep` and `ListFeeds`**

Append to `internal/radar/service.go`:

```go
// LastSweep returns the latest fetch timestamp across the user's active
// subscribed feeds, or nil if there are no subscriptions.
func (s *Service) LastSweep(ctx context.Context, userID int64) (*time.Time, error) {
	return s.store.LastSweepAt(ctx, userID)
}

// ListFeeds returns paginated feeds (admin scope; middleware enforces).
func (s *Service) ListFeeds(ctx context.Context, p ListFeedsParams) (*FeedList, error) {
	if p.Limit <= 0 || p.Limit > 100 {
		if p.Limit > 100 {
			p.Limit = 100
		} else {
			p.Limit = 50
		}
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	items, total, err := s.store.ListFeeds(ctx, p)
	if err != nil {
		return nil, err
	}
	return &FeedList{Items: items, Total: total}, nil
}
```

Add `"time"` to `service.go` import block if not already there.

- [x] **Step 4: Run tests; verify pass**

Run: `go test ./internal/radar/ -run 'TestService_(LastSweep|ListFeeds)' -v -short`
Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add internal/radar/service.go internal/radar/service_test.go
git commit -m "feat(radar): add LastSweep and ListFeeds service methods"
```

---

## Task 11: HTTP — topic handlers (List/Get/Update/Delete)

**Files:**
- Modify: `internal/radar/http.go`
- Modify: `internal/radar/http_test.go`

Tests call handlers directly. ID-based handlers need a `chi.RouteContext` with `id` set. Use this helper at the top of `http_test.go` (right after `userOnlyContext`):

```go
// withRouteID attaches a chi.RouteContext with the named URL param to ctx.
// Use for handlers that read chi.URLParam(r, "id").
func withRouteID(ctx context.Context, id string) context.Context {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return context.WithValue(ctx, chi.RouteCtxKey, rctx)
}
```

This requires `"github.com/go-chi/chi/v5"` in the imports — add if not present.

- [ ] **Step 1: Add `withRouteID` helper to `http_test.go`**

Add the helper function (and import) as shown above.

- [ ] **Step 2: Write failing test `TestHTTP_ListTopics_200`**

Append to `http_test.go`:

```go
func TestHTTP_ListTopics_200(t *testing.T) {
	store := newMockStore()
	store.listTopicsResult = []radar.TopicWithStats{
		{Topic: radar.Topic{ID: 1, Name: "A"}},
	}
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodGet, "/radar/topics", nil)
	req = req.WithContext(userOnlyContext(req.Context(), 1, false))
	rec := httptest.NewRecorder()
	h.ListTopicsHandler()(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var body struct {
		Items []radar.TopicWithStats `json:"items"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	require.Len(t, body.Items, 1)
	require.Equal(t, "A", body.Items[0].Name)
}
```

- [ ] **Step 3: Write failing test `TestHTTP_GetTopic_200_and_404`**

```go
func TestHTTP_GetTopic_200(t *testing.T) {
	store := newMockStore()
	store.getTopicResult = &radar.TopicWithStats{Topic: radar.Topic{ID: 7, Name: "X"}}
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodGet, "/radar/topics/7", nil)
	ctx := userOnlyContext(req.Context(), 1, false)
	ctx = withRouteID(ctx, "7")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h.GetTopicHandler()(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func TestHTTP_GetTopic_404(t *testing.T) {
	store := newMockStore()
	store.getTopicErr = radar.ErrNotFound
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodGet, "/radar/topics/7", nil)
	req = req.WithContext(withRouteID(userOnlyContext(req.Context(), 1, false), "7"))
	rec := httptest.NewRecorder()
	h.GetTopicHandler()(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHTTP_GetTopic_400_badID(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodGet, "/radar/topics/abc", nil)
	req = req.WithContext(withRouteID(userOnlyContext(req.Context(), 1, false), "abc"))
	rec := httptest.NewRecorder()
	h.GetTopicHandler()(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}
```

- [ ] **Step 4: Write failing test `TestHTTP_UpdateTopic_*`**

```go
func TestHTTP_UpdateTopic_200(t *testing.T) {
	store := newMockStore()
	store.updateTopicResult = &radar.Topic{ID: 7, Name: "renamed", IsActive: true}
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	name := "renamed"
	body, _ := json.Marshal(radar.UpdateTopicRequest{Name: &name})
	req := httptest.NewRequest(http.MethodPatch, "/radar/topics/7", bytes.NewReader(body))
	req = req.WithContext(withRouteID(userOnlyContext(req.Context(), 1, false), "7"))
	rec := httptest.NewRecorder()
	h.UpdateTopicHandler()(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var got radar.Topic
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
	require.Equal(t, "renamed", got.Name)
}

func TestHTTP_UpdateTopic_400_emptyPatch(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodPatch, "/radar/topics/7", strings.NewReader(`{}`))
	req = req.WithContext(withRouteID(userOnlyContext(req.Context(), 1, false), "7"))
	rec := httptest.NewRecorder()
	h.UpdateTopicHandler()(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHTTP_UpdateTopic_404(t *testing.T) {
	store := newMockStore()
	store.updateTopicErr = radar.ErrNotFound
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	name := "renamed"
	body, _ := json.Marshal(radar.UpdateTopicRequest{Name: &name})
	req := httptest.NewRequest(http.MethodPatch, "/radar/topics/7", bytes.NewReader(body))
	req = req.WithContext(withRouteID(userOnlyContext(req.Context(), 1, false), "7"))
	rec := httptest.NewRecorder()
	h.UpdateTopicHandler()(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}
```

- [ ] **Step 5: Write failing test `TestHTTP_DeleteTopic_*`**

```go
func TestHTTP_DeleteTopic_204(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodDelete, "/radar/topics/7", nil)
	req = req.WithContext(withRouteID(userOnlyContext(req.Context(), 1, false), "7"))
	rec := httptest.NewRecorder()
	h.DeleteTopicHandler()(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)
}

func TestHTTP_DeleteTopic_404(t *testing.T) {
	store := newMockStore()
	store.deleteTopicErr = radar.ErrNotFound
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodDelete, "/radar/topics/7", nil)
	req = req.WithContext(withRouteID(userOnlyContext(req.Context(), 1, false), "7"))
	rec := httptest.NewRecorder()
	h.DeleteTopicHandler()(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}
```

- [ ] **Step 6: Run tests; verify failure**

Run: `go test ./internal/radar/ -run 'TestHTTP_(ListTopics|GetTopic|UpdateTopic|DeleteTopic)' -v -short`
Expected: FAIL — handlers undefined.

- [ ] **Step 7: Implement topic handlers in `http.go`**

Append to `internal/radar/http.go`. The `chi/v5` import is needed — add if absent:

```go
import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	coreauth "github.com/ismd/linktheca/internal/core/auth"
	"github.com/ismd/linktheca/internal/core/httpx"
)
```

Then add:

```go
// ListTopicsHandler returns the http.HandlerFunc for GET /radar/topics.
func (h *HTTP) ListTopicsHandler() http.HandlerFunc { return h.listTopics }

// GetTopicHandler returns the http.HandlerFunc for GET /radar/topics/{id}.
func (h *HTTP) GetTopicHandler() http.HandlerFunc { return h.getTopic }

// UpdateTopicHandler returns the http.HandlerFunc for PATCH /radar/topics/{id}.
func (h *HTTP) UpdateTopicHandler() http.HandlerFunc { return h.updateTopic }

// DeleteTopicHandler returns the http.HandlerFunc for DELETE /radar/topics/{id}.
func (h *HTTP) DeleteTopicHandler() http.HandlerFunc { return h.deleteTopic }

func parseRadarID(r *http.Request) (int64, error) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}

func (h *HTTP) listTopics(w http.ResponseWriter, r *http.Request) {
	userID := coreauth.UserID(r.Context())
	items, err := h.svc.ListTopics(r.Context(), userID)
	if err != nil {
		writeRadarError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *HTTP) getTopic(w http.ResponseWriter, r *http.Request) {
	userID := coreauth.UserID(r.Context())
	id, err := parseRadarID(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	topic, err := h.svc.GetTopic(r.Context(), userID, id)
	if err != nil {
		writeRadarError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, topic)
}

func (h *HTTP) updateTopic(w http.ResponseWriter, r *http.Request) {
	userID := coreauth.UserID(r.Context())
	id, err := parseRadarID(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	var req UpdateTopicRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}
	topic, err := h.svc.UpdateTopic(r.Context(), userID, id, req)
	if err != nil {
		writeRadarError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, topic)
}

func (h *HTTP) deleteTopic(w http.ResponseWriter, r *http.Request) {
	userID := coreauth.UserID(r.Context())
	id, err := parseRadarID(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	if err := h.svc.DeleteTopic(r.Context(), userID, id); err != nil {
		writeRadarError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 8: Run tests; verify pass**

Run: `go test ./internal/radar/ -run 'TestHTTP_(ListTopics|GetTopic|UpdateTopic|DeleteTopic)' -v -short`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/radar/http.go internal/radar/http_test.go
git commit -m "feat(radar): add topic read-API HTTP handlers"
```

---

## Task 12: HTTP — match + status + feeds handlers

**Files:**
- Modify: `internal/radar/http.go`
- Modify: `internal/radar/http_test.go`

`ListMatches` parses `topic_id`, `state`, `limit`, `offset` from query. `UpdateMatch` reads body. `Status` is parameterless. `ListFeeds` is admin-only (middleware enforces, not handler).

- [ ] **Step 1: Write failing tests for matches**

Append to `http_test.go`:

```go
func TestHTTP_ListMatches_200_filters(t *testing.T) {
	store := newMockStore()
	store.listMatchesResult = []radar.MatchView{{ID: 1, TopicID: 7, State: "new"}}
	store.listMatchesTotal = 1
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodGet, "/radar/matches?topic_id=7&state=new&limit=10", nil)
	req = req.WithContext(userOnlyContext(req.Context(), 1, false))
	rec := httptest.NewRecorder()
	h.ListMatchesHandler()(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	require.NotNil(t, store.listMatchesParams.TopicID)
	require.Equal(t, int64(7), *store.listMatchesParams.TopicID)
	require.NotNil(t, store.listMatchesParams.State)
	require.Equal(t, "new", *store.listMatchesParams.State)
	require.Equal(t, 10, store.listMatchesParams.Limit)
}

func TestHTTP_ListMatches_200_noFilters(t *testing.T) {
	store := newMockStore()
	store.listMatchesResult = []radar.MatchView{}
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodGet, "/radar/matches", nil)
	req = req.WithContext(userOnlyContext(req.Context(), 1, false))
	rec := httptest.NewRecorder()
	h.ListMatchesHandler()(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Nil(t, store.listMatchesParams.TopicID)
	require.Nil(t, store.listMatchesParams.State)
	require.Equal(t, 50, store.listMatchesParams.Limit) // service default
}

func TestHTTP_ListMatches_400_badTopicID(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodGet, "/radar/matches?topic_id=abc", nil)
	req = req.WithContext(userOnlyContext(req.Context(), 1, false))
	rec := httptest.NewRecorder()
	h.ListMatchesHandler()(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHTTP_UpdateMatch_200(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	body, _ := json.Marshal(radar.UpdateMatchRequest{State: "seen"})
	req := httptest.NewRequest(http.MethodPatch, "/radar/matches/42", bytes.NewReader(body))
	req = req.WithContext(withRouteID(userOnlyContext(req.Context(), 1, false), "42"))
	rec := httptest.NewRecorder()
	h.UpdateMatchHandler()(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Equal(t, "seen", store.updateMatchState)
}

func TestHTTP_UpdateMatch_400_badEnum(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	body, _ := json.Marshal(radar.UpdateMatchRequest{State: "archived"})
	req := httptest.NewRequest(http.MethodPatch, "/radar/matches/42", bytes.NewReader(body))
	req = req.WithContext(withRouteID(userOnlyContext(req.Context(), 1, false), "42"))
	rec := httptest.NewRecorder()
	h.UpdateMatchHandler()(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHTTP_UpdateMatch_404(t *testing.T) {
	store := newMockStore()
	store.updateMatchErr = radar.ErrNotFound
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	body, _ := json.Marshal(radar.UpdateMatchRequest{State: "seen"})
	req := httptest.NewRequest(http.MethodPatch, "/radar/matches/42", bytes.NewReader(body))
	req = req.WithContext(withRouteID(userOnlyContext(req.Context(), 1, false), "42"))
	rec := httptest.NewRecorder()
	h.UpdateMatchHandler()(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}
```

- [ ] **Step 2: Write failing tests for status + feeds**

```go
func TestHTTP_Status_200_withLastSweep(t *testing.T) {
	store := newMockStore()
	when := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	store.lastSweepResult = &when
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodGet, "/radar/status", nil)
	req = req.WithContext(userOnlyContext(req.Context(), 1, false))
	rec := httptest.NewRecorder()
	h.StatusHandler()(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var body radar.RadarStatus
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	require.NotNil(t, body.LastSweepAt)
}

func TestHTTP_Status_200_null(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodGet, "/radar/status", nil)
	req = req.WithContext(userOnlyContext(req.Context(), 1, false))
	rec := httptest.NewRecorder()
	h.StatusHandler()(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var body radar.RadarStatus
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	require.Nil(t, body.LastSweepAt)
}

func TestHTTP_ListFeeds_200(t *testing.T) {
	store := newMockStore()
	store.listFeedsResult = []radar.Feed{{ID: 1, URL: "https://x.example/rss"}}
	store.listFeedsTotal = 1
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodGet, "/radar/feeds?limit=10", nil)
	req = req.WithContext(userOnlyContext(req.Context(), 1, true))
	rec := httptest.NewRecorder()
	h.ListFeedsHandler()(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}
```

Add `"time"` to `http_test.go`'s import block if not yet present.

- [ ] **Step 3: Run tests; verify failure**

Run: `go test ./internal/radar/ -run 'TestHTTP_(ListMatches|UpdateMatch|Status|ListFeeds)' -v -short`
Expected: FAIL.

- [ ] **Step 4: Implement match + status + feeds handlers**

Append to `internal/radar/http.go`:

```go
// ListMatchesHandler returns the http.HandlerFunc for GET /radar/matches.
func (h *HTTP) ListMatchesHandler() http.HandlerFunc { return h.listMatches }

// UpdateMatchHandler returns the http.HandlerFunc for PATCH /radar/matches/{id}.
func (h *HTTP) UpdateMatchHandler() http.HandlerFunc { return h.updateMatch }

// StatusHandler returns the http.HandlerFunc for GET /radar/status.
func (h *HTTP) StatusHandler() http.HandlerFunc { return h.status }

// ListFeedsHandler returns the http.HandlerFunc for GET /radar/feeds (admin).
func (h *HTTP) ListFeedsHandler() http.HandlerFunc { return h.listFeeds }

func (h *HTTP) listMatches(w http.ResponseWriter, r *http.Request) {
	userID := coreauth.UserID(r.Context())
	q := r.URL.Query()

	params := ListMatchesParams{UserID: userID}

	if topicStr := q.Get("topic_id"); topicStr != "" {
		topicID, err := strconv.ParseInt(topicStr, 10, 64)
		if err != nil || topicID <= 0 {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid topic_id")
			return
		}
		params.TopicID = &topicID
	}

	if state := q.Get("state"); state != "" {
		if state != "new" && state != "seen" {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "state must be new|seen")
			return
		}
		params.State = &state
	}

	if l, err := strconv.Atoi(q.Get("limit")); err == nil {
		params.Limit = l
	}
	if o, err := strconv.Atoi(q.Get("offset")); err == nil {
		params.Offset = o
	}

	result, err := h.svc.ListMatches(r.Context(), params)
	if err != nil {
		writeRadarError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *HTTP) updateMatch(w http.ResponseWriter, r *http.Request) {
	userID := coreauth.UserID(r.Context())
	id, err := parseRadarID(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	var req UpdateMatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}
	if err := h.svc.SetMatchState(r.Context(), userID, id, req.State); err != nil {
		writeRadarError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTP) status(w http.ResponseWriter, r *http.Request) {
	userID := coreauth.UserID(r.Context())
	last, err := h.svc.LastSweep(r.Context(), userID)
	if err != nil {
		writeRadarError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, RadarStatus{LastSweepAt: last})
}

func (h *HTTP) listFeeds(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	params := ListFeedsParams{}
	if l, err := strconv.Atoi(q.Get("limit")); err == nil {
		params.Limit = l
	}
	if o, err := strconv.Atoi(q.Get("offset")); err == nil {
		params.Offset = o
	}
	result, err := h.svc.ListFeeds(r.Context(), params)
	if err != nil {
		writeRadarError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}
```

- [ ] **Step 5: Run tests; verify pass**

Run: `go test ./internal/radar/ -run 'TestHTTP_(ListMatches|UpdateMatch|Status|ListFeeds)' -v -short`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/radar/http.go internal/radar/http_test.go
git commit -m "feat(radar): add match, status, feeds HTTP handlers"
```

---

## Task 13: Wire new routes in `server.go`

**Files:**
- Modify: `internal/server/server.go`

Extend the existing `r.Route("/radar", …)` block. Disabled-wildcard already covers the new paths.

- [ ] **Step 1: Add new routes inside the existing `/radar` route block**

Open `internal/server/server.go` and locate the existing `if cfg.RadarEnabled && deps.Radar != nil { … }` block (around line 105). Inside the `r.Route("/radar", …)` callback, between the existing handler registrations and the existing admin Group, add the new routes. The final block should look like this:

```go
r.Route("/radar", func(r chi.Router) {
	r.Use(coreauth.RequireUser(issuer))

	r.Post("/topics", radarHTTP.CreateTopicHandler())
	r.Get("/topics", radarHTTP.ListTopicsHandler())
	r.Get("/topics/{id}", radarHTTP.GetTopicHandler())
	r.Patch("/topics/{id}", radarHTTP.UpdateTopicHandler())
	r.Delete("/topics/{id}", radarHTTP.DeleteTopicHandler())

	r.Post("/subscriptions", radarHTTP.SubscribeHandler())

	r.Get("/matches", radarHTTP.ListMatchesHandler())
	r.Patch("/matches/{id}", radarHTTP.UpdateMatchHandler())

	r.Get("/status", radarHTTP.StatusHandler())

	r.Group(func(r chi.Router) {
		r.Use(coreauth.RequireAdmin)
		r.Post("/feeds", radarHTTP.AddFeedHandler())
		r.Get("/feeds", radarHTTP.ListFeedsHandler())
	})
})
```

- [ ] **Step 2: Verify compilation**

Run: `go build ./...`
Expected: PASS.

- [ ] **Step 3: Run the full radar test suite to confirm no regression**

Run: `go test ./internal/radar/... -v -short`
Expected: PASS for all existing + new tests.

- [ ] **Step 4: Commit**

```bash
git add internal/server/server.go
git commit -m "feat(server): wire Radar read-API routes"
```

---

## Task 14: Integration test — end-to-end Radar read flow

**Files:**
- Modify: `internal/radar/integration_test.go`

One scenario that walks through the full new surface. Reuses existing `seedRadarUser` and the chi router pattern.

- [ ] **Step 1: Write the failing integration scenario**

Append to `internal/radar/integration_test.go` (after the existing `TestIntegrationRadarFlow`):

```go
func TestIntegrationRadarReadAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := radar.NewStore(pool)
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)
	h := radar.NewHTTP(svc)

	userID := seedRadarUser(t, pool, true)
	issuer := coreauth.NewJWTIssuer("test-secret-at-least-32-bytes-long-for-hmac", 15*time.Minute)
	token, err := issuer.Issue(userID, true)
	require.NoError(t, err)
	auth := "Bearer " + token

	r := chi.NewRouter()
	r.Route("/radar", func(r chi.Router) {
		r.Use(coreauth.RequireUser(issuer))
		r.Post("/topics", h.CreateTopicHandler())
		r.Get("/topics", h.ListTopicsHandler())
		r.Get("/topics/{id}", h.GetTopicHandler())
		r.Patch("/topics/{id}", h.UpdateTopicHandler())
		r.Delete("/topics/{id}", h.DeleteTopicHandler())
		r.Post("/subscriptions", h.SubscribeHandler())
		r.Get("/matches", h.ListMatchesHandler())
		r.Patch("/matches/{id}", h.UpdateMatchHandler())
		r.Get("/status", h.StatusHandler())
		r.Group(func(r chi.Router) {
			r.Use(coreauth.RequireAdmin)
			r.Post("/feeds", h.AddFeedHandler())
			r.Get("/feeds", h.ListFeedsHandler())
		})
	})

	doJSON := func(method, path string, payload any) (*httptest.ResponseRecorder, []byte) {
		t.Helper()
		var body []byte
		if payload != nil {
			body, _ = json.Marshal(payload)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Authorization", auth)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec, rec.Body.Bytes()
	}

	// 1. Admin: add feed.
	rec, _ := doJSON(http.MethodPost, "/radar/feeds",
		radar.AddFeedRequest{URL: "https://news.example/rss"})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var feed radar.Feed
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &feed))

	// 2. Create topic + subscribe.
	rec, _ = doJSON(http.MethodPost, "/radar/topics",
		radar.CreateTopicRequest{Name: "ML", Description: "machine learning research and products"})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var topic radar.Topic
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &topic))

	rec, _ = doJSON(http.MethodPost, "/radar/subscriptions",
		radar.SubscribeRequest{FeedID: feed.ID})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	// 3. Seed a finding + match directly (bypass crawler).
	var findingID int64
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO radar_findings (feed_id, url, title) VALUES ($1, $2, $3) RETURNING id`,
		feed.ID, "https://news.example/a", "title-a").Scan(&findingID))
	var matchID int64
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO radar_topic_matches (topic_id, finding_id, similarity, state)
		 VALUES ($1, $2, $3, 'new') RETURNING id`,
		topic.ID, findingID, 0.7).Scan(&matchID))

	// Also stamp last_fetched_at so /status returns it.
	_, err = pool.Exec(context.Background(),
		`UPDATE radar_feeds SET last_fetched_at = now() WHERE id = $1`, feed.ID)
	require.NoError(t, err)

	// 4. GET /radar/topics returns aggregate stats.
	rec, _ = doJSON(http.MethodGet, "/radar/topics", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var topicsResp struct {
		Items []radar.TopicWithStats `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &topicsResp))
	require.Len(t, topicsResp.Items, 1)
	require.Equal(t, 1, topicsResp.Items[0].Stats.NewCount)
	require.Equal(t, 1, topicsResp.Items[0].Stats.TotalCount)
	require.Equal(t, 1, topicsResp.Items[0].Stats.SourceCount)

	// 5. GET /radar/matches?state=new
	rec, _ = doJSON(http.MethodGet, "/radar/matches?state=new", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var matchesResp radar.MatchList
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &matchesResp))
	require.Len(t, matchesResp.Items, 1)
	require.Equal(t, "ML", matchesResp.Items[0].TopicName)
	require.Equal(t, "https://news.example/a", matchesResp.Items[0].Finding.URL)

	// 6. PATCH match state → seen.
	rec, _ = doJSON(http.MethodPatch,
		fmt.Sprintf("/radar/matches/%d", matchID),
		radar.UpdateMatchRequest{State: "seen"})
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())

	// 7. GET /radar/topics shows new_count=0 now.
	rec, _ = doJSON(http.MethodGet, "/radar/topics", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &topicsResp))
	require.Equal(t, 0, topicsResp.Items[0].Stats.NewCount)
	require.Equal(t, 1, topicsResp.Items[0].Stats.TotalCount)

	// 8. GET /radar/status returns last_sweep_at.
	rec, _ = doJSON(http.MethodGet, "/radar/status", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var status radar.RadarStatus
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &status))
	require.NotNil(t, status.LastSweepAt)

	// 9. PATCH topic (rename).
	newName := "Machine Learning"
	rec, _ = doJSON(http.MethodPatch,
		fmt.Sprintf("/radar/topics/%d", topic.ID),
		radar.UpdateTopicRequest{Name: &newName})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// 10. DELETE topic; subsequent GET /radar/matches is empty (CASCADE).
	rec, _ = doJSON(http.MethodDelete,
		fmt.Sprintf("/radar/topics/%d", topic.ID), nil)
	require.Equal(t, http.StatusNoContent, rec.Code)

	rec, _ = doJSON(http.MethodGet, "/radar/matches", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &matchesResp))
	require.Empty(t, matchesResp.Items)
}
```

Add `"fmt"` to the integration_test.go import block if not already present.

- [ ] **Step 2: Run the integration test**

Run: `go test ./internal/radar/ -run 'TestIntegrationRadarReadAPI' -v`
Expected: PASS.

- [ ] **Step 3: Run the entire test suite to confirm no regression**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/radar/integration_test.go
git commit -m "test(radar): add integration test for read-API flow"
```

---

## Wrap-up

- [ ] Run `go vet ./...` — expect no warnings.
- [ ] Run `golangci-lint run ./...` if available — expect no new findings.
- [ ] Verify `LINKTHECA_RADAR_ENABLED=false` mode still works: `LINKTHECA_RADAR_ENABLED=false go test ./internal/server/...` (existing tests cover this; no changes needed).
- [ ] Update `MEMORY.md` only if a real surprise emerged during implementation. Routine "added endpoints" is not a memory.

## Self-review

**Spec coverage check (each spec section → task):**

- §1 endpoints (8 routes) — Tasks 11/12/13 implement and wire them.
- §2 response shapes (`TopicWithStats`, `MatchView`, `MatchFinding`, `Feed`, `RadarStatus`, `UpdateTopicRequest`, `UpdateMatchRequest`) — Task 1.
- §3 errors — reuse existing `writeRadarError` (no new task; tested in handler tests).
- §4 store (8 methods, SQL) — Tasks 2/3/4/5.
- §5 service (8 methods, embedder semantics) — Tasks 7/8/9/10.
- §6 HTTP — Tasks 11/12/13.
- §7 tests (store/service/http/integration) — interwoven across all tasks; integration consolidated in Task 14.
- §8 edge cases — covered by tests in their respective tasks (ownership-as-404, atomicity story tested via mocks in Task 8).
- §9 out of scope — nothing to do.

**Placeholder scan:** none.

**Type consistency:** `UpdateTopicRequest`/`UpdateTopicParams`, `ListMatchesParams`, `MatchList`, `FeedList`, `RadarStatus` referenced identically across tasks. Store method signatures match `StoreAPI` interface added in Task 6. `Service.ListMatches` takes `ListMatchesParams` (with `UserID` inside) in both Task 9 and Task 11/12 — consistent.

**Implementation order:** Each task's prerequisites are committed before it. Service tasks (7–10) require Task 6 (StoreAPI interface extension). HTTP tasks (11–12) require service tasks. Task 13 (wiring) requires all handlers. Task 14 (integration) requires wiring.
