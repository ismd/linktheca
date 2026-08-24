# Radar Sources UX Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the user a `/radar/sources` screen with the feed catalog and subscription toggles, and give the admin add, edit, pause, and delete right inside the catalog rows.

**Architecture:** The catalog is served by a single endpoint, `GET /radar/feeds`, open to every role; each row carries `subscribed` and `finding_count`. Writes on feeds stay behind `RequireAdmin`, and subscriptions behind the ordinary `RequireUser`. A new user is subscribed to the whole active catalog through an `OnUserCreated` hook, which `server.go` installs into `auth.Service` only when Radar is enabled. Separately, the crawler starts saving the channel title into `radar_feeds.title`.

**Tech Stack:** Go 1.x (chi, pgx, River, gofeed, testify, testcontainers), React 19 + TypeScript (react-router, TanStack Query, zod, react-hook-form, Radix UI, Tailwind), with tests in vitest + Testing Library + msw.

**Spec:** `docs/superpowers/specs/2026-08-15-radar-sources-ux-design.md`

## Global Constraints

- No database migrations. The `radar_feeds` / `radar_feed_subscriptions` / `radar_findings` / `radar_topic_matches` schema is used as is.
- Users' personal feeds, quotas, and the `created_by` column are out of scope. Do not add them.
- The fetch interval is validated against the bounds already set in `internal/radar/service.go:110-112`: `defaultFetchIntervalSeconds = 3600`, `minFetchIntervalSeconds = 300`, `maxFetchIntervalSeconds = 86400`.
- Any new method on `radar.StoreAPI` (`internal/radar/service.go:33`) must also be added to `mockStore` in `internal/radar/service_test.go` — a compile-time `var _ radar.StoreAPI = (*mockStore)(nil)` lives there, and without it the whole test package fails to build.
- Clearing a feed title travels over the wire as an empty string: `{"title": ""}` → `NULL` in the database. An absent field means "do not change".
- Go tests: `make test-unit` (fast, `-short`), `make test` (everything, needs Docker for testcontainers). Frontend: `cd web && npm test`.
- Every commit carries `Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>`.

---

## File Structure

**Backend, modified:**
- `internal/radar/types.go` — the catalog DTOs (`FeedListItem`, `UpdateFeedRequest`, `UpdateFeedParams`), plus an edit to `FeedList`.
- `internal/radar/store.go` — `ListFeeds` (a new signature plus subqueries), `Unsubscribe`, `UpdateFeed`, `DeleteFeed`, `SeedSubscriptions`, and `MarkFeedFetched` (+title).
- `internal/radar/service.go` — `validateFetchInterval`, `UpdateFeed`, `DeleteFeed`, `Unsubscribe`, `SeedSubscriptions`, the new `ListFeeds` signature, and the `StoreAPI` extension.
- `internal/radar/http.go` — `updateFeed`, `deleteFeed`, `unsubscribe`, and an edit to `listFeeds`.
- `internal/radar/crawler/crawler.go` — `ParsedFeed` and the new `Parse` signature.
- `internal/radar/jobs/crawl_feed.go` — passing the channel title into `MarkFeedFetched`.
- `internal/auth/service.go` — the `OnUserCreated` field on `ServiceConfig` and its call in `Register`.
- `internal/server/server.go` — the routes and the hook wiring.

**Backend, tests:** `internal/radar/store_test.go`, `internal/radar/service_test.go`, `internal/radar/http_test.go`, `internal/radar/integration_test.go`, `internal/radar/crawler/crawler_test.go`, `internal/radar/jobs/jobs_test.go`, `internal/auth/service_test.go`.

**Frontend, created:**
- `web/src/features/radar/components/SourceRow.tsx` — the catalog row.
- `web/src/features/radar/components/AddFeedDialog.tsx`, `EditFeedDialog.tsx`, `DeleteFeedConfirm.tsx` — the admin dialogs.
- `web/src/routes/radar.sources.tsx` — the screen.
- Tests beside them: `SourceRow.test.tsx`, `radar.sources.test.tsx`, `AddFeedDialog.test.tsx`.

**Frontend, modified:** `web/src/features/radar/types.ts`, `schemas.ts`, `api.ts`, `use-radar.tsx`, `use-mutations.tsx`, `web/src/App.tsx`, `web/src/routes/radar._index.tsx`, `web/src/routes/radar.topics._index.tsx`.

---

### Task 1: The feed catalog — the read model

**Files:**
- Modify: `internal/radar/types.go` (the `FeedList` block, ~line 175)
- Modify: `internal/radar/store.go:573` (`ListFeeds`)
- Modify: `internal/radar/service.go:33` (`StoreAPI`), `internal/radar/service.go:298` (`ListFeeds`)
- Modify: `internal/radar/http.go:308` (`listFeeds`)
- Modify: `internal/server/server.go:136-147` (moving the route)
- Test: `internal/radar/store_test.go`, `internal/radar/service_test.go`, `internal/radar/http_test.go`

**Interfaces:**
- Consumes: the existing `Feed` and `ListFeedsParams`.
- Produces: `radar.FeedListItem{Feed; Subscribed bool; FindingCount int}`; `FeedList{Items []FeedListItem; Total int}`; `Store.ListFeeds(ctx, userID int64, p ListFeedsParams) ([]FeedListItem, int, error)`; `Service.ListFeeds(ctx, userID int64, p ListFeedsParams) (*FeedList, error)`.

- [x] **Step 1: Write the failing store test**

In `internal/radar/store_test.go`:

```go
func TestStore_ListFeeds_SubscribedAndCounts(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)

	subscribed, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: fmt.Sprintf("https://a.example/%d.xml", userID), Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)
	other, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: fmt.Sprintf("https://b.example/%d.xml", userID), Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)

	_, err = store.Subscribe(ctx, userID, subscribed.ID)
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		ext := fmt.Sprintf("ext-%d", i)
		_, _, err = store.UpsertFinding(ctx, radar.FindingUpsert{
			FeedID: subscribed.ID, ExternalID: &ext,
			URL: fmt.Sprintf("https://a.example/post/%d", i),
		})
		require.NoError(t, err)
	}

	items, total, err := store.ListFeeds(ctx, userID, radar.ListFeedsParams{Limit: 100})
	require.NoError(t, err)
	require.GreaterOrEqual(t, total, 2)

	byID := map[int64]radar.FeedListItem{}
	for _, it := range items {
		byID[it.ID] = it
	}

	require.True(t, byID[subscribed.ID].Subscribed)
	require.Equal(t, 3, byID[subscribed.ID].FindingCount)
	require.False(t, byID[other.ID].Subscribed)
	require.Equal(t, 0, byID[other.ID].FindingCount)
}
```

- [x] **Step 2: Run it and confirm it does not compile**

Run: `go test ./internal/radar/ -run TestStore_ListFeeds_SubscribedAndCounts -count=1`
Expected: FAIL — `too many arguments in call to store.ListFeeds`, `undefined: radar.FeedListItem`.

- [x] **Step 3: Add the DTO to `types.go`**

Replace the existing `FeedList` block with:

```go
// FeedListItem is one catalog row: the feed plus per-user subscription state
// and how many findings it has produced.
type FeedListItem struct {
	Feed
	Subscribed   bool `json:"subscribed"`
	FindingCount int  `json:"finding_count"`
}

// FeedList holds the paginated response for GET /radar/feeds.
type FeedList struct {
	Items []FeedListItem `json:"items"`
	Total int            `json:"total"`
}
```

- [x] **Step 4: Rewrite `Store.ListFeeds`**

```go
func (s *Store) ListFeeds(ctx context.Context, userID int64, p ListFeedsParams) ([]FeedListItem, int, error) {
	var total int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM radar_feeds`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count feeds: %w", err)
	}

	rows, err := s.db.Query(ctx, `
		SELECT f.id, f.url, f.kind, f.title, f.fetch_interval_seconds, f.is_active,
		       f.last_fetched_at, f.last_error, f.created_at,
		       EXISTS (SELECT 1 FROM radar_feed_subscriptions s
		               WHERE s.feed_id = f.id AND s.user_id = $1) AS subscribed,
		       coalesce(fc.n, 0) AS finding_count
		FROM radar_feeds f
		LEFT JOIN (
			SELECT feed_id, count(*) AS n FROM radar_findings GROUP BY feed_id
		) fc ON fc.feed_id = f.id
		ORDER BY lower(coalesce(f.title, f.url)) ASC
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
			&it.Subscribed, &it.FindingCount); err != nil {
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

- [x] **Step 5: Thread userID through the service**

In `internal/radar/service.go`, replace the `StoreAPI` interface line with

```go
	ListFeeds(ctx context.Context, userID int64, p ListFeedsParams) ([]FeedListItem, int, error)
```

and the method itself:

```go
// ListFeeds returns the instance feed catalog with the caller's subscription
// state on every row.
func (s *Service) ListFeeds(ctx context.Context, userID int64, p ListFeedsParams) (*FeedList, error) {
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
	items, total, err := s.store.ListFeeds(ctx, userID, p)
	if err != nil {
		return nil, err
	}
	return &FeedList{Items: items, Total: total}, nil
}
```

- [x] **Step 6: Update mockStore**

In `internal/radar/service_test.go`, replace the `listFeedsResult []radar.Feed` field with `listFeedsResult []radar.FeedListItem` and the method with:

```go
func (m *mockStore) ListFeeds(_ context.Context, userID int64, p radar.ListFeedsParams) ([]radar.FeedListItem, int, error) {
	m.listFeedsUserID = userID
	m.listFeedsParams = p
	return m.listFeedsResult, m.listFeedsTotal, m.listFeedsErr
}
```

Add the `listFeedsUserID int64` and `listFeedsParams radar.ListFeedsParams` fields to the `mockStore` struct.

- [x] **Step 7: Update the handler**

In `internal/radar/http.go`, inside `listFeeds`, add the user before the service call:

```go
	userID := coreauth.UserID(r.Context())
	result, err := h.svc.ListFeeds(r.Context(), userID, params)
```

- [x] **Step 8: A handler test for passing userID through**

In `internal/radar/http_test.go`:

```go
func TestHTTP_ListFeeds_PassesCallerID(t *testing.T) {
	store := newMockStore()
	store.listFeedsResult = []radar.FeedListItem{{
		Feed:       radar.Feed{ID: 7, URL: "https://x.example/rss", Kind: "rss"},
		Subscribed: true, FindingCount: 12,
	}}
	store.listFeedsTotal = 1
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodGet, "/radar/feeds?limit=10", nil)
	req = req.WithContext(userOnlyContext(req.Context(), 42, false))
	rec := httptest.NewRecorder()
	h.ListFeedsHandler()(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, int64(42), store.listFeedsUserID)

	var got radar.FeedList
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.True(t, got.Items[0].Subscribed)
	require.Equal(t, 12, got.Items[0].FindingCount)
}
```

Adjust the existing `TestHTTP_ListFeeds_*` in the file to the new type if it builds `[]radar.Feed`.

- [x] **Step 9: Open the route to every role**

In `internal/server/server.go`, move `r.Get("/feeds", radarHTTP.ListFeedsHandler())` out of the `RequireAdmin` group and into the shared user group (next to `r.Post("/subscriptions", …)`). Only `r.Post("/feeds", …)` stays in the admin group.

- [x] **Step 10: Run the tests**

Run: `make test-unit && go test ./internal/radar/ -run 'TestStore_ListFeeds|TestHTTP_ListFeeds' -count=1`
Expected: PASS.

- [x] **Step 11: Commit**

```bash
git add internal/radar internal/server/server.go
git commit -m "feat(radar): expose feed catalog with per-user subscription state

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 2: Unsubscribing from a feed

**Files:**
- Modify: `internal/radar/store.go` (next to `Subscribe`, ~line 93)
- Modify: `internal/radar/service.go` (`StoreAPI`, next to `Subscribe`, ~line 153)
- Modify: `internal/radar/http.go` (next to `subscribe`, ~line 112)
- Modify: `internal/server/server.go`
- Test: `internal/radar/store_test.go`, `internal/radar/service_test.go`, `internal/radar/http_test.go`, `internal/radar/integration_test.go`

**Interfaces:**
- Consumes: `Store.Subscribe` from Task 1, unchanged.
- Produces: `Store.Unsubscribe(ctx, userID, feedID int64) error`; `Service.Unsubscribe(ctx, userID, feedID int64) error`; `HTTP.UnsubscribeHandler() http.HandlerFunc` for `DELETE /radar/subscriptions/{feedId}`.

- [x] **Step 1: Write the failing store test**

```go
func TestStore_Unsubscribe(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)
	feed, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: fmt.Sprintf("https://unsub.example/%d.xml", userID), Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)
	_, err = store.Subscribe(ctx, userID, feed.ID)
	require.NoError(t, err)

	require.NoError(t, store.Unsubscribe(ctx, userID, feed.ID))

	items, _, err := store.ListFeeds(ctx, userID, radar.ListFeedsParams{Limit: 100})
	require.NoError(t, err)
	for _, it := range items {
		if it.ID == feed.ID {
			require.False(t, it.Subscribed)
		}
	}

	// Idempotent: a second call is not an error.
	require.NoError(t, store.Unsubscribe(ctx, userID, feed.ID))
}
```

- [x] **Step 2: Run it and confirm it fails**

Run: `go test ./internal/radar/ -run TestStore_Unsubscribe -count=1`
Expected: FAIL — `store.Unsubscribe undefined`.

- [x] **Step 3: Implement the store**

```go
// Unsubscribe drops the user's subscription. Removing one that is not there is
// not an error: the endpoint is idempotent.
func (s *Store) Unsubscribe(ctx context.Context, userID, feedID int64) error {
	if _, err := s.db.Exec(ctx,
		`DELETE FROM radar_feed_subscriptions WHERE user_id = $1 AND feed_id = $2`,
		userID, feedID); err != nil {
		return fmt.Errorf("unsubscribe: %w", err)
	}
	return nil
}
```

- [x] **Step 4: The service and mockStore**

Add `Unsubscribe(ctx context.Context, userID, feedID int64) error` to `StoreAPI`, and to the service:

```go
func (s *Service) Unsubscribe(ctx context.Context, userID, feedID int64) error {
	if feedID <= 0 {
		return fmt.Errorf("%w: feed_id must be positive", ErrInvalidInput)
	}
	return s.store.Unsubscribe(ctx, userID, feedID)
}
```

In `mockStore`:

```go
func (m *mockStore) Unsubscribe(_ context.Context, userID, feedID int64) error {
	if m.unsubscribeErr != nil {
		return m.unsubscribeErr
	}
	delete(m.subs, keyOf(userID, feedID))
	return nil
}
```

plus an `unsubscribeErr error` field on the struct.

- [x] **Step 5: The handler**

In `internal/radar/http.go`:

```go
// UnsubscribeHandler returns the http.HandlerFunc for
// DELETE /radar/subscriptions/{feedId}.
func (h *HTTP) UnsubscribeHandler() http.HandlerFunc { return h.unsubscribe }

func (h *HTTP) unsubscribe(w http.ResponseWriter, r *http.Request) {
	feedID, err := strconv.ParseInt(chi.URLParam(r, "feedId"), 10, 64)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid feed id")
		return
	}

	userID := coreauth.UserID(r.Context())

	if err := h.svc.Unsubscribe(r.Context(), userID, feedID); err != nil {
		writeRadarError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
```

- [x] **Step 6: A handler test for idempotency**

```go
func TestHTTP_Unsubscribe_204Twice(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	call := func() int {
		req := httptest.NewRequest(http.MethodDelete, "/radar/subscriptions/5", nil)
		ctx := userOnlyContext(req.Context(), 1, false)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("feedId", "5")
		req = req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
		rec := httptest.NewRecorder()
		h.UnsubscribeHandler()(rec, req)
		return rec.Code
	}

	require.Equal(t, http.StatusNoContent, call())
	require.Equal(t, http.StatusNoContent, call())
}
```

- [x] **Step 7: The route**

In `internal/server/server.go`, in the user group next to `r.Post("/subscriptions", …)`:

```go
			r.Delete("/subscriptions/{feedId}", radarHTTP.UnsubscribeHandler())
```

- [x] **Step 8: An integration test — old matches survive, new ones stop**

In `internal/radar/integration_test.go`:

```go
func TestIntegrationUnsubscribeStopsNewMatchesOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := radar.NewStore(pool)
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)
	ctx := context.Background()

	userID := seedRadarUser(t, pool, false)
	topic, err := svc.CreateTopic(ctx, userID, radar.CreateTopicRequest{
		Name: "Rust", Description: "rust language news and releases",
	})
	require.NoError(t, err)

	feed, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: "https://feed.example/unsub.xml", Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)
	_, err = store.Subscribe(ctx, userID, feed.ID)
	require.NoError(t, err)

	// A finding discovered while subscribed produces a match.
	before := matchFinding(t, ctx, store, emb, feed.ID, "before")
	require.Equal(t, 1, countMatches(t, pool, topic.ID))

	require.NoError(t, svc.Unsubscribe(ctx, userID, feed.ID))

	// A finding discovered after unsubscribing produces none…
	_ = matchFinding(t, ctx, store, emb, feed.ID, "after")
	require.Equal(t, 1, countMatches(t, pool, topic.ID))

	// …and the earlier match is untouched.
	var stillThere bool
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM radar_topic_matches WHERE finding_id = $1)`,
		before).Scan(&stillThere))
	require.True(t, stillThere)
}

// matchFinding upserts a finding, embeds it, runs the matcher, returns its id.
func matchFinding(t *testing.T, ctx context.Context, store *radar.Store,
	emb *embeddings.FakeEmbedder, feedID int64, ext string) int64 {
	t.Helper()
	title := "Rust 2.0 released"
	f, _, err := store.UpsertFinding(ctx, radar.FindingUpsert{
		FeedID: feedID, ExternalID: &ext,
		URL: "https://feed.example/" + ext, Title: &title,
	})
	require.NoError(t, err)

	vecs, err := emb.Embed(ctx, []string{title})
	require.NoError(t, err)
	require.NoError(t, store.UpdateFindingEmbedding(ctx, f.ID, pgvector.NewVector(vecs[0])))
	_, err = store.MatchFindingToTopics(ctx, f.ID)
	require.NoError(t, err)
	return f.ID
}

func countMatches(t *testing.T, pool *pgxpool.Pool, topicID int64) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM radar_topic_matches WHERE topic_id = $1`, topicID).Scan(&n))
	return n
}
```

Check the `embeddings.FakeEmbedder.Embed` signature against its existing use in `internal/radar/jobs/jobs_test.go` and adjust the call if it differs.

- [x] **Step 9: Run the tests**

Run: `make test-unit && go test ./internal/radar/ -run 'Unsubscribe' -count=1`
Expected: PASS.

- [x] **Step 10: Commit**

```bash
git add internal/radar internal/server/server.go
git commit -m "feat(radar): add unsubscribe endpoint

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 3: The admin feed edit (PATCH)

**Files:**
- Modify: `internal/radar/types.go`, `internal/radar/store.go`, `internal/radar/service.go`, `internal/radar/http.go`, `internal/server/server.go`
- Test: `internal/radar/store_test.go`, `internal/radar/service_test.go`, `internal/radar/http_test.go`

**Interfaces:**
- Consumes: `Feed`, `ErrNotFound`, `ErrInvalidInput`.
- Produces: `UpdateFeedRequest{Title *string; FetchIntervalSeconds *int; IsActive *bool}`; `UpdateFeedParams` with the same fields; `Store.UpdateFeed(ctx, feedID int64, p UpdateFeedParams) (*Feed, error)`; `Service.UpdateFeed(ctx, feedID int64, req UpdateFeedRequest) (*Feed, error)`; `HTTP.UpdateFeedHandler()`.

- [x] **Step 1: A service test for validation**

In `internal/radar/service_test.go`:

```go
func TestService_UpdateFeed_Validation(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	ctx := context.Background()

	_, err := svc.UpdateFeed(ctx, 1, radar.UpdateFeedRequest{})
	require.ErrorIs(t, err, radar.ErrInvalidInput)

	tooFast := 60
	_, err = svc.UpdateFeed(ctx, 1, radar.UpdateFeedRequest{FetchIntervalSeconds: &tooFast})
	require.ErrorIs(t, err, radar.ErrInvalidInput)

	tooSlow := 999999
	_, err = svc.UpdateFeed(ctx, 1, radar.UpdateFeedRequest{FetchIntervalSeconds: &tooSlow})
	require.ErrorIs(t, err, radar.ErrInvalidInput)

	ok := 1800
	paused := false
	_, err = svc.UpdateFeed(ctx, 1, radar.UpdateFeedRequest{
		FetchIntervalSeconds: &ok, IsActive: &paused,
	})
	require.NoError(t, err)
	require.Equal(t, 1800, *store.updateFeedParams.FetchIntervalSeconds)
	require.False(t, *store.updateFeedParams.IsActive)
}
```

- [x] **Step 2: Run it and confirm it fails**

Run: `go test ./internal/radar/ -run TestService_UpdateFeed_Validation -count=1`
Expected: FAIL — `undefined: radar.UpdateFeedRequest`.

- [x] **Step 3: The DTOs in `types.go`**

```go
// UpdateFeedRequest is the payload for PATCH /radar/feeds/{id} (admin).
// All fields are optional; only non-nil fields are updated. An empty title
// clears the manual override and lets the crawler fill it in again.
type UpdateFeedRequest struct {
	Title                *string `json:"title,omitempty"`
	FetchIntervalSeconds *int    `json:"fetch_interval_seconds,omitempty"`
	IsActive             *bool   `json:"is_active,omitempty"`
}

// UpdateFeedParams is the store-level analogue of UpdateFeedRequest.
type UpdateFeedParams struct {
	Title                *string
	FetchIntervalSeconds *int
	IsActive             *bool
}
```

- [x] **Step 4: Extract the interval validation and write the service**

In `internal/radar/service.go`, add the helper and replace the check inside `AddFeed` with it:

```go
func validateFetchInterval(seconds int) error {
	if seconds < minFetchIntervalSeconds || seconds > maxFetchIntervalSeconds {
		return fmt.Errorf("%w: fetch_interval_seconds must be %d..%d",
			ErrInvalidInput, minFetchIntervalSeconds, maxFetchIntervalSeconds)
	}
	return nil
}

// UpdateFeed patches a catalog feed. Admin scope; middleware enforces.
func (s *Service) UpdateFeed(ctx context.Context, feedID int64, req UpdateFeedRequest) (*Feed, error) {
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

	return s.store.UpdateFeed(ctx, feedID, UpdateFeedParams{
		Title:                req.Title,
		FetchIntervalSeconds: req.FetchIntervalSeconds,
		IsActive:             req.IsActive,
	})
}
```

Add `UpdateFeed(ctx context.Context, feedID int64, p UpdateFeedParams) (*Feed, error)` to `StoreAPI`.

- [x] **Step 5: mockStore**

```go
func (m *mockStore) UpdateFeed(_ context.Context, feedID int64, p radar.UpdateFeedParams) (*radar.Feed, error) {
	m.updateFeedCalled = true
	m.updateFeedParams = p
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
```

plus the `updateFeedCalled bool`, `updateFeedParams radar.UpdateFeedParams`, and `updateFeedErr error` fields.

- [x] **Step 6: The store**

```go
// UpdateFeed applies a partial patch. An empty title clears the column so the
// crawler can fill it from the channel again.
func (s *Store) UpdateFeed(ctx context.Context, feedID int64, p UpdateFeedParams) (*Feed, error) {
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

	args = append(args, feedID)
	query := fmt.Sprintf(`
		UPDATE radar_feeds SET %s
		WHERE id = $%d
		RETURNING id, url, kind, title, fetch_interval_seconds, is_active,
		          last_fetched_at, last_error, created_at`,
		strings.Join(setClauses, ", "), argIdx)

	var f Feed
	if err := s.db.QueryRow(ctx, query, args...).Scan(&f.ID, &f.URL, &f.Kind, &f.Title,
		&f.FetchIntervalSeconds, &f.IsActive,
		&f.LastFetchedAt, &f.LastError, &f.CreatedAt); err != nil {
		// wrapPgError already turns pgx.ErrNoRows into ErrNotFound.
		return nil, wrapPgError(err)
	}
	return &f, nil
}
```

- [x] **Step 7: The store test**

```go
func TestStore_UpdateFeed_PartialAndClearTitle(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	feed, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: fmt.Sprintf("https://patch.example/%d.xml", time.Now().UnixNano()),
		Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)

	title := "The Verge"
	updated, err := store.UpdateFeed(ctx, feed.ID, radar.UpdateFeedParams{Title: &title})
	require.NoError(t, err)
	require.Equal(t, "The Verge", *updated.Title)
	require.Equal(t, 3600, updated.FetchIntervalSeconds, "untouched field keeps its value")

	empty := ""
	cleared, err := store.UpdateFeed(ctx, feed.ID, radar.UpdateFeedParams{Title: &empty})
	require.NoError(t, err)
	require.Nil(t, cleared.Title)

	_, err = store.UpdateFeed(ctx, 999999, radar.UpdateFeedParams{Title: &title})
	require.ErrorIs(t, err, radar.ErrNotFound)
}
```

- [x] **Step 8: The handler and the route**

```go
// UpdateFeedHandler returns the http.HandlerFunc for PATCH /radar/feeds/{id} (admin).
func (h *HTTP) UpdateFeedHandler() http.HandlerFunc { return h.updateFeed }

func (h *HTTP) updateFeed(w http.ResponseWriter, r *http.Request) {
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

	feed, err := h.svc.UpdateFeed(r.Context(), feedID, req)
	if err != nil {
		writeRadarError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, feed)
}
```

In `server.go`, inside the `RequireAdmin` group: `r.Patch("/feeds/{id}", radarHTTP.UpdateFeedHandler())`.

- [x] **Step 9: A handler test for an empty patch**

```go
func TestHTTP_UpdateFeed_EmptyPatch400(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodPatch, "/radar/feeds/1", strings.NewReader(`{}`))
	req = req.WithContext(withRouteID(userOnlyContext(req.Context(), 1, true), "1"))
	rec := httptest.NewRecorder()
	h.UpdateFeedHandler()(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.False(t, store.updateFeedCalled)
}
```

- [x] **Step 10: Run the tests and commit**

Run: `make test-unit && go test ./internal/radar/ -run 'UpdateFeed' -count=1`
Expected: PASS.

```bash
git add internal/radar internal/server/server.go
git commit -m "feat(radar): let admins patch catalog feeds

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 4: The admin feed delete (DELETE)

**Files:**
- Modify: `internal/radar/store.go`, `internal/radar/service.go`, `internal/radar/http.go`, `internal/server/server.go`
- Test: `internal/radar/http_test.go`, `internal/radar/integration_test.go`

**Interfaces:**
- Consumes: `ErrNotFound`.
- Produces: `Store.DeleteFeed(ctx, feedID int64) error`; `Service.DeleteFeed(ctx, feedID int64) error`; `HTTP.DeleteFeedHandler()`.

- [x] **Step 1: An integration test for the cascade**

```go
func TestIntegrationDeleteFeedCascades(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := radar.NewStore(pool)
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)
	ctx := context.Background()

	userID := seedRadarUser(t, pool, true)
	topic, err := svc.CreateTopic(ctx, userID, radar.CreateTopicRequest{
		Name: "Rust", Description: "rust language news and releases",
	})
	require.NoError(t, err)

	feed, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: "https://feed.example/cascade.xml", Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)
	_, err = store.Subscribe(ctx, userID, feed.ID)
	require.NoError(t, err)

	findingID := matchFinding(t, ctx, store, emb, feed.ID, "cascade-1")
	require.Equal(t, 1, countMatches(t, pool, topic.ID))

	require.NoError(t, svc.DeleteFeed(ctx, feed.ID))

	require.Equal(t, 0, countMatches(t, pool, topic.ID))

	var findings int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM radar_findings WHERE id = $1`, findingID).Scan(&findings))
	require.Equal(t, 0, findings)

	require.ErrorIs(t, svc.DeleteFeed(ctx, feed.ID), radar.ErrNotFound)
}
```

- [x] **Step 2: Run it and confirm it fails**

Run: `go test ./internal/radar/ -run TestIntegrationDeleteFeedCascades -count=1`
Expected: FAIL — `svc.DeleteFeed undefined`.

- [x] **Step 3: The store**

```go
// DeleteFeed removes a feed. Findings and their matches go with it via
// ON DELETE CASCADE, for every user on the instance.
func (s *Store) DeleteFeed(ctx context.Context, feedID int64) error {
	cmd, err := s.db.Exec(ctx, `DELETE FROM radar_feeds WHERE id = $1`, feedID)
	if err != nil {
		return fmt.Errorf("delete feed: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
```

- [x] **Step 4: The service, StoreAPI, and mockStore**

```go
// DeleteFeed removes a feed from the catalog. Admin scope; middleware enforces.
func (s *Service) DeleteFeed(ctx context.Context, feedID int64) error {
	if feedID <= 0 {
		return fmt.Errorf("%w: feed id must be positive", ErrInvalidInput)
	}
	return s.store.DeleteFeed(ctx, feedID)
}
```

In `StoreAPI`: `DeleteFeed(ctx context.Context, feedID int64) error`. In `mockStore`:

```go
func (m *mockStore) DeleteFeed(_ context.Context, feedID int64) error {
	m.deleteFeedCalled = true
	if m.deleteFeedErr != nil {
		return m.deleteFeedErr
	}
	f, ok := m.feeds[feedID]
	if !ok {
		return radar.ErrNotFound
	}
	delete(m.feedsByURL, f.URL)
	delete(m.feeds, feedID)
	return nil
}
```

plus the `deleteFeedCalled bool` and `deleteFeedErr error` fields.

- [x] **Step 5: The handler and the route**

```go
// DeleteFeedHandler returns the http.HandlerFunc for DELETE /radar/feeds/{id} (admin).
func (h *HTTP) DeleteFeedHandler() http.HandlerFunc { return h.deleteFeed }

func (h *HTTP) deleteFeed(w http.ResponseWriter, r *http.Request) {
	feedID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid feed id")
		return
	}

	if err := h.svc.DeleteFeed(r.Context(), feedID); err != nil {
		writeRadarError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
```

In `server.go`, inside the `RequireAdmin` group: `r.Delete("/feeds/{id}", radarHTTP.DeleteFeedHandler())`.

- [x] **Step 6: A handler test for the 404**

```go
func TestHTTP_DeleteFeed_404(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodDelete, "/radar/feeds/77", nil)
	req = req.WithContext(withRouteID(userOnlyContext(req.Context(), 1, true), "77"))
	rec := httptest.NewRecorder()
	h.DeleteFeedHandler()(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}
```

- [x] **Step 7: Run the tests and commit** — the tests were run and are green; the commit was deferred by request.

Run: `make test-unit && go test ./internal/radar/ -run 'DeleteFeed' -count=1`
Expected: PASS.

```bash
git add internal/radar internal/server/server.go
git commit -m "feat(radar): let admins delete catalog feeds

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 5: Auto-subscribing a new user

**Files:**
- Modify: `internal/radar/store.go`, `internal/radar/service.go`
- Modify: `internal/auth/service.go:30-33` (`ServiceConfig`), `internal/auth/service.go:47-77` (`Register`)
- Modify: `internal/server/server.go:47-51` (constructing `authSvc`) and the `cfg.RadarEnabled` branch
- Test: `internal/auth/service_test.go`, `internal/radar/store_test.go`

**Interfaces:**
- Consumes: `Store.Subscribe` (Task 1), `Store.AddFeed`.
- Produces: `Store.SeedSubscriptions(ctx, userID int64) (int, error)`; `Service.SeedSubscriptions(ctx, userID int64) error`; the `auth.ServiceConfig.OnUserCreated func(ctx context.Context, userID int64)` field.

**Wiring order:** `authSvc` is created before `radarSvc`. Rather than reordering the blocks, `server.go` declares a mutable hook variable that the closure reads at call time:

```go
	var onUserCreated func(ctx context.Context, userID int64)
	authSvc := auth.NewService(authStore, issuer, auth.ServiceConfig{
		RefreshTTL:          cfg.JWTRefreshTTL,
		RegistrationEnabled: cfg.RegistrationEnabled,
		OnUserCreated: func(ctx context.Context, userID int64) {
			if onUserCreated != nil {
				onUserCreated(ctx, userID)
			}
		},
	})
```

and inside `if cfg.RadarEnabled && deps.Radar != nil` the variable gets its real value.

- [x] **Step 1: An auth test for the hook being called**

In `internal/auth/service_test.go`:

```go
func TestRegister_CallsOnUserCreated(t *testing.T) {
	store := newMockStore()
	var got int64
	svc := auth.NewService(store, coreauth.NewJWTIssuer("secret", time.Minute), auth.ServiceConfig{
		RefreshTTL:          time.Hour,
		RegistrationEnabled: true,
		OnUserCreated:       func(_ context.Context, userID int64) { got = userID },
	})

	res, err := svc.Register(context.Background(), auth.RegisterRequest{
		Email: "hook@example.com", Password: "password123", DisplayName: "Hook",
	})
	require.NoError(t, err)
	require.Equal(t, res.User.ID, got)
}

func TestRegister_NilHookIsFine(t *testing.T) {
	store := newMockStore()
	svc := auth.NewService(store, coreauth.NewJWTIssuer("secret", time.Minute), auth.ServiceConfig{
		RefreshTTL:          time.Hour,
		RegistrationEnabled: true,
	})

	_, err := svc.Register(context.Background(), auth.RegisterRequest{
		Email: "nohook@example.com", Password: "password123", DisplayName: "NoHook",
	})
	require.NoError(t, err)
}
```

Check the `coreauth.NewJWTIssuer` signature and the `auth.RegisterRequest` fields against the existing tests in the file and adjust.

- [x] **Step 2: Run it and confirm it fails**

Run: `go test ./internal/auth/ -run TestRegister_ -count=1`
Expected: FAIL — `unknown field OnUserCreated in struct literal`.

- [x] **Step 3: The hook in auth**

In `ServiceConfig`:

```go
type ServiceConfig struct {
	RefreshTTL          time.Duration
	RegistrationEnabled bool

	// OnUserCreated runs right after a user row is created. It is best-effort
	// bookkeeping for other modules (Radar seeds feed subscriptions here), so
	// it returns nothing: the caller owns its error policy and registration
	// must not fail because a side module is unhappy.
	OnUserCreated func(ctx context.Context, userID int64)
}
```

In `Register`, right after a successful `s.store.CreateUser`:

```go
	if s.cfg.OnUserCreated != nil {
		s.cfg.OnUserCreated(ctx, user.ID)
	}
```

- [x] **Step 4: A store test for seeding**

In `internal/radar/store_test.go`:

```go
func TestStore_SeedSubscriptions_ActiveOnlyAndIdempotent(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	stamp := time.Now().UnixNano()
	active, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: fmt.Sprintf("https://seed-a.example/%d.xml", stamp), Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)
	paused, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: fmt.Sprintf("https://seed-b.example/%d.xml", stamp), Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)
	off := false
	_, err = store.UpdateFeed(ctx, paused.ID, radar.UpdateFeedParams{IsActive: &off})
	require.NoError(t, err)

	userID := seedUser(t, pool)
	n, err := store.SeedSubscriptions(ctx, userID)
	require.NoError(t, err)
	require.GreaterOrEqual(t, n, 1)

	items, _, err := store.ListFeeds(ctx, userID, radar.ListFeedsParams{Limit: 100})
	require.NoError(t, err)
	byID := map[int64]radar.FeedListItem{}
	for _, it := range items {
		byID[it.ID] = it
	}
	require.True(t, byID[active.ID].Subscribed)
	require.False(t, byID[paused.ID].Subscribed)

	again, err := store.SeedSubscriptions(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, 0, again, "second seeding inserts nothing")
}
```

- [x] **Step 5: The store and the service**

```go
// SeedSubscriptions subscribes a fresh user to every active catalog feed and
// returns how many subscriptions were inserted. Re-running it inserts nothing.
func (s *Store) SeedSubscriptions(ctx context.Context, userID int64) (int, error) {
	cmd, err := s.db.Exec(ctx, `
		INSERT INTO radar_feed_subscriptions (user_id, feed_id)
		SELECT $1, id FROM radar_feeds WHERE is_active
		ON CONFLICT DO NOTHING`, userID)
	if err != nil {
		return 0, fmt.Errorf("seed subscriptions: %w", err)
	}
	return int(cmd.RowsAffected()), nil
}
```

In `StoreAPI`: `SeedSubscriptions(ctx context.Context, userID int64) (int, error)`. In the service:

```go
// SeedSubscriptions is called from the auth module's OnUserCreated hook.
func (s *Service) SeedSubscriptions(ctx context.Context, userID int64) error {
	_, err := s.store.SeedSubscriptions(ctx, userID)
	return err
}
```

In `mockStore`:

```go
func (m *mockStore) SeedSubscriptions(_ context.Context, userID int64) (int, error) {
	if m.seedErr != nil {
		return 0, m.seedErr
	}
	n := 0
	for id, f := range m.feeds {
		if !f.IsActive {
			continue
		}
		key := keyOf(userID, id)
		if _, ok := m.subs[key]; ok {
			continue
		}
		m.subs[key] = &radar.Subscription{UserID: userID, FeedID: id, CreatedAt: time.Now()}
		n++
	}
	return n, nil
}
```

plus a `seedErr error` field.

- [x] **Step 6: Wiring in server.go**

Replace the creation of `authSvc` with the hook-variable version (see the block in this task's header), and inside the `if cfg.RadarEnabled && deps.Radar != nil` branch, right after `radarSvc := radar.NewService(...)`:

```go
		onUserCreated = func(ctx context.Context, userID int64) {
			if err := radarSvc.SeedSubscriptions(ctx, userID); err != nil {
				logger.Error("seed radar subscriptions", "user_id", userID, "error", err)
			}
		}
```

Add `"context"` to the imports in `server.go`.

- [x] **Step 7: Run the tests**

Run: `make test-unit && go test ./internal/auth/ ./internal/radar/ ./internal/server/ -count=1`
Expected: PASS.

- [x] **Step 8: Commit**

```bash
git add internal/auth internal/radar internal/server
git commit -m "feat(radar): subscribe new users to the active catalog

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 6: The channel title from the crawler

**Files:**
- Modify: `internal/radar/crawler/crawler.go:86-95` (`Parse`)
- Modify: `internal/radar/store.go:152-160` (`MarkFeedFetched`)
- Modify: `internal/radar/jobs/crawl_feed.go:46-74`
- Test: `internal/radar/crawler/crawler_test.go`, `internal/radar/store_test.go`

**Interfaces:**
- Consumes: `Store.UpdateFeed` (Task 3) — in the tests, to check that the manual name wins.
- Produces: `crawler.ParsedFeed{Title string; Items []*gofeed.Item}`; `crawler.Parse(body []byte) (*ParsedFeed, error)`; `Store.MarkFeedFetched(ctx, feedID int64, etag, lastModified, title *string) error`.

- [x] **Step 1: The parser test**

In `internal/radar/crawler/crawler_test.go`:

```go
func TestParse_ReturnsChannelTitle(t *testing.T) {
	rss := []byte(`<?xml version="1.0"?><rss version="2.0"><channel>
	  <title>  The Verge  </title>
	  <item><title>Post</title><link>https://example.com/1</link></item>
	</channel></rss>`)

	got, err := crawler.Parse(rss)
	require.NoError(t, err)
	require.Equal(t, "The Verge", got.Title)
	require.Len(t, got.Items, 1)
}
```

- [x] **Step 2: Run it and confirm it fails**

Run: `go test ./internal/radar/crawler/ -run TestParse_ReturnsChannelTitle -count=1`
Expected: FAIL — `got.Title undefined (type []*gofeed.Item has no field Title)`.

- [x] **Step 3: The new shape of Parse**

```go
// ParsedFeed is what one fetched document yields: the channel's own title and
// its entries.
type ParsedFeed struct {
	Title string
	Items []*gofeed.Item
}

func Parse(body []byte) (*ParsedFeed, error) {
	parser := gofeed.NewParser()

	feed, err := parser.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("parse feed: %w", err)
	}

	return &ParsedFeed{Title: strings.TrimSpace(feed.Title), Items: feed.Items}, nil
}
```

Fix the existing `crawler.Parse` calls in `crawler_test.go` (`TestParse_RSS`, `TestToUpserts_*`) — they now use `parsed.Items`.

- [x] **Step 4: MarkFeedFetched writes the title only when it is empty**

```go
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
```

- [x] **Step 5: A store test for the manual name winning**

```go
func TestStore_MarkFeedFetched_TitleFillsOnlyWhenEmpty(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	feed, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: fmt.Sprintf("https://title.example/%d.xml", time.Now().UnixNano()),
		Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)

	auto := "Auto Title"
	require.NoError(t, store.MarkFeedFetched(ctx, feed.ID, nil, nil, &auto))

	items, _, err := store.ListFeeds(ctx, 0, radar.ListFeedsParams{Limit: 100})
	require.NoError(t, err)
	require.Equal(t, "Auto Title", *findFeed(t, items, feed.ID).Title)

	manual := "Manual Title"
	_, err = store.UpdateFeed(ctx, feed.ID, radar.UpdateFeedParams{Title: &manual})
	require.NoError(t, err)

	other := "Auto Again"
	require.NoError(t, store.MarkFeedFetched(ctx, feed.ID, nil, nil, &other))

	items, _, err = store.ListFeeds(ctx, 0, radar.ListFeedsParams{Limit: 100})
	require.NoError(t, err)
	require.Equal(t, "Manual Title", *findFeed(t, items, feed.ID).Title)
}

func findFeed(t *testing.T, items []radar.FeedListItem, id int64) radar.FeedListItem {
	t.Helper()
	for _, it := range items {
		if it.ID == id {
			return it
		}
	}
	t.Fatalf("feed %d not in catalog", id)
	return radar.FeedListItem{}
}
```

- [x] **Step 6: Pass the title through in the job**

In `internal/radar/jobs/crawl_feed.go`:

```go
	if res.NotModified {
		return w.store.MarkFeedFetched(ctx, feedID, ptrOrNil(res.Etag), ptrOrNil(res.LastModified), nil)
	}

	parsed, err := crawler.Parse(res.Body)
	if err != nil {
		_ = w.store.MarkFeedError(ctx, feedID, err.Error())
		return fmt.Errorf("parse feed %d: %w", feedID, err)
	}

	for _, up := range crawler.ToUpserts(feedID, parsed.Items) {
```

and the final line:

```go
	return w.store.MarkFeedFetched(ctx, feedID,
		ptrOrNil(res.Etag), ptrOrNil(res.LastModified), ptrOrNil(parsed.Title))
```

- [x] **Step 7: Run the tests and commit** — the tests were run and are green; the commit was deferred by request.

Run: `make test-unit && go test ./internal/radar/... -count=1`
Expected: PASS.

```bash
git add internal/radar
git commit -m "feat(radar): store the feed's channel title on fetch

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 7: Frontend — the data layer

**Files:**
- Modify: `web/src/features/radar/types.ts`, `schemas.ts`, `api.ts`, `use-radar.tsx`, `use-mutations.tsx`
- Test: `web/src/features/radar/api.test.ts`, `web/src/features/radar/use-mutations.test.tsx`

**Interfaces:**
- Consumes: the `GET /radar/feeds` JSON from Task 1, and the mutations from Tasks 2-4.
- Produces: the `FeedListItem` type; `listFeeds(): Promise<FeedListItem[]>`; `subscribeFeed(feedId: number)`, `unsubscribeFeed(feedId: number)`, `addFeed(input: AddFeedInput)`, `updateFeed(id, input: UpdateFeedInput)`, `deleteFeed(id)`; `radarKeys.feeds`; and the `useFeedsQuery`, `useToggleSubscription`, `useAddFeed`, `useUpdateFeed`, `useDeleteFeed` hooks.

- [x] **Step 1: The API-layer test**

In `web/src/features/radar/api.test.ts` (the file is already wired to msw through `@/test/setup`):

```ts
it("maps the feed catalog", async () => {
  server.use(
    http.get("/api/radar/feeds", () =>
      HttpResponse.json({
        items: [
          {
            id: 3, url: "https://theverge.com/rss", kind: "rss", title: "The Verge",
            fetch_interval_seconds: 3600, is_active: true,
            last_fetched_at: "2026-08-15T10:00:00Z", last_error: null,
            created_at: "2026-08-01T10:00:00Z",
            subscribed: true, finding_count: 214,
          },
        ],
        total: 1,
      }),
    ),
  );

  const feeds = await listFeeds();
  expect(feeds[0].title).toBe("The Verge");
  expect(feeds[0].subscribed).toBe(true);
  expect(feeds[0].findingCount).toBe(214);
  expect(feeds[0].lastFetchedAt).toBeInstanceOf(Date);
});

it("sends an empty title to clear it", async () => {
  let body: unknown;
  server.use(
    http.patch("/api/radar/feeds/3", async ({ request }) => {
      body = await request.json();
      return HttpResponse.json({
        id: 3, url: "https://theverge.com/rss", kind: "rss", title: null,
        fetch_interval_seconds: 3600, is_active: true,
        last_fetched_at: null, last_error: null, created_at: "2026-08-01T10:00:00Z",
      });
    }),
  );

  await updateFeed(3, { title: "" });
  expect(body).toEqual({ title: "" });
});
```

- [x] **Step 2: Run it and confirm it fails**

Run: `cd web && npx vitest run src/features/radar/api.test.ts`
Expected: FAIL — `listFeeds is not exported`.

- [x] **Step 3: The type in `types.ts`**

```ts
export type FeedListItem = {
  id: number;
  url: string;
  kind: string;
  title: string | null;
  fetchIntervalSeconds: number;
  isActive: boolean;
  lastFetchedAt: Date | null;
  lastError: string | null;
  createdAt: Date;
  subscribed: boolean;
  findingCount: number;
};
```

- [x] **Step 4: The schemas in `schemas.ts`**

```ts
export const RawFeedSchema = z.object({
  id: z.number().int(),
  url: z.string(),
  kind: z.string(),
  title: z.string().nullable(),
  fetch_interval_seconds: z.number().int(),
  is_active: z.boolean(),
  last_fetched_at: z.string().nullable().optional(),
  last_error: z.string().nullable().optional(),
  created_at: z.string(),
});

export const RawFeedListItemSchema = RawFeedSchema.extend({
  subscribed: z.boolean(),
  finding_count: z.number().int(),
});

export const RawFeedListSchema = z.object({
  items: z.array(RawFeedListItemSchema),
  total: z.number().int(),
});

export function mapFeedListItem(
  raw: z.infer<typeof RawFeedListItemSchema>,
): FeedListItem {
  return {
    id: raw.id,
    url: raw.url,
    kind: raw.kind,
    title: raw.title,
    fetchIntervalSeconds: raw.fetch_interval_seconds,
    isActive: raw.is_active,
    lastFetchedAt: raw.last_fetched_at ? new Date(raw.last_fetched_at) : null,
    lastError: raw.last_error ?? null,
    createdAt: new Date(raw.created_at),
    subscribed: raw.subscribed,
    findingCount: raw.finding_count,
  };
}
```

Import `FeedListItem` from `./types` at the top of the file.

- [x] **Step 5: The functions in `api.ts`**

```ts
export async function listFeeds(): Promise<FeedListItem[]> {
  const raw = await apiFetch<unknown>(`/radar/feeds?limit=100`);
  const parsed = parseInDev(RawFeedListSchema, raw);
  return parsed.items.map(mapFeedListItem);
}

export async function subscribeFeed(feedId: number): Promise<void> {
  await apiFetch<void>(`/radar/subscriptions`, {
    method: "POST",
    body: JSON.stringify({ feed_id: feedId }),
  });
}

export async function unsubscribeFeed(feedId: number): Promise<void> {
  await apiFetch<void>(`/radar/subscriptions/${feedId}`, { method: "DELETE" });
}

export type AddFeedInput = { url: string; fetchIntervalSeconds: number };

export async function addFeed(input: AddFeedInput): Promise<void> {
  await apiFetch<void>(`/radar/feeds`, {
    method: "POST",
    body: JSON.stringify({
      url: input.url,
      fetch_interval_seconds: input.fetchIntervalSeconds,
    }),
  });
}

export type UpdateFeedInput = {
  title?: string;
  fetchIntervalSeconds?: number;
  isActive?: boolean;
};

export async function updateFeed(id: number, input: UpdateFeedInput): Promise<void> {
  const body: Record<string, unknown> = {};
  if (input.title !== undefined) body.title = input.title;
  if (input.fetchIntervalSeconds !== undefined) {
    body.fetch_interval_seconds = input.fetchIntervalSeconds;
  }
  if (input.isActive !== undefined) body.is_active = input.isActive;
  await apiFetch<void>(`/radar/feeds/${id}`, {
    method: "PATCH",
    body: JSON.stringify(body),
  });
}

export async function deleteFeed(id: number): Promise<void> {
  await apiFetch<void>(`/radar/feeds/${id}`, { method: "DELETE" });
}
```

- [x] **Step 6: The query in `use-radar.tsx`**

Add `feeds: ["radar", "feeds"] as const,` to `radarKeys`, plus the hook:

```tsx
export function useFeedsQuery() {
  return useQuery({
    queryKey: radarKeys.feeds,
    queryFn: listFeeds,
  });
}
```

- [x] **Step 7: A test for the optimistic toggle**

In `web/src/features/radar/use-mutations.test.tsx`:

```tsx
const rawFeed = (id: number, subscribed: boolean) => ({
  id, url: `https://f${id}.example/rss`, kind: "rss", title: `Feed ${id}`,
  fetch_interval_seconds: 3600, is_active: true,
  last_fetched_at: null, last_error: null, created_at: "2026-08-01T10:00:00Z",
  subscribed, finding_count: 0,
});

it("rolls the subscription toggle back when the request fails", async () => {
  server.use(
    http.get("/api/radar/feeds", () =>
      HttpResponse.json({ items: [rawFeed(3, false)], total: 1 }),
    ),
    http.post("/api/radar/subscriptions", () =>
      HttpResponse.json({ error: "internal" }, { status: 500 }),
    ),
  );

  const { qc, wrapper } = makeWrapper();
  const { result } = renderHook(
    () => ({ feeds: useFeedsQuery(), toggle: useToggleSubscription() }),
    { wrapper },
  );
  await waitFor(() => expect(result.current.feeds.isSuccess).toBe(true));

  await act(async () => {
    await result.current.toggle
      .mutateAsync({ feedId: 3, subscribed: true })
      .catch(() => undefined);
  });

  const cached = qc.getQueryData<FeedListItem[]>(radarKeys.feeds);
  expect(cached?.[0].subscribed).toBe(false);
});
```

- [x] **Step 8: The mutations in `use-mutations.tsx`**

```tsx
type ToggleArgs = { feedId: number; subscribed: boolean };

export function useToggleSubscription() {
  const qc = useQueryClient();
  return useMutation<void, Error, ToggleArgs, { previous: FeedListItem[] | undefined }>({
    mutationFn: ({ feedId, subscribed }) =>
      subscribed ? subscribeFeed(feedId) : unsubscribeFeed(feedId),
    onMutate: async ({ feedId, subscribed }) => {
      await qc.cancelQueries({ queryKey: radarKeys.feeds });
      const previous = qc.getQueryData<FeedListItem[]>(radarKeys.feeds);
      if (previous) {
        qc.setQueryData<FeedListItem[]>(
          radarKeys.feeds,
          previous.map((f) => (f.id === feedId ? { ...f, subscribed } : f)),
        );
      }
      return { previous };
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.previous !== undefined) {
        qc.setQueryData(radarKeys.feeds, ctx.previous);
      }
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: radarKeys.feeds });
    },
  });
}

export function useAddFeed() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: AddFeedInput) => addFeed(input),
    onSuccess: () => qc.invalidateQueries({ queryKey: radarKeys.feeds }),
  });
}

export function useUpdateFeed() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, input }: { id: number; input: UpdateFeedInput }) =>
      updateFeed(id, input),
    onSuccess: () => qc.invalidateQueries({ queryKey: radarKeys.feeds }),
  });
}

export function useDeleteFeed() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => deleteFeed(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: radarKeys.feeds }),
  });
}
```

Add the `subscribeFeed`, `unsubscribeFeed`, `addFeed`, `updateFeed`, `deleteFeed` imports and the `AddFeedInput`, `UpdateFeedInput`, `FeedListItem` types.

- [x] **Step 9: Run the tests and commit** — tests, typecheck, and lint were run and are green; the commit was deferred by request.

Run: `cd web && npm test && npm run typecheck && npm run lint`
Expected: PASS.

```bash
git add web/src/features/radar
git commit -m "feat(radar): add feed catalog data layer to the web app

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 8: The `/radar/sources` screen

**Files:**
- Create: `web/src/features/radar/components/SourceRow.tsx`, `web/src/features/radar/components/SourceRow.test.tsx`
- Create: `web/src/routes/radar.sources.tsx`, `web/src/routes/radar.sources.test.tsx`
- Modify: `web/src/App.tsx`, `web/src/routes/radar._index.tsx`, `web/src/routes/radar.topics._index.tsx`

**Interfaces:**
- Consumes: `useFeedsQuery`, `useToggleSubscription` (Task 7), `FeedListItem`, `useAuthStore`, `RadarDisabled`, `PageHeader`.
- Produces: the `SourceRow` component with the props `{ feed: FeedListItem; isAdmin: boolean; onToggle: (subscribed: boolean) => void; onEdit: () => void; onDelete: () => void }`; and the `radar/sources` route.

- [x] **Step 1: The catalog row test**

`web/src/features/radar/components/SourceRow.test.tsx`:

```tsx
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SourceRow } from "./SourceRow";
import type { FeedListItem } from "../types";

const feed = (over: Partial<FeedListItem> = {}): FeedListItem => ({
  id: 1,
  url: "https://theverge.com/rss",
  kind: "rss",
  title: "The Verge",
  fetchIntervalSeconds: 3600,
  isActive: true,
  lastFetchedAt: null,
  lastError: null,
  createdAt: new Date("2026-08-01T10:00:00Z"),
  subscribed: false,
  findingCount: 214,
  ...over,
});

describe("SourceRow", () => {
  it("hides admin actions from ordinary users", () => {
    render(
      <SourceRow feed={feed()} isAdmin={false} onToggle={() => {}} onEdit={() => {}} onDelete={() => {}} />,
    );
    expect(screen.queryByRole("button", { name: /edit/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /delete/i })).not.toBeInTheDocument();
  });

  it("toggles the subscription", async () => {
    const onToggle = vi.fn();
    render(
      <SourceRow feed={feed()} isAdmin={false} onToggle={onToggle} onEdit={() => {}} onDelete={() => {}} />,
    );
    await userEvent.click(screen.getByRole("checkbox", { name: /the verge/i }));
    expect(onToggle).toHaveBeenCalledWith(true);
  });

  it("falls back to the hostname and surfaces fetch errors", () => {
    render(
      <SourceRow
        feed={feed({ title: null, lastError: "404 Not Found", lastFetchedAt: new Date() })}
        isAdmin
        onToggle={() => {}}
        onEdit={() => {}}
        onDelete={() => {}}
      />,
    );
    expect(screen.getByText("theverge.com")).toBeInTheDocument();
    expect(screen.getByText(/404 Not Found/)).toBeInTheDocument();
  });

  it("marks a paused feed", () => {
    render(
      <SourceRow feed={feed({ isActive: false })} isAdmin onToggle={() => {}} onEdit={() => {}} onDelete={() => {}} />,
    );
    expect(screen.getByText(/paused/i)).toBeInTheDocument();
  });
});
```

- [x] **Step 2: Run it and confirm it fails**

Run: `cd web && npx vitest run src/features/radar/components/SourceRow.test.tsx`
Expected: FAIL — module `./SourceRow` not found.

- [x] **Step 3: Implement `SourceRow`**

```tsx
import { relativeFromNow } from "@/features/library/time";
import type { FeedListItem } from "../types";

type Props = {
  feed: FeedListItem;
  isAdmin: boolean;
  onToggle: (subscribed: boolean) => void;
  onEdit: () => void;
  onDelete: () => void;
};

function host(url: string): string {
  try {
    return new URL(url).host.replace(/^www\./, "");
  } catch {
    return url;
  }
}

function fmtInterval(seconds: number): string {
  const hours = Math.round(seconds / 3600);
  if (seconds < 3600) return `every ${Math.round(seconds / 60)}m`;
  return hours === 1 ? "every 1h" : `every ${hours}h`;
}

function meta(feed: FeedListItem): string[] {
  const parts = [host(feed.url)];
  if (!feed.isActive) {
    parts.push("paused");
  } else {
    parts.push(fmtInterval(feed.fetchIntervalSeconds));
  }
  if (feed.lastError) {
    parts.push(`⚠ ${feed.lastError}`);
  } else if (feed.lastFetchedAt) {
    parts.push(`fetched ${relativeFromNow(feed.lastFetchedAt)}`);
  } else {
    parts.push("never fetched");
  }
  parts.push(`${feed.findingCount} items`);
  return parts;
}

export function SourceRow({ feed, isAdmin, onToggle, onEdit, onDelete }: Props) {
  const name = feed.title ?? host(feed.url);
  const inputId = `feed-${feed.id}`;

  return (
    <div
      className={`flex items-start justify-between gap-4 py-4 border-b border-rule ${
        feed.isActive ? "" : "opacity-60"
      }`}
    >
      <div className="flex items-start gap-3">
        <input
          id={inputId}
          type="checkbox"
          checked={feed.subscribed}
          onChange={(e) => onToggle(e.target.checked)}
          className="mt-1 h-4 w-4 accent-vermillion"
        />
        <div>
          <label htmlFor={inputId} className="font-display text-xl text-ink cursor-pointer">
            {name}
          </label>
          <p className="label-sc mt-1 text-muted-foreground">{meta(feed).join(" · ")}</p>
        </div>
      </div>

      {isAdmin && (
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

- [x] **Step 4: The screen test**

`web/src/routes/radar.sources.test.tsx` — modelled on `radar._index.test.tsx` (msw + `MemoryRouter`):

```tsx
it("shows a different empty state for admins", async () => {
  server.use(http.get("/api/radar/feeds", () => HttpResponse.json({ items: [], total: 0 })));

  useAuthStore.getState().setSession("t", {
    id: 1, email: "a@example.com", displayName: "A", isAdmin: false,
  });
  renderAt("/radar/sources");
  expect(await screen.findByText(/ask the instance admin/i)).toBeInTheDocument();
});

it("lists the catalog", async () => {
  server.use(
    http.get("/api/radar/feeds", () =>
      HttpResponse.json({ items: [rawFeed(3, true)], total: 1 }),
    ),
  );

  useAuthStore.getState().setSession("t", {
    id: 1, email: "a@example.com", displayName: "A", isAdmin: false,
  });
  renderAt("/radar/sources");
  expect(await screen.findByRole("checkbox", { name: /feed 3/i })).toBeChecked();
});
```

- [x] **Step 5: Implement the route**

`web/src/routes/radar.sources.tsx`:

```tsx
import { PageHeader } from "@/shared/layout/PageHeader";
import { ApiError } from "@/shared/api/errors";
import { useAuthStore } from "@/features/auth/store";
import { useFeedsQuery } from "@/features/radar/use-radar";
import { useToggleSubscription } from "@/features/radar/use-mutations";
import { SourceRow } from "@/features/radar/components/SourceRow";
import { RadarDisabled } from "@/features/radar/components/RadarDisabled";

export default function SourcesRoute() {
  const feeds = useFeedsQuery();
  const toggle = useToggleSubscription();
  const isAdmin = useAuthStore((s) => s.user?.isAdmin ?? false);

  if (feeds.error instanceof ApiError && feeds.error.code === "radar_disabled") {
    return <RadarDisabled />;
  }

  const items = feeds.data ?? [];
  const subscribedCount = items.filter((f) => f.subscribed).length;

  return (
    <div>
      <PageHeader
        title="Sources"
        subtitle={
          items.length
            ? `${items.length} feeds · ${subscribedCount} subscribed · changes apply from the next sweep`
            : "Feeds this instance watches"
        }
      />
      <div className="px-4 lg:px-8 pb-10">
        {feeds.isSuccess && items.length === 0 && (
          <p className="font-body text-muted-foreground pt-8">
            {isAdmin
              ? "No sources yet. Add the first feed to start watching."
              : "No sources yet. Ask the instance admin to add feeds."}
          </p>
        )}
        {items.map((feed) => (
          <SourceRow
            key={feed.id}
            feed={feed}
            isAdmin={isAdmin}
            onToggle={(subscribed) => toggle.mutate({ feedId: feed.id, subscribed })}
            onEdit={() => {}}
            onDelete={() => {}}
          />
        ))}
      </div>
    </div>
  );
}
```

The `onEdit` / `onDelete` stubs are filled in during Task 9.

- [x] **Step 6: Registering the route and the navigation**

In `web/src/App.tsx`, add the `import SourcesRoute from "./routes/radar.sources";` import and the `{ path: "radar/sources", element: <SourcesRoute /> },` entry next to `radar/topics`.

In `web/src/routes/radar._index.tsx` and `web/src/routes/radar.topics._index.tsx`, add a second link to the `PageHeader`'s `actions`:

```tsx
          <Link
            to="/radar/sources"
            className="label-sc text-muted-foreground hover:text-vermillion"
          >
            Sources →
          </Link>
```

- [x] **Step 7: Run the tests and commit** — tests, typecheck, and lint were run and are green; the commit was deferred by request.

Run: `cd web && npm test && npm run typecheck && npm run lint`
Expected: PASS.

```bash
git add web/src
git commit -m "feat(radar): add the sources screen with subscription toggles

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

### Task 9: The admin dialogs

**Files:**
- Create: `web/src/features/radar/components/AddFeedDialog.tsx`, `AddFeedDialog.test.tsx`, `EditFeedDialog.tsx`, `DeleteFeedConfirm.tsx`
- Modify: `web/src/routes/radar.sources.tsx`

**Interfaces:**
- Consumes: `useAddFeed`, `useUpdateFeed`, `useDeleteFeed` (Task 7), `SourceRow` (Task 8).
- Produces: `AddFeedDialog{open, onOpenChange}`; `EditFeedDialog{feed: FeedListItem | null, onOpenChange}`; `DeleteFeedConfirm{feed: FeedListItem | null, pending, onOpenChange, onConfirm}`.

- [x] **Step 1: An add-dialog test for the 409**

`web/src/features/radar/components/AddFeedDialog.test.tsx`:

```tsx
it("shows the duplicate error inline", async () => {
  server.use(
    http.post("/api/radar/feeds", () =>
      HttpResponse.json({ error: { code: "duplicate" } }, { status: 409 }),
    ),
  );

  const { wrapper } = makeWrapper();
  render(<AddFeedDialog open onOpenChange={() => {}} />, { wrapper });

  await userEvent.type(
    screen.getByLabelText(/feed url/i),
    "https://theverge.com/rss",
  );
  await userEvent.click(screen.getByRole("button", { name: /add feed/i }));

  expect(await screen.findByRole("alert")).toHaveTextContent(/already in the catalog/i);
});
```

Check the error response shape against `ApiError` in `web/src/shared/api/errors.ts` and the msw stubs in `use-mutations.test.tsx`, so `err.status === 409` really gets set.

- [x] **Step 2: Run it and confirm it fails**

Run: `cd web && npx vitest run src/features/radar/components/AddFeedDialog.test.tsx`
Expected: FAIL — module `./AddFeedDialog` not found.

- [x] **Step 3: `AddFeedDialog`**

Modelled on `NewTopicDialog.tsx` (react-hook-form + zodResolver + `Dialog`):

```tsx
const schema = z.object({
  url: z.string().url("Enter a valid http(s) URL"),
  fetchIntervalSeconds: z.coerce.number().int(),
});
type FormValues = z.infer<typeof schema>;

export const INTERVAL_OPTIONS = [
  { value: 1800, label: "every 30m" },
  { value: 3600, label: "every 1h" },
  { value: 10800, label: "every 3h" },
  { value: 21600, label: "every 6h" },
  { value: 43200, label: "every 12h" },
  { value: 86400, label: "every 24h" },
];

function mapError(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.status === 409) return "This feed is already in the catalog";
    if (err.status === 400) return err.message || "Invalid input";
    if (err.status === 403) return "Only an instance admin can add feeds";
  }
  return "Could not save — please try again";
}
```

Submit calls `useAddFeed().mutateAsync`; on success it fires `toast.success("Feed added")` and closes, and on error it sets `setTopError(mapError(err))` in a `role="alert"` block (the same markup as in `NewTopicDialog`).

- [x] **Step 4: `EditFeedDialog`**

The same skeleton; the fields are `title` (text, where an empty value is sent as `""` and clears the name), an interval select from `INTERVAL_OPTIONS`, and a `Paused` checkbox (the request carries `isActive: !paused`). The default values come from the `feed` prop. Submit is `useUpdateFeed().mutateAsync({ id: feed.id, input })`.

- [x] **Step 5: `DeleteFeedConfirm`**

A copy of the `DeleteTopicConfirm.tsx` structure with different text:

```tsx
        <AlertDialogTitle className="display-tight text-2xl">
          Delete &ldquo;{name}&rdquo;?
        </AlertDialogTitle>
        <AlertDialogDescription className="font-body text-muted-foreground">
          {feed.findingCount} findings and their matches will be removed for all
          users. This cannot be undone.
        </AlertDialogDescription>
```

- [x] **Step 6: Wire the dialogs into the screen**

In `web/src/routes/radar.sources.tsx`, add local state and replace the stubs:

```tsx
  const [addOpen, setAddOpen] = useState(false);
  const [editing, setEditing] = useState<FeedListItem | null>(null);
  const [deleting, setDeleting] = useState<FeedListItem | null>(null);
  const remove = useDeleteFeed();
```

`PageHeader` gets `actions={isAdmin ? <Button onClick={() => setAddOpen(true)}>Add feed</Button> : undefined}`, the rows get `onEdit={() => setEditing(feed)}` and `onDelete={() => setDeleting(feed)}`, and the three dialogs render below the list. The delete confirmation:

```tsx
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
```

- [x] **Step 7: A screen test for the admin actions**

In `web/src/routes/radar.sources.test.tsx`:

```tsx
it("shows the finding count in the delete confirmation", async () => {
  server.use(
    http.get("/api/radar/feeds", () =>
      HttpResponse.json({ items: [rawFeed(3, true, { finding_count: 214 })], total: 1 }),
    ),
  );

  useAuthStore.getState().setSession("t", {
    id: 1, email: "a@example.com", displayName: "A", isAdmin: true,
  });
  renderAt("/radar/sources");

  await userEvent.click(await screen.findByRole("button", { name: /delete/i }));
  expect(await screen.findByText(/214 findings/i)).toBeInTheDocument();
});
```

Extend the `rawFeed` helper from Task 8's Step 4 with a third override argument.

- [x] **Step 8: Run everything and commit** — tests, typecheck, lint, and test-unit were run and are green; the commit was deferred by request.

Run: `cd web && npm test && npm run typecheck && npm run lint && cd .. && make test-unit`
Expected: PASS.

```bash
git add web/src
git commit -m "feat(radar): add admin feed dialogs to the sources screen

Co-Authored-By: Claude Opus 5 <noreply@anthropic.com>"
```

---

## Final verification

- [x] **The full run:** `make test` (needs Docker) and `cd web && npm test && npm run build`.
- [x] **Manual check:** bring up `make dev-db && make run`, register a second user while the catalog is non-empty — they should get subscriptions to every active feed; on `/radar/sources` untick a box, then add and delete a feed as the admin.
