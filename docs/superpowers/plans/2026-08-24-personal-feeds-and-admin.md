# Personal Feeds and Admin Section Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give users their own feeds in Radar, and admins a separate Admin section that manages the global catalog.

**Architecture:** `radar_feeds` gains `owner_user_id` (`NULL` = global feed). The visibility rule `owner_user_id IS NULL OR owner_user_id = :caller` is threaded through every store query. Write scope is carried by the route namespace: `/radar/feeds` only writes the caller's personal feeds, `/admin/radar/feeds` behind `RequireAdmin` only writes global ones, and the service exposes paired methods (`AddUserFeed`/`AddGlobalFeed` and so on) so a handler cannot pick the wrong scope. On the frontend, `/radar/sources` splits into two sections and Admin becomes a fourth sidebar item behind an `AdminRoute` gate.

**Tech Stack:** Go 1.x (chi, pgx, goose, River, gofeed, testify, testcontainers), React 19 + TypeScript (react-router v7, TanStack Query v5, zod, react-hook-form, Radix UI, Tailwind v4, zustand), tests with vitest + Testing Library + msw.

**Spec:** `docs/superpowers/specs/2026-08-24-personal-feeds-and-admin-design.md`

## Global Constraints

- One visibility rule holds everywhere in the code: `owner_user_id IS NULL OR owner_user_id = :caller`. A row outside the visibility or write scope must look **nonexistent** — `ErrNotFound`/404, never 403.
- Fetch-interval bounds do not change: `defaultFetchIntervalSeconds = 3600`, `minFetchIntervalSeconds = 300`, `maxFetchIntervalSeconds = 86400` (`internal/radar/service.go:113-117`).
- Any new method on `radar.StoreAPI` (`internal/radar/service.go:35`) must also be added to `mockStore` (`internal/radar/service_test.go:18`) — a compile-time `var _ radar.StoreAPI = (*mockStore)(nil)` lives there, and without it the whole test package fails to build.
- Clearing a feed title travels over the wire as an empty string: `{"title": ""}` → `NULL` in the database. An absent field means "do not change".
- Personal-feed quota: env `LINKTHECA_RADAR_MAX_USER_FEEDS`, default `20`.
- `ListDueFeeds` and `MatchFindingToTopics` are **not** touched by this work.
- Go tests: `make test-unit` (fast, `-short`), `make test` (everything, needs Docker for testcontainers). Frontend: `cd web && npm test && npm run typecheck && npm run lint`.
- Every commit carries `Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>`.

---

## File Structure

**Backend, created:**
- `migrations/014_radar_feeds_owner.sql` — the owner column and partial unique indexes.

**Backend, modified:**
- `internal/radar/types.go` — `FeedScope`, `AddFeedResult`, `ErrQuotaExceeded`, and fields on `AddFeedParams` / `ListFeedsParams` / `FeedListItem` / `RadarStatus`.
- `internal/radar/store.go` — ownership in every feed query, plus `GetGlobalFeedByURL` and `CountUserFeeds`.
- `internal/radar/service.go` — paired methods per scope, the quota, `ServiceOption`.
- `internal/radar/http.go` — renamed admin handlers, new user handlers, `quota_exceeded`.
- `internal/core/config/config.go`, `.env.example` — `RadarMaxUserFeeds`.
- `internal/server/server.go` — `/radar/feeds` moves into the user group, new `/admin/radar`.

**Backend, tests:** `internal/radar/store_test.go`, `service_test.go`, `http_test.go`, `integration_test.go`, `internal/core/config/config_test.go`.

**Frontend, created:**
- `web/src/shared/layout/AdminRoute.tsx` (+ `AdminRoute.test.tsx`) — the section gate.
- `web/src/features/admin/api.ts`, `web/src/features/admin/use-admin-feeds.tsx` (+ `use-admin-feeds.test.tsx`) — global catalog data.
- `web/src/routes/admin.sources.tsx` (+ `admin.sources.test.tsx`) — the admin screen.

**Frontend, modified:** `web/src/features/radar/{types.ts,schemas.ts,api.ts,use-mutations.tsx}`, `web/src/features/radar/components/{SourceRow.tsx,AddFeedDialog.tsx,EditFeedDialog.tsx,DeleteFeedConfirm.tsx}`, `web/src/shared/layout/Sidebar.tsx`, `web/src/App.tsx`, `web/src/routes/radar.sources.tsx`, and the tests beside them.

**Documentation:** `docs/superpowers/specs/2026-05-06-user-added-feeds-deferred.md`, `docs/superpowers/specs/2026-08-15-radar-sources-ux-design.md`.

---

### Task 1: The owner column and the write path

**Files:**
- Create: `migrations/014_radar_feeds_owner.sql`
- Modify: `internal/radar/types.go` (`AddFeedParams`, ~line 98)
- Modify: `internal/radar/store.go:75` (`AddFeed`)
- Modify: `internal/radar/service_test.go` (`mockStore.AddFeed`, ~line 116)
- Test: `internal/radar/store_test.go`

**Interfaces:**
- Consumes: the existing `Feed`, `AddFeedParams`, `Store.AddFeed`.
- Produces: `AddFeedParams{URL string; Kind string; FetchIntervalSeconds int; OwnerUserID *int64}`; `Store.AddFeed` writes the owner; in the database, the `radar_feeds.owner_user_id` column and the `radar_feeds_global_url_idx`, `radar_feeds_owner_url_idx`, `radar_feeds_owner_idx` indexes.

- [ ] **Step 1: Write the failing store test**

In `internal/radar/store_test.go`:

```go
func TestStore_AddFeed_OwnershipAndPartialUniqueness(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userA := seedUser(t, pool)
	userB := seedUser(t, pool)
	url := fmt.Sprintf("https://own.example/%d.xml", time.Now().UnixNano())

	// The same URL is fine for two different owners.
	a, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: url, Kind: "rss", FetchIntervalSeconds: 3600, OwnerUserID: &userA,
	})
	require.NoError(t, err)
	b, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: url, Kind: "rss", FetchIntervalSeconds: 3600, OwnerUserID: &userB,
	})
	require.NoError(t, err)
	require.NotEqual(t, a.ID, b.ID)

	// The same owner may not add it twice.
	_, err = store.AddFeed(ctx, radar.AddFeedParams{
		URL: url, Kind: "rss", FetchIntervalSeconds: 3600, OwnerUserID: &userA,
	})
	require.ErrorIs(t, err, radar.ErrDuplicate)

	// A global row with that URL is still allowed…
	_, err = store.AddFeed(ctx, radar.AddFeedParams{
		URL: url, Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)

	// …but only one of it.
	_, err = store.AddFeed(ctx, radar.AddFeedParams{
		URL: url, Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.ErrorIs(t, err, radar.ErrDuplicate)
}
```

Add a cascade test beside it — it checks exactly the `ON DELETE CASCADE` the new
migration introduces:

```go
func TestStore_DeletingUserRemovesTheirFeeds(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	owner := seedUser(t, pool)
	feed, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL:                  fmt.Sprintf("https://cascade.example/%d.xml", time.Now().UnixNano()),
		Kind:                 "rss",
		FetchIntervalSeconds: 3600,
		OwnerUserID:          &owner,
	})
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, owner)
	require.NoError(t, err)

	var left int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM radar_feeds WHERE id = $1`, feed.ID).Scan(&left))
	require.Equal(t, 0, left, "a deleted user's feeds go with them, they are not promoted")
}
```

`seedUser` already exists in `store_test.go` — check its signature; if it takes
only `(t, pool)`, call it twice, since it must return distinct ids.

- [ ] **Step 2: Run it and confirm it does not compile**

Run: `go test ./internal/radar/ -run TestStore_AddFeed_OwnershipAndPartialUniqueness -count=1`
Expected: FAIL — `unknown field OwnerUserID in struct literal`.

- [ ] **Step 3: Write the migration**

`migrations/014_radar_feeds_owner.sql`:

```sql
-- +goose Up
-- owner_user_id NULL means the feed belongs to the shared catalog that admins
-- curate; a user id means it is that account's personal feed, visible only to
-- them. CASCADE, not SET NULL: deleting a user must remove their feeds rather
-- than silently promoting them into everyone's catalog.
ALTER TABLE radar_feeds
  ADD COLUMN owner_user_id BIGINT NULL REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE radar_feeds DROP CONSTRAINT radar_feeds_url_key;

CREATE UNIQUE INDEX radar_feeds_global_url_idx
  ON radar_feeds (url) WHERE owner_user_id IS NULL;
CREATE UNIQUE INDEX radar_feeds_owner_url_idx
  ON radar_feeds (url, owner_user_id) WHERE owner_user_id IS NOT NULL;
CREATE INDEX radar_feeds_owner_idx
  ON radar_feeds (owner_user_id) WHERE owner_user_id IS NOT NULL;

-- +goose Down
-- Fails if personal feeds share a URL with each other or with a catalog feed:
-- the restored constraint is stricter than the partial indexes it replaces.
DROP INDEX radar_feeds_owner_idx;
DROP INDEX radar_feeds_owner_url_idx;
DROP INDEX radar_feeds_global_url_idx;
ALTER TABLE radar_feeds ADD CONSTRAINT radar_feeds_url_key UNIQUE (url);
ALTER TABLE radar_feeds DROP COLUMN owner_user_id;
```

- [ ] **Step 4: Add the field to `AddFeedParams`**

In `internal/radar/types.go`, replace the `AddFeedParams` block with:

```go
type AddFeedParams struct {
	URL                  string
	Kind                 string
	FetchIntervalSeconds int
	// OwnerUserID nil creates a shared catalog feed; a user id creates that
	// account's personal feed.
	OwnerUserID *int64
}
```

- [ ] **Step 5: Write the owner in `Store.AddFeed`**

In `internal/radar/store.go`, replace the query inside `AddFeed`:

```go
	row := s.db.QueryRow(ctx, `
		INSERT INTO radar_feeds (url, kind, fetch_interval_seconds, owner_user_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id, url, kind, title, fetch_interval_seconds, is_active,
		          last_fetched_at, last_error, created_at
	`, p.URL, p.Kind, p.FetchIntervalSeconds, p.OwnerUserID)
```

- [ ] **Step 6: Fix `mockStore.AddFeed`**

In `internal/radar/service_test.go` the mock deduplicates by bare URL — the key
is composite now. Replace the method body with:

```go
func (m *mockStore) AddFeed(_ context.Context, p radar.AddFeedParams) (*radar.Feed, error) {
	if m.addFeedErr != nil {
		return nil, m.addFeedErr
	}
	key := feedKey(p.URL, p.OwnerUserID)
	if _, ok := m.feedsByURL[key]; ok {
		return nil, radar.ErrDuplicate
	}
	m.nextFeedID++
	f := &radar.Feed{
		ID: m.nextFeedID, URL: p.URL, Kind: p.Kind,
		FetchIntervalSeconds: p.FetchIntervalSeconds, IsActive: true,
		CreatedAt: time.Now(),
	}
	m.feeds[f.ID] = f
	m.feedsByURL[key] = f
	m.feedOwners[f.ID] = p.OwnerUserID
	return f, nil
}

// feedKey mirrors the partial unique indexes: one global row per URL, one
// personal row per (URL, owner).
func feedKey(url string, owner *int64) string {
	if owner == nil {
		return "global:" + url
	}
	return fmt.Sprintf("user:%d:%s", *owner, url)
}
```

Add a `feedOwners map[int64]*int64` field to the `mockStore` struct and
initialize it in the constructor (`~line 79`) next to `feeds`/`feedsByURL`. Add
the `"fmt"` import if it is not there yet. The existing `DeleteFeed`
(`~line 764`) clears `m.feedsByURL[f.URL]` — replace that with
`delete(m.feedsByURL, feedKey(f.URL, m.feedOwners[feedID]))` and
`delete(m.feedOwners, feedID)`.

- [ ] **Step 7: Run the tests**

Run: `make test-unit && go test ./internal/radar/ -run 'TestStore_AddFeed|TestStore_DeletingUser' -count=1`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add migrations internal/radar
git commit -m "feat(radar): add an owner column to feeds

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 2: Visibility when reading the catalog

**Files:**
- Modify: `internal/radar/types.go` (`ListFeedsParams` ~line 169, `FeedListItem` ~line 177)
- Modify: `internal/radar/store.go:572` (`ListFeeds`)
- Modify: `internal/radar/service.go:298` (`ListFeeds`)
- Modify: `internal/radar/http.go:305` (`listFeeds`)
- Test: `internal/radar/store_test.go`, `internal/radar/http_test.go`

**Interfaces:**
- Consumes: `Store.AddFeed` with an owner (Task 1).
- Produces: `radar.FeedScope` with the constants `FeedScopeVisible` and `FeedScopeGlobal`; `ListFeedsParams{Limit, Offset int; Scope FeedScope}`; `FeedListItem` with an `IsOwn bool` field (json `is_own`). The `Store.ListFeeds` and `Service.ListFeeds` signatures do not change.

- [ ] **Step 1: Write the failing store test**

In `internal/radar/store_test.go`:

```go
func TestStore_ListFeeds_Visibility(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userA := seedUser(t, pool)
	userB := seedUser(t, pool)
	stamp := time.Now().UnixNano()

	global, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: fmt.Sprintf("https://vis-g.example/%d.xml", stamp), Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)
	mine, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: fmt.Sprintf("https://vis-a.example/%d.xml", stamp), Kind: "rss", FetchIntervalSeconds: 3600,
		OwnerUserID: &userA,
	})
	require.NoError(t, err)
	theirs, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: fmt.Sprintf("https://vis-b.example/%d.xml", stamp), Kind: "rss", FetchIntervalSeconds: 3600,
		OwnerUserID: &userB,
	})
	require.NoError(t, err)

	items, total, err := store.ListFeeds(ctx, userA, radar.ListFeedsParams{
		Limit: 100, Scope: radar.FeedScopeVisible,
	})
	require.NoError(t, err)

	byID := map[int64]radar.FeedListItem{}
	for _, it := range items {
		byID[it.ID] = it
	}
	require.Contains(t, byID, global.ID)
	require.Contains(t, byID, mine.ID)
	require.NotContains(t, byID, theirs.ID, "another account's personal feed must be invisible")
	require.Equal(t, len(items), total, "total must be scoped like the page")

	require.True(t, byID[mine.ID].IsOwn)
	require.False(t, byID[global.ID].IsOwn)

	// The admin catalog view hides personal feeds entirely.
	adminItems, _, err := store.ListFeeds(ctx, userA, radar.ListFeedsParams{
		Limit: 100, Scope: radar.FeedScopeGlobal,
	})
	require.NoError(t, err)
	for _, it := range adminItems {
		require.False(t, it.IsOwn)
		require.NotEqual(t, mine.ID, it.ID)
	}
}
```

The test compares `total` with the page length, which holds because `limit` is
well above the number of feeds in the test schema (every test gets its own
database schema).

- [ ] **Step 2: Run it and confirm it fails**

Run: `go test ./internal/radar/ -run TestStore_ListFeeds_Visibility -count=1`
Expected: FAIL — `undefined: radar.FeedScopeVisible`, `it.IsOwn undefined`.

- [ ] **Step 3: Types in `types.go`**

Replace the `ListFeedsParams` and `FeedListItem` blocks with:

```go
// FeedScope narrows which catalog rows a read may see.
type FeedScope string

const (
	// FeedScopeVisible is what a user may see: the shared catalog plus their
	// own personal feeds.
	FeedScopeVisible FeedScope = "visible"
	// FeedScopeGlobal is the shared catalog alone, for the admin screen.
	FeedScopeGlobal FeedScope = "global"
)

// ListFeedsParams holds query parameters for the feed catalog.
type ListFeedsParams struct {
	Limit  int
	Offset int
	Scope  FeedScope
}

// FeedListItem is one catalog row: the feed plus per-user subscription state,
// how many findings it has produced, and whether the caller owns it.
type FeedListItem struct {
	Feed
	Subscribed   bool `json:"subscribed"`
	FindingCount int  `json:"finding_count"`
	IsOwn        bool `json:"is_own"`
}
```

- [ ] **Step 4: The filter in `Store.ListFeeds`**

Replace the method body with:

```go
func (s *Store) ListFeeds(ctx context.Context, userID int64, p ListFeedsParams) ([]FeedListItem, int, error) {
	// The predicate is shared by the count and the page so the two agree.
	where := `(f.owner_user_id IS NULL OR f.owner_user_id = $1)`
	if p.Scope == FeedScopeGlobal {
		where = `f.owner_user_id IS NULL`
	}

	var total int
	if err := s.db.QueryRow(ctx,
		`SELECT count(*) FROM radar_feeds f WHERE `+where, userID).Scan(&total); err != nil {
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
```

`is_own` is computed as `owner_user_id IS NOT NULL` rather than by comparing
against `$1`: `where` has already excluded other people's personal feeds, so any
remaining personal row is the caller's. Under `FeedScopeGlobal` there are no
personal rows at all and the flag is always `false`.

- [ ] **Step 5: Normalize the scope in the service**

In `internal/radar/service.go`, inside `ListFeeds`, before calling the store:

```go
	if p.Scope == "" {
		p.Scope = FeedScopeVisible
	}
```

- [ ] **Step 6: The handler sets the user scope**

In `internal/radar/http.go`, inside `listFeeds`, replace the initialization:

```go
	params := ListFeedsParams{Scope: FeedScopeVisible}
```

While here, drop the stale "(admin)" from the `ListFeedsParams` comment in
`types.go` — already done in Step 3 — and from the comment above `listFeeds` if
it carries one.

- [ ] **Step 7: Handler test for `is_own` in the JSON**

In `internal/radar/http_test.go`:

```go
func TestHTTP_ListFeeds_ExposesIsOwn(t *testing.T) {
	store := newMockStore()
	store.listFeedsResult = []radar.FeedListItem{{
		Feed:       radar.Feed{ID: 7, URL: "https://x.example/rss", Kind: "rss"},
		Subscribed: true, FindingCount: 12, IsOwn: true,
	}}
	store.listFeedsTotal = 1
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodGet, "/radar/feeds?limit=10", nil)
	req = req.WithContext(userOnlyContext(req.Context(), 42, false))
	rec := httptest.NewRecorder()
	h.ListFeedsHandler()(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, radar.FeedScopeVisible, store.listFeedsParams.Scope)
	require.Contains(t, rec.Body.String(), `"is_own":true`)
}
```

- [ ] **Step 8: Run the tests**

Run: `make test-unit && go test ./internal/radar/ -run 'ListFeeds' -count=1`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/radar
git commit -m "feat(radar): scope the feed catalog to what the caller may see

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 3: Visibility in subscriptions

**Files:**
- Modify: `internal/radar/store.go:93` (`Subscribe`), `internal/radar/store.go:663` (`SeedSubscriptions`)
- Test: `internal/radar/store_test.go`

**Interfaces:**
- Consumes: `Store.AddFeed` with an owner (Task 1), `Store.ListFeeds` with `Scope` (Task 2).
- Produces: no signature changes. `Store.Subscribe` returns `ErrFeedNotFound` for an invisible feed; `Store.SeedSubscriptions` subscribes only to active global feeds.

- [ ] **Step 1: Write the failing tests**

In `internal/radar/store_test.go`:

```go
func TestStore_Subscribe_RejectsForeignPersonalFeed(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userA := seedUser(t, pool)
	userB := seedUser(t, pool)
	stamp := time.Now().UnixNano()

	theirs, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: fmt.Sprintf("https://sub-b.example/%d.xml", stamp), Kind: "rss",
		FetchIntervalSeconds: 3600, OwnerUserID: &userB,
	})
	require.NoError(t, err)

	_, err = store.Subscribe(ctx, userA, theirs.ID)
	require.ErrorIs(t, err, radar.ErrFeedNotFound)

	// The owner still can, and it stays idempotent.
	_, err = store.Subscribe(ctx, userB, theirs.ID)
	require.NoError(t, err)
	_, err = store.Subscribe(ctx, userB, theirs.ID)
	require.NoError(t, err)
}

func TestStore_SeedSubscriptions_GlobalOnly(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	owner := seedUser(t, pool)
	stamp := time.Now().UnixNano()

	global, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: fmt.Sprintf("https://seed-g.example/%d.xml", stamp), Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)
	personal, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: fmt.Sprintf("https://seed-p.example/%d.xml", stamp), Kind: "rss",
		FetchIntervalSeconds: 3600, OwnerUserID: &owner,
	})
	require.NoError(t, err)

	newcomer := seedUser(t, pool)
	n, err := store.SeedSubscriptions(ctx, newcomer)
	require.NoError(t, err)
	require.GreaterOrEqual(t, n, 1)

	items, _, err := store.ListFeeds(ctx, newcomer, radar.ListFeedsParams{
		Limit: 100, Scope: radar.FeedScopeVisible,
	})
	require.NoError(t, err)
	byID := map[int64]radar.FeedListItem{}
	for _, it := range items {
		byID[it.ID] = it
	}
	require.True(t, byID[global.ID].Subscribed)
	require.NotContains(t, byID, personal.ID, "someone else's personal feed is not seeded")
}
```

- [ ] **Step 2: Run them and confirm they fail**

Run: `go test ./internal/radar/ -run 'TestStore_Subscribe_RejectsForeignPersonalFeed|TestStore_SeedSubscriptions_GlobalOnly' -count=1`
Expected: FAIL — subscribing to a foreign feed succeeds instead of returning `ErrFeedNotFound`.

- [ ] **Step 3: Rewrite `Store.Subscribe`**

```go
// Subscribe adds the user's subscription. The visibility predicate lives in the
// statement itself, so guessing a feed id cannot subscribe anyone to another
// account's personal feed: no row matches and the caller sees ErrFeedNotFound.
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
```

Here `pgx.ErrNoRows` means "the feed is invisible or absent", so it is mapped to
`ErrFeedNotFound` before `wrapPgError`, which would have turned it into
`ErrNotFound`.

- [ ] **Step 4: Restrict `SeedSubscriptions` to global feeds**

```go
	cmd, err := s.db.Exec(ctx, `
		INSERT INTO radar_feed_subscriptions (user_id, feed_id)
		SELECT $1, id FROM radar_feeds WHERE is_active AND owner_user_id IS NULL
		ON CONFLICT DO NOTHING`, userID)
```

- [ ] **Step 5: Run the tests**

Run: `make test-unit && go test ./internal/radar/ -run 'Subscribe|SeedSubscriptions' -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/radar
git commit -m "feat(radar): keep subscriptions inside the caller's visibility

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 4: Write scope in the store

**Files:**
- Modify: `internal/radar/store.go:616` (`UpdateFeed`), `internal/radar/store.go:677` (`DeleteFeed`)
- Modify: `internal/radar/service.go` (`StoreAPI` ~lines 53-54, `UpdateFeed` ~line 333, `DeleteFeed` ~line 361)
- Modify: `internal/radar/service_test.go` (`mockStore.UpdateFeed` ~line 731, `mockStore.DeleteFeed` ~line 757)
- Test: `internal/radar/store_test.go`

**Interfaces:**
- Consumes: `Store.AddFeed` with an owner (Task 1).
- Produces: `Store.UpdateFeed(ctx, feedID int64, owner *int64, p UpdateFeedParams) (*Feed, error)`; `Store.DeleteFeed(ctx, feedID int64, owner *int64) error`. `owner == nil` addresses global rows, a value addresses that user's personal feeds. The `Service.UpdateFeed`/`DeleteFeed` signatures do **not** change in this task — they pass `nil` for now and keep today's admin behaviour; Task 5 replaces them with the pairs.

- [ ] **Step 1: Write the failing store test**

In `internal/radar/store_test.go`:

```go
func TestStore_UpdateDeleteFeed_ScopedToOwner(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userA := seedUser(t, pool)
	userB := seedUser(t, pool)
	stamp := time.Now().UnixNano()

	global, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: fmt.Sprintf("https://scope-g.example/%d.xml", stamp), Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)
	mine, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: fmt.Sprintf("https://scope-a.example/%d.xml", stamp), Kind: "rss",
		FetchIntervalSeconds: 3600, OwnerUserID: &userA,
	})
	require.NoError(t, err)

	title := "Renamed"

	// A user may patch their own feed…
	updated, err := store.UpdateFeed(ctx, mine.ID, &userA, radar.UpdateFeedParams{Title: &title})
	require.NoError(t, err)
	require.Equal(t, "Renamed", *updated.Title)

	// …but not a catalog feed, and not someone else's.
	_, err = store.UpdateFeed(ctx, global.ID, &userA, radar.UpdateFeedParams{Title: &title})
	require.ErrorIs(t, err, radar.ErrNotFound)
	_, err = store.UpdateFeed(ctx, mine.ID, &userB, radar.UpdateFeedParams{Title: &title})
	require.ErrorIs(t, err, radar.ErrNotFound)

	// The admin scope reaches catalog rows only.
	_, err = store.UpdateFeed(ctx, global.ID, nil, radar.UpdateFeedParams{Title: &title})
	require.NoError(t, err)
	_, err = store.UpdateFeed(ctx, mine.ID, nil, radar.UpdateFeedParams{Title: &title})
	require.ErrorIs(t, err, radar.ErrNotFound)

	// Same rules for deletion.
	require.ErrorIs(t, store.DeleteFeed(ctx, mine.ID, nil), radar.ErrNotFound)
	require.ErrorIs(t, store.DeleteFeed(ctx, global.ID, &userA), radar.ErrNotFound)
	require.NoError(t, store.DeleteFeed(ctx, mine.ID, &userA))
	require.NoError(t, store.DeleteFeed(ctx, global.ID, nil))
}
```

- [ ] **Step 2: Run it and confirm it fails**

Run: `go test ./internal/radar/ -run TestStore_UpdateDeleteFeed_ScopedToOwner -count=1`
Expected: FAIL — `too many arguments in call to store.UpdateFeed`.

- [ ] **Step 3: Scope in `Store.UpdateFeed`**

Change the signature and add the scope predicate to the `WHERE`:

```go
// UpdateFeed applies a partial patch inside one ownership scope: owner nil
// addresses catalog rows, a user id addresses that account's personal feeds.
// A row outside the scope is indistinguishable from a missing one.
func (s *Store) UpdateFeed(ctx context.Context, feedID int64, owner *int64, p UpdateFeedParams) (*Feed, error) {
```

Inside, after building `setClauses` and before assembling the query, replace the
argument block with:

```go
	args = append(args, feedID, owner)
	query := fmt.Sprintf(`
		UPDATE radar_feeds SET %s
		WHERE id = $%d AND owner_user_id IS NOT DISTINCT FROM $%d
		RETURNING id, url, kind, title, fetch_interval_seconds, is_active,
		          last_fetched_at, last_error, created_at`,
		strings.Join(setClauses, ", "), argIdx, argIdx+1)
```

`IS NOT DISTINCT FROM` is the only form where `NULL = NULL` holds, so one
predicate covers both scopes.

- [ ] **Step 4: Scope in `Store.DeleteFeed`**

```go
// DeleteFeed removes a feed inside one ownership scope. Findings and their
// matches go with it via ON DELETE CASCADE.
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
```

- [ ] **Step 5: Thread the new signature through `StoreAPI` and the service**

In `StoreAPI` (`internal/radar/service.go`), replace the two lines with:

```go
	UpdateFeed(ctx context.Context, feedID int64, owner *int64, p UpdateFeedParams) (*Feed, error)
	DeleteFeed(ctx context.Context, feedID int64, owner *int64) error
```

In `Service.UpdateFeed` and `Service.DeleteFeed`, pass `nil` for now — the
behaviour stays today's admin behaviour, and the pairs arrive in Task 5:

```go
	return s.store.UpdateFeed(ctx, feedID, nil, UpdateFeedParams{...})
```
```go
	return s.store.DeleteFeed(ctx, feedID, nil)
```

- [ ] **Step 6: Update `mockStore`**

```go
func (m *mockStore) UpdateFeed(_ context.Context, feedID int64, owner *int64, p radar.UpdateFeedParams) (*radar.Feed, error) {
	m.updateFeedCalled = true
	m.updateFeedParams = p
	m.updateFeedOwner = owner
	if m.updateFeedErr != nil {
		return nil, m.updateFeedErr
	}
	f, ok := m.feeds[feedID]
	if !ok {
		return &radar.Feed{ID: feedID}, nil
	}
	if p.Title != nil {
		f.Title = p.Title
	}
	if p.FetchIntervalSeconds != nil {
		f.FetchIntervalSeconds = *p.FetchIntervalSeconds
	}
	if p.IsActive != nil {
		f.IsActive = *p.IsActive
	}
	return f, nil
}

func (m *mockStore) DeleteFeed(_ context.Context, feedID int64, owner *int64) error {
	m.deleteFeedCalled = true
	m.deleteFeedOwner = owner
	if m.deleteFeedErr != nil {
		return m.deleteFeedErr
	}
	f, ok := m.feeds[feedID]
	if !ok {
		return radar.ErrNotFound
	}
	delete(m.feedsByURL, feedKey(f.URL, m.feedOwners[feedID]))
	delete(m.feedOwners, feedID)
	delete(m.feeds, feedID)
	return nil
}
```

Add `updateFeedOwner *int64` and `deleteFeedOwner *int64` fields to the
`mockStore` struct.

- [ ] **Step 7: Run the tests**

Run: `make test-unit && go test ./internal/radar/ -run 'UpdateFeed|DeleteFeed' -count=1`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/radar
git commit -m "feat(radar): scope feed writes to an owner

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 5: Paired service methods and the quota

**Files:**
- Modify: `internal/radar/types.go` (sentinels ~line 11, `AddFeedResult`)
- Modify: `internal/radar/store.go` (new `GetGlobalFeedByURL`, `CountUserFeeds`)
- Modify: `internal/radar/service.go` (`StoreAPI`, `NewService`, the feed methods)
- Modify: `internal/radar/service_test.go` (`mockStore`)
- Test: `internal/radar/store_test.go`, `internal/radar/service_test.go`

**Interfaces:**
- Consumes: `Store.AddFeed` (Task 1), `Store.Subscribe` (Task 3), `Store.UpdateFeed`/`DeleteFeed` with `owner` (Task 4).
- Produces: `radar.ErrQuotaExceeded`; `AddFeedResult{Feed *Feed \`json:"feed"\`; Created bool \`json:"created"\`}`; `Store.GetGlobalFeedByURL(ctx, url string) (*Feed, error)`; `Store.CountUserFeeds(ctx, userID int64) (int, error)`; `ServiceOption` and `WithMaxUserFeeds(n int) ServiceOption`; `NewService(store StoreAPI, embedder embeddings.Client, opts ...ServiceOption) *Service`; the methods `Service.AddUserFeed(ctx, userID int64, req AddFeedRequest) (*AddFeedResult, error)`, `Service.AddGlobalFeed(ctx, req AddFeedRequest) (*Feed, error)`, `Service.UpdateUserFeed(ctx, userID, feedID int64, req UpdateFeedRequest) (*Feed, error)`, `Service.UpdateGlobalFeed(ctx, feedID int64, req UpdateFeedRequest) (*Feed, error)`, `Service.DeleteUserFeed(ctx, userID, feedID int64) error`, `Service.DeleteGlobalFeed(ctx, feedID int64) error`. The old `Service.AddFeed`/`UpdateFeed`/`DeleteFeed` are removed.

- [ ] **Step 1: Write the failing service tests**

In `internal/radar/service_test.go`:

```go
func TestService_AddUserFeed_ReusesCatalogFeed(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	ctx := context.Background()

	// A catalog feed already covers this URL.
	catalog, err := svc.AddGlobalFeed(ctx, radar.AddFeedRequest{URL: "https://shared.example/rss"})
	require.NoError(t, err)

	res, err := svc.AddUserFeed(ctx, 42, radar.AddFeedRequest{URL: "https://shared.example/rss"})
	require.NoError(t, err)
	require.False(t, res.Created, "no personal row is created for a catalog URL")
	require.Equal(t, catalog.ID, res.Feed.ID)
	require.True(t, store.hasSubscription(42, catalog.ID))
}

func TestService_AddUserFeed_CreatesAndSubscribes(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	res, err := svc.AddUserFeed(context.Background(), 42,
		radar.AddFeedRequest{URL: "https://mine.example/rss"})
	require.NoError(t, err)
	require.True(t, res.Created)
	require.True(t, store.hasSubscription(42, res.Feed.ID))
}

func TestService_AddUserFeed_QuotaExceeded(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024},
		radar.WithMaxUserFeeds(1))
	ctx := context.Background()

	_, err := svc.AddUserFeed(ctx, 42, radar.AddFeedRequest{URL: "https://a.example/rss"})
	require.NoError(t, err)

	_, err = svc.AddUserFeed(ctx, 42, radar.AddFeedRequest{URL: "https://b.example/rss"})
	require.ErrorIs(t, err, radar.ErrQuotaExceeded)
}

func TestService_UserFeedWritesCarryOwner(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	ctx := context.Background()

	title := "Renamed"
	_, err := svc.UpdateUserFeed(ctx, 42, 1, radar.UpdateFeedRequest{Title: &title})
	require.NoError(t, err)
	require.NotNil(t, store.updateFeedOwner)
	require.Equal(t, int64(42), *store.updateFeedOwner)

	_, err = svc.UpdateGlobalFeed(ctx, 1, radar.UpdateFeedRequest{Title: &title})
	require.NoError(t, err)
	require.Nil(t, store.updateFeedOwner, "the admin scope addresses catalog rows")
}
```

Add a helper to `mockStore`:

```go
func (m *mockStore) hasSubscription(userID, feedID int64) bool {
	_, ok := m.subs[keyOf(userID, feedID)]
	return ok
}
```

- [ ] **Step 2: Run them and confirm they fail**

Run: `go test ./internal/radar/ -run 'TestService_AddUserFeed|TestService_UserFeedWrites' -count=1`
Expected: FAIL — `svc.AddGlobalFeed undefined`, `radar.WithMaxUserFeeds undefined`.

- [ ] **Step 3: Sentinel and DTO in `types.go`**

Add to the `var (...)` block:

```go
	ErrQuotaExceeded       = errors.New("quota exceeded")
```

Next to `AddFeedRequest`, add:

```go
// AddFeedResult reports whether a row was created or an existing catalog feed
// was reused, in which case the caller was only subscribed to it. The flag
// travels in the body because the API client does not surface status codes.
type AddFeedResult struct {
	Feed    *Feed `json:"feed"`
	Created bool  `json:"created"`
}
```

- [ ] **Step 4: New store methods**

In `internal/radar/store.go`:

```go
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
```

- [ ] **Step 5: Store test for the new methods**

In `internal/radar/store_test.go`:

```go
func TestStore_GetGlobalFeedByURL_And_CountUserFeeds(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	owner := seedUser(t, pool)
	stamp := time.Now().UnixNano()
	personalURL := fmt.Sprintf("https://count-p.example/%d.xml", stamp)
	globalURL := fmt.Sprintf("https://count-g.example/%d.xml", stamp)

	_, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: personalURL, Kind: "rss", FetchIntervalSeconds: 3600, OwnerUserID: &owner,
	})
	require.NoError(t, err)

	// A personal-only URL is not a catalog feed.
	_, err = store.GetGlobalFeedByURL(ctx, personalURL)
	require.ErrorIs(t, err, radar.ErrNotFound)

	global, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: globalURL, Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)
	found, err := store.GetGlobalFeedByURL(ctx, globalURL)
	require.NoError(t, err)
	require.Equal(t, global.ID, found.ID)

	n, err := store.CountUserFeeds(ctx, owner)
	require.NoError(t, err)
	require.Equal(t, 1, n, "catalog feeds do not count against a user's quota")
}
```

- [ ] **Step 6: The option and the quota on `Service`**

In `internal/radar/service.go`, extend `StoreAPI` with two lines:

```go
	GetGlobalFeedByURL(ctx context.Context, url string) (*Feed, error)
	CountUserFeeds(ctx context.Context, userID int64) (int, error)
```

Replace the `Service` declaration and constructor:

```go
// defaultMaxUserFeeds caps personal feeds per account. An open write endpoint
// with no ceiling is a DoS vector for the crawler and for TEI.
const defaultMaxUserFeeds = 20

type Service struct {
	store        StoreAPI
	embedder     embeddings.Client
	maxUserFeeds int
}

// ServiceOption tunes optional Service behaviour.
type ServiceOption func(*Service)

// WithMaxUserFeeds caps how many personal feeds one account may own. Values
// below 1 are ignored so a missing config cannot lock everyone out.
func WithMaxUserFeeds(n int) ServiceOption {
	return func(s *Service) {
		if n > 0 {
			s.maxUserFeeds = n
		}
	}
}

func NewService(store StoreAPI, embedder embeddings.Client, opts ...ServiceOption) *Service {
	s := &Service{store: store, embedder: embedder, maxUserFeeds: defaultMaxUserFeeds}
	for _, opt := range opts {
		opt(s)
	}
	return s
}
```

- [ ] **Step 7: Replace `AddFeed` with the pair**

Delete `Service.AddFeed` (`~line 119`) and write in its place:

```go
// validateAddFeed normalises and checks the shared part of both add paths.
func validateAddFeed(req AddFeedRequest) (url, kind string, interval int, err error) {
	url = strings.TrimSpace(req.URL)

	if url == "" || len(url) > 2000 {
		return "", "", 0, fmt.Errorf("%w: url must be 1..2000 chars", ErrInvalidInput)
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "", "", 0, fmt.Errorf("%w: url must be http(s)", ErrInvalidInput)
	}

	kind = "rss"
	if req.Kind != nil {
		kind = *req.Kind
		if kind != "rss" && kind != "atom" {
			return "", "", 0, fmt.Errorf("%w: kind must be rss|atom", ErrInvalidInput)
		}
	}

	interval = defaultFetchIntervalSeconds
	if req.FetchIntervalSeconds != nil {
		interval = *req.FetchIntervalSeconds
		if err := validateFetchInterval(interval); err != nil {
			return "", "", 0, err
		}
	}

	return url, kind, interval, nil
}

// AddGlobalFeed creates a catalog feed everyone may subscribe to. Admin scope;
// the /admin/radar route enforces it. Existing users are not auto-subscribed.
func (s *Service) AddGlobalFeed(ctx context.Context, req AddFeedRequest) (*Feed, error) {
	url, kind, interval, err := validateAddFeed(req)
	if err != nil {
		return nil, err
	}

	return s.store.AddFeed(ctx, AddFeedParams{
		URL: url, Kind: kind, FetchIntervalSeconds: interval,
	})
}

// AddUserFeed creates a personal feed for the caller and subscribes them to it.
// If the URL is already in the shared catalog no row is created: the caller is
// simply subscribed to the catalog feed, so a popular feed is not duplicated
// once per account.
func (s *Service) AddUserFeed(ctx context.Context, userID int64, req AddFeedRequest) (*AddFeedResult, error) {
	url, kind, interval, err := validateAddFeed(req)
	if err != nil {
		return nil, err
	}

	switch existing, err := s.store.GetGlobalFeedByURL(ctx, url); {
	case err == nil:
		if _, err := s.store.Subscribe(ctx, userID, existing.ID); err != nil {
			return nil, err
		}
		return &AddFeedResult{Feed: existing, Created: false}, nil
	case !errors.Is(err, ErrNotFound):
		return nil, err
	}

	// Best-effort ceiling: two concurrent adds can land one feed over the
	// limit. A transaction here would buy nothing worth its cost.
	n, err := s.store.CountUserFeeds(ctx, userID)
	if err != nil {
		return nil, err
	}
	if n >= s.maxUserFeeds {
		return nil, fmt.Errorf("%w: at most %d personal feeds", ErrQuotaExceeded, s.maxUserFeeds)
	}

	feed, err := s.store.AddFeed(ctx, AddFeedParams{
		URL: url, Kind: kind, FetchIntervalSeconds: interval, OwnerUserID: &userID,
	})
	if err != nil {
		return nil, err
	}

	if _, err := s.store.Subscribe(ctx, userID, feed.ID); err != nil {
		return nil, err
	}

	return &AddFeedResult{Feed: feed, Created: true}, nil
}
```

- [ ] **Step 8: Replace `UpdateFeed`/`DeleteFeed` with pairs**

Delete `Service.UpdateFeed` and `Service.DeleteFeed` and write:

```go
// updateFeed patches one feed inside a single ownership scope. Callers pick the
// scope by choosing UpdateUserFeed or UpdateGlobalFeed, so a handler cannot
// reach outside the namespace it is mounted on.
func (s *Service) updateFeed(ctx context.Context, owner *int64, feedID int64, req UpdateFeedRequest) (*Feed, error) {
	if feedID <= 0 {
		return nil, fmt.Errorf("%w: feed id must be positive", ErrInvalidInput)
	}
	if req.Title == nil && req.FetchIntervalSeconds == nil && req.IsActive == nil {
		return nil, fmt.Errorf("%w: no fields to update", ErrInvalidInput)
	}
	if req.FetchIntervalSeconds != nil {
		if err := validateFetchInterval(*req.FetchIntervalSeconds); err != nil {
			return nil, err
		}
	}
	if req.Title != nil {
		trimmed := strings.TrimSpace(*req.Title)
		if len(trimmed) > 500 {
			return nil, fmt.Errorf("%w: title must be at most 500 chars", ErrInvalidInput)
		}
		req.Title = &trimmed
	}

	return s.store.UpdateFeed(ctx, feedID, owner, UpdateFeedParams{
		Title:                req.Title,
		FetchIntervalSeconds: req.FetchIntervalSeconds,
		IsActive:             req.IsActive,
	})
}

// UpdateUserFeed patches one of the caller's own personal feeds.
func (s *Service) UpdateUserFeed(ctx context.Context, userID, feedID int64, req UpdateFeedRequest) (*Feed, error) {
	return s.updateFeed(ctx, &userID, feedID, req)
}

// UpdateGlobalFeed patches a catalog feed. Admin scope.
func (s *Service) UpdateGlobalFeed(ctx context.Context, feedID int64, req UpdateFeedRequest) (*Feed, error) {
	return s.updateFeed(ctx, nil, feedID, req)
}

func (s *Service) deleteFeed(ctx context.Context, owner *int64, feedID int64) error {
	if feedID <= 0 {
		return fmt.Errorf("%w: feed id must be positive", ErrInvalidInput)
	}
	return s.store.DeleteFeed(ctx, feedID, owner)
}

// DeleteUserFeed removes one of the caller's own personal feeds.
func (s *Service) DeleteUserFeed(ctx context.Context, userID, feedID int64) error {
	return s.deleteFeed(ctx, &userID, feedID)
}

// DeleteGlobalFeed removes a catalog feed for everyone. Admin scope.
func (s *Service) DeleteGlobalFeed(ctx context.Context, feedID int64) error {
	return s.deleteFeed(ctx, nil, feedID)
}
```

- [ ] **Step 9: Finish `mockStore`**

```go
func (m *mockStore) GetGlobalFeedByURL(_ context.Context, url string) (*radar.Feed, error) {
	if f, ok := m.feedsByURL[feedKey(url, nil)]; ok {
		return f, nil
	}
	return nil, radar.ErrNotFound
}

func (m *mockStore) CountUserFeeds(_ context.Context, userID int64) (int, error) {
	n := 0
	for _, owner := range m.feedOwners {
		if owner != nil && *owner == userID {
			n++
		}
	}
	return n, nil
}
```

Rewrite the existing tests that call `svc.AddFeed` / `svc.UpdateFeed` /
`svc.DeleteFeed` (in `service_test.go`, `http_test.go`, `integration_test.go`)
to the new names: admin call sites become `*Global*`, user ones `*User*`.
`TestService_AddFeed_Validation` (`service_test.go:255`) becomes
`TestService_AddGlobalFeed_Validation`.

- [ ] **Step 10: Run the tests**

Run: `make test-unit && go test ./internal/radar/ -count=1`
Expected: PASS.

- [ ] **Step 11: Commit**

```bash
git add internal/radar
git commit -m "feat(radar): add personal feeds with a per-account quota

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 6: HTTP handlers, config, and routes

**Files:**
- Modify: `internal/radar/http.go` (`writeRadarError` :71, `AddFeedHandler` :88, `updateFeed` :353, `deleteFeed` :380, `status` ~:300)
- Modify: `internal/radar/types.go` (`RadarStatus` ~line 234)
- Modify: `internal/core/config/config.go:33-35`, `.env.example`
- Modify: `internal/server/server.go:130-175`
- Test: `internal/radar/http_test.go`, `internal/radar/integration_test.go`, `internal/core/config/config_test.go`

**Interfaces:**
- Consumes: the service methods from Task 5.
- Produces: `HTTP.AddUserFeedHandler()`, `HTTP.UpdateUserFeedHandler()`, `HTTP.DeleteUserFeedHandler()`, `HTTP.ListGlobalFeedsHandler()`, `HTTP.AddGlobalFeedHandler()`, `HTTP.UpdateGlobalFeedHandler()`, `HTTP.DeleteGlobalFeedHandler()`; `RadarStatus{LastSweepAt *time.Time; MaxUserFeeds int}`; `config.Config.RadarMaxUserFeeds`; the `/radar/feeds` (user) and `/admin/radar/feeds` (admin) routes.

- [ ] **Step 1: Write the failing handler tests**

In `internal/radar/http_test.go`:

```go
func TestHTTP_AddUserFeed_201AndQuota409(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024},
		radar.WithMaxUserFeeds(1))
	h := radar.NewHTTP(svc)

	post := func(url string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(radar.AddFeedRequest{URL: url})
		req := httptest.NewRequest(http.MethodPost, "/radar/feeds", bytes.NewReader(body))
		req = req.WithContext(userOnlyContext(req.Context(), 42, false))
		rec := httptest.NewRecorder()
		h.AddUserFeedHandler()(rec, req)
		return rec
	}

	first := post("https://a.example/rss")
	require.Equal(t, http.StatusCreated, first.Code)
	require.Contains(t, first.Body.String(), `"created":true`)

	second := post("https://b.example/rss")
	require.Equal(t, http.StatusConflict, second.Code)
	require.Contains(t, second.Body.String(), `"quota_exceeded"`)
}

func TestHTTP_UpdateUserFeed_PassesCallerID(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodPatch, "/radar/feeds/5",
		strings.NewReader(`{"is_active":false}`))
	req = req.WithContext(withRouteID(userOnlyContext(req.Context(), 42, false), "5"))
	rec := httptest.NewRecorder()
	h.UpdateUserFeedHandler()(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, store.updateFeedOwner)
	require.Equal(t, int64(42), *store.updateFeedOwner)
}
```

Add the `bytes` and `encoding/json` imports if the file lacks them.

- [ ] **Step 2: Run them and confirm they fail**

Run: `go test ./internal/radar/ -run 'TestHTTP_AddUserFeed|TestHTTP_UpdateUserFeed' -count=1`
Expected: FAIL — `h.AddUserFeedHandler undefined`.

- [ ] **Step 3: The quota branch in `writeRadarError`**

In `internal/radar/http.go`, add a case **before** `ErrNotFound`:

```go
	case errors.Is(err, ErrQuotaExceeded):
		httpx.WriteError(w, http.StatusConflict, "quota_exceeded", err.Error())
```

Same status as `duplicate`; the client tells them apart by code, not status.

- [ ] **Step 4: The user handlers**

Replace `func (h *HTTP) AddFeedHandler()` and `h.addFeed` with two pairs:

```go
// AddUserFeedHandler returns the http.HandlerFunc for POST /radar/feeds:
// the caller's own personal feed.
func (h *HTTP) AddUserFeedHandler() http.HandlerFunc { return h.addUserFeed }

func (h *HTTP) addUserFeed(w http.ResponseWriter, r *http.Request) {
	var req AddFeedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}

	userID := coreauth.UserID(r.Context())

	res, err := h.svc.AddUserFeed(r.Context(), userID, req)
	if err != nil {
		writeRadarError(w, err)
		return
	}

	status := http.StatusOK // an existing catalog feed was reused
	if res.Created {
		status = http.StatusCreated
	}
	httpx.WriteJSON(w, status, res)
}

// AddGlobalFeedHandler returns the http.HandlerFunc for
// POST /admin/radar/feeds (admin).
func (h *HTTP) AddGlobalFeedHandler() http.HandlerFunc { return h.addGlobalFeed }

func (h *HTTP) addGlobalFeed(w http.ResponseWriter, r *http.Request) {
	var req AddFeedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}

	feed, err := h.svc.AddGlobalFeed(r.Context(), req)
	if err != nil {
		writeRadarError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, feed)
}
```

- [ ] **Step 5: Pairs for PATCH, DELETE, and the admin listing**

Replace `UpdateFeedHandler`/`updateFeed` and `DeleteFeedHandler`/`deleteFeed`
with:

```go
// UpdateUserFeedHandler returns the http.HandlerFunc for PATCH /radar/feeds/{id}.
func (h *HTTP) UpdateUserFeedHandler() http.HandlerFunc {
	return h.patchFeed(func(r *http.Request, id int64, req UpdateFeedRequest) (*Feed, error) {
		return h.svc.UpdateUserFeed(r.Context(), coreauth.UserID(r.Context()), id, req)
	})
}

// UpdateGlobalFeedHandler returns the http.HandlerFunc for
// PATCH /admin/radar/feeds/{id} (admin).
func (h *HTTP) UpdateGlobalFeedHandler() http.HandlerFunc {
	return h.patchFeed(func(r *http.Request, id int64, req UpdateFeedRequest) (*Feed, error) {
		return h.svc.UpdateGlobalFeed(r.Context(), id, req)
	})
}

// patchFeed shares the id parsing and body decoding; the scope comes from the
// closure the route was mounted with.
func (h *HTTP) patchFeed(apply func(*http.Request, int64, UpdateFeedRequest) (*Feed, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		feedID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid feed id")
			return
		}

		var req UpdateFeedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json body")
			return
		}

		feed, err := apply(r, feedID, req)
		if err != nil {
			writeRadarError(w, err)
			return
		}

		httpx.WriteJSON(w, http.StatusOK, feed)
	}
}

// DeleteUserFeedHandler returns the http.HandlerFunc for DELETE /radar/feeds/{id}.
func (h *HTTP) DeleteUserFeedHandler() http.HandlerFunc {
	return h.removeFeed(func(r *http.Request, id int64) error {
		return h.svc.DeleteUserFeed(r.Context(), coreauth.UserID(r.Context()), id)
	})
}

// DeleteGlobalFeedHandler returns the http.HandlerFunc for
// DELETE /admin/radar/feeds/{id} (admin).
func (h *HTTP) DeleteGlobalFeedHandler() http.HandlerFunc {
	return h.removeFeed(func(r *http.Request, id int64) error {
		return h.svc.DeleteGlobalFeed(r.Context(), id)
	})
}

func (h *HTTP) removeFeed(apply func(*http.Request, int64) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		feedID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid feed id")
			return
		}

		if err := apply(r, feedID); err != nil {
			writeRadarError(w, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// ListGlobalFeedsHandler returns the http.HandlerFunc for
// GET /admin/radar/feeds (admin): the shared catalog alone.
func (h *HTTP) ListGlobalFeedsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.listFeedsScoped(w, r, FeedScopeGlobal)
	}
}
```

Turn the existing `listFeeds` into `listFeedsScoped(w, r, scope FeedScope)`,
which sets `ListFeedsParams{Scope: scope}`, and keep `ListFeedsHandler` as a
wrapper passing `FeedScopeVisible`.

- [ ] **Step 6: The quota on `/radar/status`**

In `internal/radar/types.go`:

```go
// RadarStatus is the response for GET /radar/status.
type RadarStatus struct {
	LastSweepAt  *time.Time `json:"last_sweep_at"`
	MaxUserFeeds int        `json:"max_user_feeds"`
}
```

In `internal/radar/service.go`, add a getter:

```go
// MaxUserFeeds exposes the personal-feed quota so the UI can show "n / max".
func (s *Service) MaxUserFeeds() int { return s.maxUserFeeds }
```

In `internal/radar/http.go`, inside `status`, replace the response write with:

```go
	httpx.WriteJSON(w, http.StatusOK, RadarStatus{
		LastSweepAt: last, MaxUserFeeds: h.svc.MaxUserFeeds(),
	})
```

- [ ] **Step 7: Config**

In `internal/core/config/config.go`, next to `RadarMaxWorkers`:

```go
	RadarMaxUserFeeds      int           `env:"LINKTHECA_RADAR_MAX_USER_FEEDS" envDefault:"20"`
```

In `.env.example`, next to the other `LINKTHECA_RADAR_*` entries:

```
# How many personal Radar feeds one account may add.
LINKTHECA_RADAR_MAX_USER_FEEDS=20
```

In `internal/core/config/config_test.go`, add to the defaults test:

```go
	require.Equal(t, 20, cfg.RadarMaxUserFeeds)
```

and to the `t.Setenv` test, the override:

```go
	t.Setenv("LINKTHECA_RADAR_MAX_USER_FEEDS", "5")
	// ...
	require.Equal(t, 5, cfg.RadarMaxUserFeeds)
```

- [ ] **Step 8: Routes**

In `internal/server/server.go`, inside the `r.Route("/radar", …)` block, replace
the `r.Get("/feeds", …)` line and delete the inner admin group:

```go
			r.Get("/feeds", radarHTTP.ListFeedsHandler())
			r.Post("/feeds", radarHTTP.AddUserFeedHandler())
			r.Patch("/feeds/{id}", radarHTTP.UpdateUserFeedHandler())
			r.Delete("/feeds/{id}", radarHTTP.DeleteUserFeedHandler())
			r.Post("/subscriptions", radarHTTP.SubscribeHandler())
			r.Delete("/subscriptions/{feedId}", radarHTTP.UnsubscribeHandler())
```

Right after the closing brace of `r.Route("/radar", …)`, a new block:

```go
		// The admin catalog lives on its own namespace so the route, not a
		// branch inside a shared handler, decides which rows may be written.
		r.Route("/admin/radar", func(r chi.Router) {
			r.Use(coreauth.RequireUser(issuer))
			r.Use(coreauth.RequireAdmin)

			r.Get("/feeds", radarHTTP.ListGlobalFeedsHandler())
			r.Post("/feeds", radarHTTP.AddGlobalFeedHandler())
			r.Patch("/feeds/{id}", radarHTTP.UpdateGlobalFeedHandler())
			r.Delete("/feeds/{id}", radarHTTP.DeleteGlobalFeedHandler())
		})
```

In the else branch, add next to the two existing lines:

```go
		r.HandleFunc("/admin/radar", radar.DisabledHandler)
		r.HandleFunc("/admin/radar/*", radar.DisabledHandler)
```

And pass the quota into the service:

```go
		radarSvc := radar.NewService(radarStore, deps.Radar.Embedder,
			radar.WithMaxUserFeeds(cfg.RadarMaxUserFeeds))
```

The block goes inside the same `if cfg.RadarEnabled && deps.Radar != nil`
branch, beside `r.Route("/radar", …)` rather than nested within it. When
`/admin/users` arrives it will mount as a separate `r.Route("/admin/users", …)`
outside that branch — the prefixes do not overlap, so `chi` will not conflict.

- [ ] **Step 9: Rewrite the admin-gate integration test**

In `internal/radar/integration_test.go`, replace
`TestIntegrationAddFeedRequiresAdmin` with:

```go
func TestIntegrationUserMayAddPersonalFeed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := radar.NewStore(pool)
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	userID := seedRadarUser(t, pool, false) // not admin
	issuer := coreauth.NewJWTIssuer("test-secret-at-least-32-bytes-long-for-hmac", 15*time.Minute)
	token, _ := issuer.Issue(userID, false)

	r := chi.NewRouter()
	r.Route("/radar", func(r chi.Router) {
		r.Use(coreauth.RequireUser(issuer))
		r.Post("/feeds", h.AddUserFeedHandler())
	})

	body, _ := json.Marshal(radar.AddFeedRequest{URL: "https://personal.example/f"})
	req := httptest.NewRequest(http.MethodPost, "/radar/feeds", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var owned int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM radar_feeds WHERE owner_user_id = $1`, userID).Scan(&owned))
	require.Equal(t, 1, owned)
}

func TestIntegrationGlobalFeedsRequireAdmin(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := radar.NewStore(pool)
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	userID := seedRadarUser(t, pool, false) // not admin
	issuer := coreauth.NewJWTIssuer("test-secret-at-least-32-bytes-long-for-hmac", 15*time.Minute)
	token, _ := issuer.Issue(userID, false)

	r := chi.NewRouter()
	r.Route("/admin/radar", func(r chi.Router) {
		r.Use(coreauth.RequireUser(issuer))
		r.Use(coreauth.RequireAdmin)
		r.Post("/feeds", h.AddGlobalFeedHandler())
	})

	body, _ := json.Marshal(radar.AddFeedRequest{URL: "https://x.example/f"})
	req := httptest.NewRequest(http.MethodPost, "/admin/radar/feeds", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)
}
```

- [ ] **Step 10: Integration test for match isolation**

Append to `internal/radar/integration_test.go`:

```go
func TestIntegrationPersonalFeedMatchesOnlyItsOwner(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := radar.NewStore(pool)
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)
	ctx := context.Background()

	owner := seedRadarUser(t, pool, false)
	other := seedRadarUser(t, pool, false)

	ownerTopic, err := svc.CreateTopic(ctx, owner, radar.CreateTopicRequest{
		Name: "Rust", Description: "rust language news and releases",
	})
	require.NoError(t, err)
	otherTopic, err := svc.CreateTopic(ctx, other, radar.CreateTopicRequest{
		Name: "Rust", Description: "rust language news and releases",
	})
	require.NoError(t, err)

	res, err := svc.AddUserFeed(ctx, owner, radar.AddFeedRequest{
		URL: "https://personal.example/only.xml",
	})
	require.NoError(t, err)
	require.True(t, res.Created)

	matchFinding(t, ctx, store, emb, res.Feed.ID, "personal-1")

	require.Equal(t, 1, countMatches(t, pool, ownerTopic.ID))
	require.Equal(t, 0, countMatches(t, pool, otherTopic.ID),
		"a personal feed must not reach another account's topics")
}
```

`seedRadarUser` builds the email from `t.Name()`, so two calls inside one test
collide on `users.email` — fix the helper first by adding a counter to the
address:

```go
var seedUserSeq atomic.Int64

func seedRadarUser(t *testing.T, pool *pgxpool.Pool, isAdmin bool) int64 {
	t.Helper()
	var id int64
	email := fmt.Sprintf("u+%s-%d@example.com", t.Name(), seedUserSeq.Add(1))
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email, password_hash, display_name, is_admin)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		email, "x", "Tester", isAdmin).Scan(&id)
	require.NoError(t, err)
	return id
}
```

Add the `fmt` and `sync/atomic` imports. `matchFinding` and `countMatches`
already exist in the file.

- [ ] **Step 11: Run everything**

Run: `make test-unit && make test`
Expected: PASS.

- [ ] **Step 12: Commit**

```bash
git add internal .env.example
git commit -m "feat(radar): split feed routes into user and admin namespaces

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 7: Frontend — the data layer

**Files:**
- Modify: `web/src/features/radar/types.ts:80-92`, `web/src/features/radar/schemas.ts:160-198`, `web/src/features/radar/api.ts:150-200`, `web/src/features/radar/use-mutations.tsx:138`
- Create: `web/src/features/admin/api.ts`, `web/src/features/admin/use-admin-feeds.tsx`
- Test: `web/src/features/radar/api.test.ts`, `web/src/features/admin/use-admin-feeds.test.tsx`

**Interfaces:**
- Consumes: the JSON from Task 6.
- Produces: `FeedListItem` with `isOwn: boolean`; `addFeed(input: AddFeedInput): Promise<{ created: boolean }>`; in `features/admin/api.ts` — `listGlobalFeeds(): Promise<FeedListItem[]>`, `addGlobalFeed(input: AddFeedInput): Promise<void>`, `updateGlobalFeed(id: number, input: UpdateFeedInput): Promise<void>`, `deleteGlobalFeed(id: number): Promise<void>`; in `use-admin-feeds.tsx` — `adminKeys.feeds`, `useGlobalFeedsQuery()`, `useAddGlobalFeed()`, `useUpdateGlobalFeed()`, `useDeleteGlobalFeed()`.

- [ ] **Step 1: Write the failing API test**

In `web/src/features/radar/api.test.ts`:

```ts
it("maps is_own and reports whether a feed was created", async () => {
  server.use(
    http.get("/api/radar/feeds", () =>
      HttpResponse.json({
        items: [
          {
            id: 3, url: "https://mine.example/rss", kind: "rss", title: "Mine",
            fetch_interval_seconds: 3600, is_active: true,
            last_fetched_at: null, last_error: null,
            created_at: "2026-08-01T10:00:00Z",
            subscribed: true, finding_count: 4, is_own: true,
          },
        ],
        total: 1,
      }),
    ),
    http.post("/api/radar/feeds", () =>
      HttpResponse.json(
        {
          feed: {
            id: 9, url: "https://shared.example/rss", kind: "rss", title: null,
            fetch_interval_seconds: 3600, is_active: true,
            last_fetched_at: null, last_error: null,
            created_at: "2026-08-01T10:00:00Z",
          },
          created: false,
        },
        { status: 200 },
      ),
    ),
  );

  const feeds = await listFeeds();
  expect(feeds[0].isOwn).toBe(true);

  const result = await addFeed({ url: "https://shared.example/rss", fetchIntervalSeconds: 3600 });
  expect(result.created).toBe(false);
});
```

- [ ] **Step 2: Run it and confirm it fails**

Run: `cd web && npx vitest run src/features/radar/api.test.ts`
Expected: FAIL — `feeds[0].isOwn` is `undefined`.

- [ ] **Step 3: Type and schemas**

In `web/src/features/radar/types.ts`, add to `FeedListItem`:

```ts
  isOwn: boolean;
```

In `web/src/features/radar/schemas.ts`:

```ts
export const RawFeedListItemSchema = RawFeedSchema.extend({
  subscribed: z.boolean(),
  finding_count: z.number().int(),
  is_own: z.boolean(),
});

export const RawAddFeedResultSchema = z.object({
  feed: RawFeedSchema,
  created: z.boolean(),
});
```

and add `isOwn: raw.is_own,` to `mapFeedListItem`.

- [ ] **Step 4: `addFeed` returns the flag**

In `web/src/features/radar/api.ts`, replace `addFeed`:

```ts
// The server answers 201 for a new personal feed and 200 when the URL was
// already in the shared catalog and the caller was merely subscribed. The API
// client does not surface status codes, so the flag rides in the body.
export async function addFeed(input: AddFeedInput): Promise<{ created: boolean }> {
  const raw = await apiFetch<unknown>(`/radar/feeds`, {
    method: "POST",
    body: JSON.stringify({
      url: input.url,
      fetch_interval_seconds: input.fetchIntervalSeconds,
    }),
  });
  return { created: parseInDev(RawAddFeedResultSchema, raw).created };
}
```

Add `RawAddFeedResultSchema` to the import from `./schemas`.

- [ ] **Step 5: Test for the admin hooks**

Create `web/src/features/admin/use-admin-feeds.test.tsx`:

```tsx
import { describe, it, expect } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useGlobalFeedsQuery } from "./use-admin-feeds";

function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return {
    qc,
    wrapper: ({ children }: { children: React.ReactNode }) => (
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    ),
  };
}

describe("useGlobalFeedsQuery", () => {
  it("reads the admin catalog endpoint", async () => {
    server.use(
      http.get("/api/admin/radar/feeds", () =>
        HttpResponse.json({
          items: [
            {
              id: 1, url: "https://theverge.com/rss", kind: "rss", title: "The Verge",
              fetch_interval_seconds: 3600, is_active: true,
              last_fetched_at: null, last_error: null,
              created_at: "2026-08-01T10:00:00Z",
              subscribed: false, finding_count: 214, is_own: false,
            },
          ],
          total: 1,
        }),
      ),
    );

    const { wrapper } = makeWrapper();
    const { result } = renderHook(() => useGlobalFeedsQuery(), { wrapper });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.[0].title).toBe("The Verge");
  });
});
```

- [ ] **Step 6: Run it and confirm it fails**

Run: `cd web && npx vitest run src/features/admin/use-admin-feeds.test.tsx`
Expected: FAIL — module `./use-admin-feeds` not found.

- [ ] **Step 7: Implement `features/admin/api.ts`**

```ts
import { apiFetch } from "@/shared/api/client";
import { parseInDev } from "@/features/radar/api";
import { RawFeedListSchema, mapFeedListItem } from "@/features/radar/schemas";
import type { FeedListItem } from "@/features/radar/types";
import type { AddFeedInput, UpdateFeedInput } from "@/features/radar/api";

// The admin catalog is Radar data, so it reuses Radar's wire schemas; only the
// endpoints differ.
export async function listGlobalFeeds(): Promise<FeedListItem[]> {
  const raw = await apiFetch<unknown>(`/admin/radar/feeds?limit=100`);
  return parseInDev(RawFeedListSchema, raw).items.map(mapFeedListItem);
}

export async function addGlobalFeed(input: AddFeedInput): Promise<void> {
  await apiFetch<void>(`/admin/radar/feeds`, {
    method: "POST",
    body: JSON.stringify({
      url: input.url,
      fetch_interval_seconds: input.fetchIntervalSeconds,
    }),
  });
}

export async function updateGlobalFeed(id: number, input: UpdateFeedInput): Promise<void> {
  const body: Record<string, unknown> = {};
  if (input.title !== undefined) body.title = input.title;
  if (input.fetchIntervalSeconds !== undefined) {
    body.fetch_interval_seconds = input.fetchIntervalSeconds;
  }
  if (input.isActive !== undefined) body.is_active = input.isActive;
  await apiFetch<void>(`/admin/radar/feeds/${id}`, {
    method: "PATCH",
    body: JSON.stringify(body),
  });
}

export async function deleteGlobalFeed(id: number): Promise<void> {
  await apiFetch<void>(`/admin/radar/feeds/${id}`, { method: "DELETE" });
}
```

`parseInDev` (`web/src/features/radar/api.ts:30`) is currently not exported —
add `export` to it.

- [ ] **Step 8: Implement `features/admin/use-admin-feeds.tsx`**

```tsx
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  listGlobalFeeds,
  addGlobalFeed,
  updateGlobalFeed,
  deleteGlobalFeed,
} from "./api";
import type { AddFeedInput, UpdateFeedInput } from "@/features/radar/api";

export const adminKeys = {
  all: ["admin"] as const,
  feeds: ["admin", "feeds"] as const,
};

export function useGlobalFeedsQuery() {
  return useQuery({ queryKey: adminKeys.feeds, queryFn: listGlobalFeeds });
}

export function useAddGlobalFeed() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: AddFeedInput) => addGlobalFeed(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: adminKeys.feeds }),
  });
}

export function useUpdateGlobalFeed() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: UpdateFeedInput }) =>
      updateGlobalFeed(id, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: adminKeys.feeds }),
  });
}

export function useDeleteGlobalFeed() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => deleteGlobalFeed(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: adminKeys.feeds }),
  });
}
```

The admin catalog mutations also invalidate `radarKeys.feeds`: a deleted or
paused global feed must disappear from the Radar screen too. Add to each
`onSuccess`:

```tsx
      qc.invalidateQueries({ queryKey: radarKeys.feeds });
```

importing `radarKeys` from `@/features/radar/use-radar`.

- [ ] **Step 9: Run the tests**

Run: `cd web && npm test && npm run typecheck && npm run lint`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add web/src/features
git commit -m "feat(admin): add the global feed catalog data layer

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 8: Admin navigation and the section gate

**Files:**
- Create: `web/src/shared/layout/AdminRoute.tsx`, `web/src/shared/layout/AdminRoute.test.tsx`
- Modify: `web/src/shared/layout/Sidebar.tsx:5-9`, `web/src/shared/layout/Sidebar.test.tsx`
- Modify: `web/src/App.tsx:24-57`
- Create: a placeholder `web/src/routes/admin.sources.tsx` (the full screen lands in Task 11)

**Interfaces:**
- Consumes: `useAuthStore` (`web/src/features/auth/store.ts`), `FullPageSpinner`.
- Produces: an `AdminRoute` component (a layout route with no props); the routes `admin` → redirect to `admin/sources` and `admin/sources`; `Sidebar` renders the `Admin` item only for an admin.

- [ ] **Step 1: Write the failing tests**

Create `web/src/shared/layout/AdminRoute.test.tsx`:

```tsx
import { describe, it, expect, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router";
import { AdminRoute } from "./AdminRoute";
import { useAuthStore } from "@/features/auth/store";

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route element={<AdminRoute />}>
          <Route path="admin/sources" element={<p>Admin screen</p>} />
        </Route>
        <Route path="library" element={<p>Library screen</p>} />
        <Route path="login" element={<p>Login screen</p>} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("AdminRoute", () => {
  beforeEach(() => {
    useAuthStore.setState({ status: "anonymous", user: null });
  });

  it("lets an admin through", () => {
    useAuthStore.getState().setSession("t", {
      id: 1, email: "a@example.com", displayName: "A", isAdmin: true,
    });
    renderAt("/admin/sources");
    expect(screen.getByText("Admin screen")).toBeInTheDocument();
  });

  it("sends a signed-in non-admin to the library", () => {
    useAuthStore.getState().setSession("t", {
      id: 2, email: "b@example.com", displayName: "B", isAdmin: false,
    });
    renderAt("/admin/sources");
    expect(screen.getByText("Library screen")).toBeInTheDocument();
  });

  it("sends an anonymous visitor to login", () => {
    renderAt("/admin/sources");
    expect(screen.getByText("Login screen")).toBeInTheDocument();
  });
});
```

In `web/src/shared/layout/Sidebar.test.tsx`, replace the second test with:

```tsx
  it("renders Library, Radar, and Settings as enabled nav links", () => {
    useAuthStore.setState({ status: "anonymous", user: null });
    renderWithRouter();
    expect(screen.getByRole("link", { name: /library/i })).toHaveAttribute("href", "/library");
    expect(screen.getByRole("link", { name: /radar/i })).toHaveAttribute("href", "/radar");
    expect(screen.getByRole("link", { name: /settings/i })).toHaveAttribute("href", "/settings");
  });

  it("hides Admin from a non-admin", () => {
    useAuthStore.getState().setSession("t", {
      id: 1, email: "a@example.com", displayName: "A", isAdmin: false,
    });
    renderWithRouter();
    expect(screen.queryByRole("link", { name: /admin/i })).not.toBeInTheDocument();
  });

  it("shows Admin to an admin", () => {
    useAuthStore.getState().setSession("t", {
      id: 1, email: "a@example.com", displayName: "A", isAdmin: true,
    });
    renderWithRouter();
    expect(screen.getByRole("link", { name: /admin/i })).toHaveAttribute("href", "/admin/sources");
  });
```

Add the `useAuthStore` import to the test file. Check the `setSession` signature
against `web/src/routes/radar.sources.test.tsx`, which already has such a helper.

- [ ] **Step 2: Run them and confirm they fail**

Run: `cd web && npx vitest run src/shared/layout`
Expected: FAIL — module `./AdminRoute` not found; the Sidebar does not render Admin.

- [ ] **Step 3: Implement `AdminRoute`**

`web/src/shared/layout/AdminRoute.tsx`:

```tsx
import { Navigate, Outlet, useLocation } from "react-router";
import { useAuthStore } from "@/features/auth/store";
import { FullPageSpinner } from "./FullPageSpinner";

// Cosmetic gate. The backend's RequireAdmin is what actually protects the data;
// this only keeps the screen out of a non-admin's way.
export function AdminRoute() {
  const status = useAuthStore((s) => s.status);
  const isAdmin = useAuthStore((s) => s.user?.isAdmin ?? false);
  const location = useLocation();

  if (status === "bootstrapping") return <FullPageSpinner />;
  if (status === "anonymous") {
    return <Navigate to="/login" state={{ from: location }} replace />;
  }
  if (!isAdmin) return <Navigate to="/library" replace />;

  return <Outlet />;
}
```

- [ ] **Step 4: A role-aware `Sidebar`**

In `web/src/shared/layout/Sidebar.tsx`, replace the constant and read the store:

```tsx
import { NavLink } from "react-router";
import { cn } from "@/shared/lib/cn";
import { APP_VERSION } from "@/shared/version";
import { useAuthStore } from "@/features/auth/store";

const baseNavItems = [
  { to: "/library", label: "Library" },
  { to: "/radar", label: "Radar" },
  { to: "/settings", label: "Settings" },
];

const adminNavItem = { to: "/admin/sources", label: "Admin" };

export function Sidebar({ onNavigate }: { onNavigate?: () => void }) {
  const isAdmin = useAuthStore((s) => s.user?.isAdmin ?? false);
  // Numbers follow position rather than being pinned to a label, so adding
  // Admin does not renumber the items above it.
  const navItems = isAdmin ? [...baseNavItems, adminNavItem] : baseNavItems;
```

In the `map`, replace the `item.number` reference with a computed number:

```tsx
          {navItems.map((item, i) => (
            <li key={item.to}>
              <NavLink
                to={item.to}
                onClick={onNavigate}
                className={({ isActive }) =>
                  cn(
                    "nav-item flex items-baseline gap-3 px-4 py-2 hover:text-ink",
                    isActive && "active",
                  )
                }
              >
                <span className="nav-number font-mono text-xs text-muted-foreground">
                  {String(i + 1).padStart(2, "0")}
                </span>
                <span className="nav-label font-display text-xl text-ink-3">{item.label}</span>
              </NavLink>
            </li>
          ))}
```

- [ ] **Step 5: A placeholder screen and the routes**

Create `web/src/routes/admin.sources.tsx` — a temporary minimum; the full screen
arrives in Task 11:

```tsx
import { PageHeader } from "@/shared/layout/PageHeader";

export default function AdminSourcesRoute() {
  return (
    <div>
      <PageHeader
        title="Global sources"
        subtitle="Feeds every account can subscribe to"
      />
    </div>
  );
}
```

In `web/src/App.tsx`, add the imports:

```tsx
import { Navigate } from "react-router";
import { AdminRoute } from "@/shared/layout/AdminRoute";
import AdminSourcesRoute from "./routes/admin.sources";
```

and inside `AppLayout`'s `children`, after `settings`:

```tsx
              {
                element: <AdminRoute />,
                children: [
                  { path: "admin", element: <Navigate to="/admin/sources" replace /> },
                  { path: "admin/sources", element: <AdminSourcesRoute /> },
                ],
              },
```

- [ ] **Step 6: Run the tests**

Run: `cd web && npm test && npm run typecheck && npm run lint`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add web/src
git commit -m "feat(admin): add the Admin section to the sidebar behind a role gate

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 9: `SourceRow` and the dialogs across both scopes

**Files:**
- Modify: `web/src/features/radar/components/SourceRow.tsx`, `SourceRow.test.tsx`
- Modify: `web/src/features/radar/components/AddFeedDialog.tsx`, `EditFeedDialog.tsx`, `DeleteFeedConfirm.tsx`
- Test: `web/src/features/radar/components/AddFeedDialog.test.tsx`

**Interfaces:**
- Consumes: `useAddFeed`/`useUpdateFeed`/`useDeleteFeed` (`features/radar/use-mutations`), `useAddGlobalFeed`/`useUpdateGlobalFeed`/`useDeleteGlobalFeed` (Task 7), `FeedListItem.isOwn` (Task 7).
- Produces: `SourceRow` with props `{ feed: FeedListItem; canManage: boolean; onToggle?: (subscribed: boolean) => void; onEdit?: () => void; onDelete?: () => void }`; `AddFeedDialog` with a `scope: "personal" | "global"` prop; `EditFeedDialog` and `DeleteFeedConfirm` pick their scope from `feed.isOwn`.

- [ ] **Step 1: Rewrite the row test**

In `web/src/features/radar/components/SourceRow.test.tsx`, add `isOwn: false,`
to the `feed()` factory and replace the tests with:

```tsx
  it("hides the management actions when the row is not manageable", () => {
    render(<SourceRow feed={feed()} canManage={false} onToggle={() => {}} />);
    expect(screen.queryByRole("button", { name: /edit/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /delete/i })).not.toBeInTheDocument();
  });

  it("shows them when it is", () => {
    render(
      <SourceRow
        feed={feed({ isOwn: true })}
        canManage
        onToggle={() => {}}
        onEdit={() => {}}
        onDelete={() => {}}
      />,
    );
    expect(screen.getByRole("button", { name: /edit/i })).toBeInTheDocument();
  });

  it("omits the checkbox when subscription is not offered", () => {
    render(<SourceRow feed={feed()} canManage onEdit={() => {}} onDelete={() => {}} />);
    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
    expect(screen.getByText("The Verge")).toBeInTheDocument();
  });

  it("toggles the subscription", async () => {
    const onToggle = vi.fn();
    render(<SourceRow feed={feed()} canManage={false} onToggle={onToggle} />);
    await userEvent.click(screen.getByRole("checkbox", { name: /the verge/i }));
    expect(onToggle).toHaveBeenCalledWith(true);
  });
```

Adjust the existing "falls back to the hostname…" and "marks a paused feed"
tests to the new props (`canManage` instead of `isAdmin`, optional callbacks).

- [ ] **Step 2: Run them and confirm they fail**

Run: `cd web && npx vitest run src/features/radar/components/SourceRow.test.tsx`
Expected: FAIL — prop types do not match, and the checkbox always renders.

- [ ] **Step 3: Rewrite `SourceRow`**

Replace the props block and the markup:

```tsx
type Props = {
  feed: FeedListItem;
  canManage: boolean;
  // Absent on the admin screen: curating the catalog is a different job from
  // subscribing to it.
  onToggle?: (subscribed: boolean) => void;
  onEdit?: () => void;
  onDelete?: () => void;
};

export function SourceRow({ feed, canManage, onToggle, onEdit, onDelete }: Props) {
  const name = feed.title ?? host(feed.url);
  const inputId = `feed-${feed.id}`;

  return (
    <div
      className={`flex items-start justify-between gap-4 py-4 border-b border-rule ${
        feed.isActive ? "" : "opacity-60"
      }`}
    >
      <div className="flex items-start gap-3">
        {onToggle && (
          <input
            id={inputId}
            type="checkbox"
            checked={feed.subscribed}
            onChange={(e) => onToggle(e.target.checked)}
            className="mt-1 h-4 w-4 accent-vermillion"
          />
        )}
        <div>
          {onToggle ? (
            <label htmlFor={inputId} className="font-display text-xl text-ink cursor-pointer">
              {name}
            </label>
          ) : (
            <p className="font-display text-xl text-ink">{name}</p>
          )}
          <p className="label-sc mt-1 text-muted-foreground">{meta(feed).join(" · ")}</p>
        </div>
      </div>

      {canManage && (
        <div className="flex items-center gap-3 shrink-0">
          <button
            type="button"
            onClick={onEdit}
            className="label-sc text-muted-foreground hover:text-vermillion"
          >
            Edit
          </button>
          <button
            type="button"
            onClick={onDelete}
            className="label-sc text-muted-foreground hover:text-vermillion"
          >
            Delete
          </button>
        </div>
      )}
    </div>
  );
}
```

- [ ] **Step 4: `AddFeedDialog` test for the quota**

Append to `web/src/features/radar/components/AddFeedDialog.test.tsx`:

```tsx
it("shows the quota error inline", async () => {
  server.use(
    http.post("/api/radar/feeds", () =>
      HttpResponse.json(
        { error: "quota_exceeded", message: "at most 20 personal feeds" },
        { status: 409 },
      ),
    ),
  );

  renderDialog({ scope: "personal" });
  await userEvent.type(screen.getByLabelText(/url/i), "https://x.example/rss");
  await userEvent.click(screen.getByRole("button", { name: /add/i }));

  expect(await screen.findByRole("alert")).toHaveTextContent(/limit/i);
});
```

Check the file's existing render helper and pass the new `scope` prop through it.

- [ ] **Step 5: Scope and errors in `AddFeedDialog`**

Replace `mapFeedError`:

```tsx
export function mapFeedError(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.code === "quota_exceeded") {
      return "You've reached your limit of personal sources. Remove one first.";
    }
    if (err.status === 409) return "This feed is already in your sources";
    if (err.status === 400) return err.message || "Invalid input";
    if (err.status === 403) return "You are not allowed to add this feed";
  }
  return "Could not save — please try again";
}
```

Have `AddFeedForm` take the scope and pick its mutation. Both hooks are called
unconditionally — they are plain `useMutation` objects, so the rules of hooks
hold:

```tsx
type Scope = "personal" | "global";

function AddFeedForm({ scope, onClose }: { scope: Scope; onClose: () => void }) {
  const personal = useAddFeed();
  const global = useAddGlobalFeed();
  const add = scope === "global" ? global : personal;
```

In `onSubmit`, distinguish the two outcomes for the personal scope:

```tsx
    try {
      if (scope === "global") {
        await global.mutateAsync({ url, fetchIntervalSeconds });
        toast.success("Feed added to the catalog");
      } else {
        const { created } = await personal.mutateAsync({ url, fetchIntervalSeconds });
        toast.success(
          created ? "Source added" : "Already in the shared catalog — subscribed",
        );
      }
      onClose();
    } catch (err) {
      setTopError(mapFeedError(err));
    }
```

The outer component takes `scope` and swaps the copy:

```tsx
type Props = {
  open: boolean;
  scope: Scope;
  onOpenChange: (open: boolean) => void;
};

export function AddFeedDialog({ open, scope, onOpenChange }: Props) {
  const isGlobal = scope === "global";
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="paper-surface max-h-[85dvh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle className="display-tight text-3xl">
            {isGlobal ? "Add feed" : "Add source"}
          </DialogTitle>
          <DialogDescription className="label-sc text-muted-foreground">
            {isGlobal
              ? "Everyone on this instance can subscribe to it."
              : "Only you will see it."}
          </DialogDescription>
        </DialogHeader>
        {open && <AddFeedForm scope={scope} onClose={() => onOpenChange(false)} />}
      </DialogContent>
    </Dialog>
  );
}
```

Label the submit button `{add.isPending ? "Adding…" : isGlobal ? "Add feed" : "Add source"}` —
`scope` is already threaded into the form for that.

- [ ] **Step 6: Scope in `EditFeedDialog` and `DeleteFeedConfirm`**

`EditFeedDialog` receives the feed, so the scope follows from it and no new prop
is needed. In `EditFeedForm`:

```tsx
  const personal = useUpdateFeed();
  const global = useUpdateGlobalFeed();
  // A row is manageable on exactly one screen, so its ownership picks the scope.
  const update = feed.isOwn ? personal : global;
```

`DeleteFeedConfirm` holds no mutation of its own — its props
(`web/src/features/radar/components/DeleteFeedConfirm.tsx:13-18`) are
`{ feed, pending, onOpenChange, onConfirm }`, and the screen supplies the right
mutation. That contract stays; only the consequence text changes:

```tsx
  const consequence = feed.isOwn
    ? `${feed.findingCount} findings and their matches will be removed.`
    : `${feed.findingCount} findings and their matches will be removed for all users.`;
```

- [ ] **Step 7: Run the tests**

Run: `cd web && npm test && npm run typecheck && npm run lint`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add web/src/features/radar
git commit -m "refactor(radar): make the source row and dialogs scope-aware

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 10: The `/radar/sources` screen with two sections

**Files:**
- Modify: `web/src/routes/radar.sources.tsx`
- Test: `web/src/routes/radar.sources.test.tsx`

**Interfaces:**
- Consumes: `useFeedsQuery`, `useRadarStatusQuery` (`web/src/features/radar/use-radar.tsx:98`), `useToggleSubscription`, `useDeleteFeed`, `SourceRow` and the dialogs from Task 9.
- Produces: a screen with no exported API; its behaviour is pinned by the tests.

- [ ] **Step 1: Write the failing screen tests**

In `web/src/routes/radar.sources.test.tsx`, add `is_own` to the `rawFeed`
factory and append:

```tsx
it("splits personal sources from the shared catalog", async () => {
  server.use(
    http.get("/api/radar/feeds", () =>
      HttpResponse.json({
        items: [
          { ...rawFeed(1, true), title: "My Blog", is_own: true },
          { ...rawFeed(2, false), title: "The Verge", is_own: false },
        ],
        total: 2,
      }),
    ),
  );

  signIn(false);
  renderRoute();

  expect(await screen.findByText("My Blog")).toBeInTheDocument();
  expect(screen.getByText(/my sources/i)).toBeInTheDocument();
  expect(screen.getByText(/catalog/i)).toBeInTheDocument();

  // Only the personal row is manageable.
  expect(screen.getAllByRole("button", { name: /edit/i })).toHaveLength(1);
});

it("offers Add source to an ordinary user", async () => {
  server.use(
    http.get("/api/radar/feeds", () => HttpResponse.json({ items: [], total: 0 })),
  );

  signIn(false);
  renderRoute();

  expect(await screen.findByRole("button", { name: /add source/i })).toBeInTheDocument();
});
```

Delete the existing "hides the add-feed button from ordinary users" test — that
behaviour is gone.

- [ ] **Step 2: Run them and confirm they fail**

Run: `cd web && npx vitest run src/routes/radar.sources.test.tsx`
Expected: FAIL — there are no sections, and the button is hidden from non-admins.

- [ ] **Step 3: Sections and dialog entry points**

Replace the body of `SourcesRoute`:

```tsx
export default function SourcesRoute() {
  const feeds = useFeedsQuery();
  const status = useRadarStatusQuery();
  const toggle = useToggleSubscription();
  const remove = useDeleteFeed();

  const [addOpen, setAddOpen] = useState(false);
  const [editing, setEditing] = useState<FeedListItem | null>(null);
  const [deleting, setDeleting] = useState<FeedListItem | null>(null);

  if (feeds.error instanceof ApiError && feeds.error.code === "radar_disabled") {
    return <RadarDisabled />;
  }

  const items = feeds.data ?? [];
  const mine = items.filter((f) => f.isOwn);
  const catalog = items.filter((f) => !f.isOwn);
  const subscribedCount = items.filter((f) => f.subscribed).length;
  const quota = status.data?.maxUserFeeds;

  return (
    <div>
      <PageHeader
        title="Sources"
        subtitle={
          items.length
            ? `${items.length} feeds · ${subscribedCount} subscribed · changes apply from the next sweep`
            : "Feeds this instance watches"
        }
        actions={<Button onClick={() => setAddOpen(true)}>Add source</Button>}
      />
      <div className="px-4 lg:px-8 pb-10">
        <SectionHeader
          label="My sources"
          count={quota ? `${mine.length} / ${quota}` : `${mine.length}`}
        />
        {mine.length === 0 ? (
          <p className="font-body text-muted-foreground pb-6">
            Add your own RSS or Atom feed — only you will see it.
          </p>
        ) : (
          mine.map((feed) => (
            <SourceRow
              key={feed.id}
              feed={feed}
              canManage
              onToggle={(subscribed) => toggle.mutate({ feedId: feed.id, subscribed })}
              onEdit={() => setEditing(feed)}
              onDelete={() => setDeleting(feed)}
            />
          ))
        )}

        <SectionHeader label="Catalog" count={`${catalog.length} feeds`} />
        {catalog.length === 0 ? (
          <p className="font-body text-muted-foreground pb-6">
            No shared sources yet. Ask the instance admin to add feeds.
          </p>
        ) : (
          catalog.map((feed) => (
            <SourceRow
              key={feed.id}
              feed={feed}
              canManage={false}
              onToggle={(subscribed) => toggle.mutate({ feedId: feed.id, subscribed })}
            />
          ))
        )}
      </div>

      <AddFeedDialog open={addOpen} scope="personal" onOpenChange={setAddOpen} />
      <EditFeedDialog
        feed={editing}
        onOpenChange={(open) => {
          if (!open) setEditing(null);
        }}
      />
      <DeleteFeedConfirm
        feed={deleting}
        pending={remove.isPending}
        onOpenChange={(open) => {
          if (!open) setDeleting(null);
        }}
        onConfirm={async () => {
          if (!deleting) return;
          try {
            await remove.mutateAsync(deleting.id);
            toast.success("Source deleted");
          } catch {
            toast.error("Could not delete the source");
          } finally {
            setDeleting(null);
          }
        }}
      />
    </div>
  );
}

// SectionHeader repeats the divider used on the topics list.
function SectionHeader({ label, count }: { label: string; count: string }) {
  return (
    <div className="flex items-center gap-4 pt-8 pb-4">
      <div className="label-sc-lg text-ink">{label}</div>
      <div className="flex-1 rule-dotted" />
      <div className="label-sc text-muted-foreground">{count}</div>
    </div>
  );
}
```

Drop the `useAuthStore` import — it is no longer needed — and add the
`useRadarStatusQuery` import from `@/features/radar/use-radar`.

- [ ] **Step 4: Thread `maxUserFeeds` through the status**

In `web/src/features/radar/schemas.ts`, add
`max_user_feeds: z.number().int().optional(),` to `RawRadarStatusSchema`, and
`maxUserFeeds: raw.max_user_feeds ?? null,` to `mapRadarStatus`. In
`web/src/features/radar/types.ts`, add `maxUserFeeds: number | null;` to the
`RadarStatus` type.

The field is optional in the schema so that existing msw status fixtures in
other tests do not start failing.

- [ ] **Step 5: Run the tests**

Run: `cd web && npm test && npm run typecheck && npm run lint`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src
git commit -m "feat(radar): split the sources screen into personal and catalog

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 11: The `/admin/sources` screen

**Files:**
- Modify: `web/src/routes/admin.sources.tsx` (the placeholder from Task 8)
- Create: `web/src/routes/admin.sources.test.tsx`

**Interfaces:**
- Consumes: `useGlobalFeedsQuery`, `useDeleteGlobalFeed` (Task 7), `SourceRow`, `AddFeedDialog`, `EditFeedDialog`, `DeleteFeedConfirm` (Task 9).
- Produces: a screen with no exported API.

- [ ] **Step 1: Write the failing test**

Create `web/src/routes/admin.sources.test.tsx` modelled on
`web/src/routes/radar.sources.test.tsx` (a local `QueryClient` with
`retry:false`, `MemoryRouter`, snake_case msw fixtures):

```tsx
it("manages catalog rows and offers no subscription checkbox", async () => {
  server.use(
    http.get("/api/admin/radar/feeds", () =>
      HttpResponse.json({
        items: [
          {
            id: 1, url: "https://theverge.com/rss", kind: "rss", title: "The Verge",
            fetch_interval_seconds: 3600, is_active: true,
            last_fetched_at: null, last_error: null,
            created_at: "2026-08-01T10:00:00Z",
            subscribed: false, finding_count: 214, is_own: false,
          },
        ],
        total: 1,
      }),
    ),
  );

  renderRoute();

  expect(await screen.findByText("The Verge")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: /edit/i })).toBeInTheDocument();
  expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
});

it("prompts to add the first feed when the catalog is empty", async () => {
  server.use(
    http.get("/api/admin/radar/feeds", () => HttpResponse.json({ items: [], total: 0 })),
  );

  renderRoute();

  expect(await screen.findByText(/add the first shared feed/i)).toBeInTheDocument();
});
```

- [ ] **Step 2: Run it and confirm it fails**

Run: `cd web && npx vitest run src/routes/admin.sources.test.tsx`
Expected: FAIL — the placeholder renders nothing of the sort.

- [ ] **Step 3: Implement the screen**

```tsx
import { useState } from "react";
import { toast } from "sonner";
import { PageHeader } from "@/shared/layout/PageHeader";
import { Button } from "@/shared/ui/button";
import { ApiError } from "@/shared/api/errors";
import {
  useGlobalFeedsQuery,
  useDeleteGlobalFeed,
} from "@/features/admin/use-admin-feeds";
import { SourceRow } from "@/features/radar/components/SourceRow";
import { AddFeedDialog } from "@/features/radar/components/AddFeedDialog";
import { EditFeedDialog } from "@/features/radar/components/EditFeedDialog";
import { DeleteFeedConfirm } from "@/features/radar/components/DeleteFeedConfirm";
import { RadarDisabled } from "@/features/radar/components/RadarDisabled";
import type { FeedListItem } from "@/features/radar/types";

export default function AdminSourcesRoute() {
  const feeds = useGlobalFeedsQuery();
  const remove = useDeleteGlobalFeed();

  const [addOpen, setAddOpen] = useState(false);
  const [editing, setEditing] = useState<FeedListItem | null>(null);
  const [deleting, setDeleting] = useState<FeedListItem | null>(null);

  if (feeds.error instanceof ApiError && feeds.error.code === "radar_disabled") {
    return <RadarDisabled />;
  }

  const items = feeds.data ?? [];

  return (
    <div>
      <PageHeader
        title="Global sources"
        subtitle={
          items.length
            ? `${items.length} feeds · every account can subscribe to them`
            : "Feeds every account can subscribe to"
        }
        actions={<Button onClick={() => setAddOpen(true)}>Add feed</Button>}
      />
      <div className="px-4 lg:px-8 pb-10">
        {feeds.isSuccess && items.length === 0 && (
          <p className="font-body text-muted-foreground pt-8">
            Add the first shared feed to start watching.
          </p>
        )}
        {items.map((feed) => (
          <SourceRow
            key={feed.id}
            feed={feed}
            canManage
            onEdit={() => setEditing(feed)}
            onDelete={() => setDeleting(feed)}
          />
        ))}
      </div>

      <AddFeedDialog open={addOpen} scope="global" onOpenChange={setAddOpen} />
      <EditFeedDialog
        feed={editing}
        onOpenChange={(open) => {
          if (!open) setEditing(null);
        }}
      />
      <DeleteFeedConfirm
        feed={deleting}
        pending={remove.isPending}
        onOpenChange={(open) => {
          if (!open) setDeleting(null);
        }}
        onConfirm={async () => {
          if (!deleting) return;
          try {
            await remove.mutateAsync(deleting.id);
            toast.success("Feed deleted");
          } catch {
            toast.error("Could not delete the feed");
          } finally {
            setDeleting(null);
          }
        }}
      />
    </div>
  );
}
```

- [ ] **Step 4: Run the tests**

Run: `cd web && npm test && npm run typecheck && npm run lint`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/routes
git commit -m "feat(admin): add the global sources screen

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 12: Documentation and manual verification

**Files:**
- Modify: `docs/superpowers/specs/2026-05-06-user-added-feeds-deferred.md`
- Modify: `docs/superpowers/specs/2026-08-15-radar-sources-ux-design.md`

**Interfaces:**
- Consumes: all the behaviour from Tasks 1-11.
- Produces: documentation that reflects the new state.

- [ ] **Step 1: Mark the deferred spec closed**

In the header of `docs/superpowers/specs/2026-05-06-user-added-feeds-deferred.md`,
replace the status line with:

```markdown
**Status:** superseded by
`docs/superpowers/specs/2026-08-24-personal-feeds-and-admin-design.md` (2026-08-24).
Personal feeds, URL deduplication, and the quota are implemented there. Still
deferred: promoting a personal feed into the catalog, demoting it back, and
editing quotas from the UI.
**Decided:** 2026-05-06.
```

- [ ] **Step 2: Record the revised decision**

In `docs/superpowers/specs/2026-08-15-radar-sources-ux-design.md`, under decision
#1 of the `## Decisions` section, add:

```markdown
   > **Revised 2026-08-24**
   > (`2026-08-24-personal-feeds-and-admin-design.md`): managing the global
   > catalog moved into the Admin section, and `/radar/sources` was split into
   > personal sources and the catalog.
```

- [ ] **Step 3: Manual verification**

Bring up `make dev-db && make run`, then `cd web && npm run dev`:

1. Sign in as the admin → the sidebar shows **Admin**; `/admin/sources` lists the
   global feeds with Edit/Delete and no checkboxes; add a global feed.
2. Register a second user → they are auto-subscribed to the active global feeds;
   no Admin item in their sidebar; navigating to `/admin/sources` by hand
   redirects to `/library`.
3. As the second user on `/radar/sources`: add your own feed → it appears under
   `MY SOURCES` with Edit/Delete and marked subscribed; the counter reads `1 / 20`.
4. Add a URL that is already in the global catalog → no new row appears, the feed
   under `CATALOG` becomes checked, and the toast mentions the subscription.
5. The admin does not see the second user's personal feed on `/admin/sources`, and
   `curl -X PATCH /api/admin/radar/feeds/{its id}` answers 404.
6. As the first user, `curl -X PATCH /api/radar/feeds/{another user's personal id}` → 404.
7. Restart with `LINKTHECA_RADAR_ENABLED=false` → `/admin/radar/feeds` answers
   `radar_disabled`.

- [ ] **Step 4: Final run**

Run: `make test && cd web && npm test && npm run typecheck && npm run lint`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add docs
git commit -m "docs(radar): record that personal feeds supersede the deferred spec

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```
