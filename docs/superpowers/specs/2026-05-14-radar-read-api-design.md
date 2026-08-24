# Radar read API — design

**Date:** 2026-05-14
**Status:** approved, ready for writing-plans
**Scope:** the Radar backend — read endpoints for the upcoming Radar UI. No frontend.

## Context

The crawler, the embedding service, and the matching worker already run (phase
3a). The backend can create topics, subscriptions, and feeds, but there are no
`GET` operations. The frontend foundation, auth, and library are done; the next
step is to build the `/radar` and `/radar/:id` pages, and that UI needs read
endpoints, per-topic aggregates, and denormalized matches.

This spec defines exactly the minimum backend the SPA needs for three screens: a
topic list with metrics, a single-topic view with a feed of matches, and an admin
feed list. After it, the "Radar UI" frontend plan will be a thin layer over the
API.

## Decisions taken during brainstorming

| # | Question | Decision |
|---|---|---|
| 1 | What goes into the next plan | The Radar backend read API, no frontend |
| 2 | The primary entity of the Radar feed | Matches (`radar_topic_matches`), not findings |
| 3 | Pagination style | offset+limit plus `total` (as in Library) |
| 4 | Topics list pagination | Not paginated — a user has dozens of topics |
| 5 | Topic list shape | An enriched `TopicWithStats` with aggregates (`new_count`, `total_count`, `source_count`, `last_match_at`) |
| 6 | Match denormalization | Include `topic_name` and `feed_title` in `MatchView` |
| 7 | Feeds list (admin) | Included in this plan — `GET /radar/feeds` behind admin |
| 8 | Owned-by-another-user: 404 or 403 | 404 — do not leak the existence of an id |
| 9 | Cursor pagination | Deferred; offset+limit is enough for the MVP |

## 1. Endpoints

Everything mounts under `/radar` and requires `RequireUser` (with an admin guard
where noted). The `LINKTHECA_RADAR_ENABLED=false` branch already returns 501 for
all of `/radar/*` through a wildcard in `server.go` — new routes fall under it
automatically.

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `GET` | `/radar/topics` | user | The user's topics with aggregates. Not paginated. |
| `GET` | `/radar/topics/{id}` | user | One topic plus the same aggregates. |
| `PATCH` | `/radar/topics/{id}` | user | Field updates; `description` triggers an embedding recompute. |
| `DELETE` | `/radar/topics/{id}` | user | 204; CASCADE over matches. |
| `GET` | `/radar/matches` | user | `{ items, total }`, query: `topic_id?`, `state?`, `limit`, `offset`. |
| `PATCH` | `/radar/matches/{id}` | user | `{ state }`. |
| `GET` | `/radar/status` | user | `{ last_sweep_at }` — the latest `last_fetched_at` across subscribed feeds. |
| `GET` | `/radar/feeds` | **admin** | `{ items, total }`, query: `limit`, `offset`. |

## 2. Response shapes

All JSON is snake_case. The TS side maps it to camelCase.

### `TopicWithStats`

Returned from `GET /radar/topics` (as an array inside `{ items }`) and from
`GET /radar/topics/{id}` (as a single object).

```json
{
  "id": 12,
  "user_id": 1,
  "name": "Local-first software",
  "description": "Apps that put user data on the user's device …",
  "match_threshold": 0.55,
  "is_active": true,
  "has_embedding": true,
  "created_at": "2026-04-22T10:00:00Z",
  "updated_at": "2026-04-25T09:00:00Z",
  "stats": {
    "new_count": 7,
    "total_count": 41,
    "source_count": 5,
    "last_match_at": "2026-05-14T12:01:00Z"
  }
}
```

- `stats.new_count` — `COUNT(*) WHERE state='new'`
- `stats.total_count` — `COUNT(*)` with no filter
- `stats.source_count` — `COUNT(DISTINCT findings.feed_id)`
- `stats.last_match_at` — `MAX(matched_at)`, nullable

### `MatchView`

Returned from `GET /radar/matches` as `items[]`.

```json
{
  "id": 9001,
  "topic_id": 12,
  "topic_name": "Local-first software",
  "similarity": 0.71,
  "state": "new",
  "matched_at": "2026-05-14T12:01:00Z",
  "finding": {
    "id": 4488,
    "feed_id": 3,
    "feed_title": "Hacker News",
    "url": "https://example.com/article",
    "title": "An article",
    "summary": "…",
    "published_at": "2026-05-14T10:00:00Z",
    "discovered_at": "2026-05-14T11:55:00Z"
  }
}
```

`topic_name` and `feed_title` are denormalized on purpose — the UI cards need
them without an extra round trip. A JOIN over topics and feeds is cheaper than a
second query from the browser.

### `Feed`

Already defined in `internal/radar/types.go`. The shape does not change. List:
`{ items: Feed[], total }`.

### `RadarStatus`

```json
{ "last_sweep_at": "2026-05-14T11:55:00Z" }
```

`last_sweep_at` is `null` when the user has no active subscriptions.

### Update DTOs

**`UpdateTopicRequest`** (for `PATCH /radar/topics/{id}`):

```json
{
  "name": "…",            // optional
  "description": "…",     // optional, a change triggers an embedding recompute
  "match_threshold": 0.6, // optional, validated to [0, 1]
  "is_active": false      // optional
}
```

Every field is nullable; only the ones passed are updated. At least one field is
required (otherwise `bad_request`).

**`UpdateMatchRequest`**:

```json
{ "state": "seen" }
```

`state` ∈ `{"new", "seen"}`, otherwise `bad_request`.

## 3. Errors

We use the existing `writeRadarError`. No additions needed:

- `400 bad_request` — invalid JSON, a missing field in a PATCH, a bad enum, a
  threshold outside `[0,1]`, a `limit` outside `[1,100]`.
- `404 not_found` — the topic or match was not found, or belongs to another user.
- `503 embedder_unavailable` — the topic description changed but the embedder is
  down.
- `500 internal` — the fallback.

A `PATCH` with no fields → `bad_request "no fields to update"`.

## 4. The store layer

We extend `internal/radar/store.go` with these methods:

```go
// Topics
ListTopicsWithStats(ctx, userID int64) ([]TopicWithStats, error)
GetTopicWithStats(ctx, userID, topicID int64) (*TopicWithStats, error)
UpdateTopic(ctx, userID, topicID int64, p UpdateTopicParams) (*Topic, error)
DeleteTopic(ctx, userID, topicID int64) error

// Matches
ListMatches(ctx, userID int64, p ListMatchesParams) ([]MatchView, int /*total*/, error)
UpdateMatchState(ctx, userID, matchID int64, state string) error

// Feeds (admin)
ListFeeds(ctx, p ListFeedsParams) ([]Feed, int, error)

// Status
LastSweepAt(ctx, userID int64) (*time.Time, error)
```

`UpdateTopicParams { Name, Description *string; MatchThreshold *float32; IsActive *bool }` —
all nullable. The store updates only the fields passed; it never touches the
embedding. Re-embedding is done by the service through a separate
`UpdateTopicEmbedding` call (which already exists), see §5.

### Key SQL queries

**`ListTopicsWithStats`**:

```sql
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
WHERE t.user_id = $1
ORDER BY t.is_active DESC, t.created_at DESC;
```

The existing `radar_topic_matches_topic_state_idx (topic_id, state, matched_at DESC)`
index covers the aggregates. No additional indexes are added.

**`ListMatches`** — a JOIN over topics and feeds, with ownership through
`t.user_id`:

```sql
SELECT
  m.id, m.topic_id, t.name AS topic_name,
  m.similarity, m.state, m.matched_at,
  f.id AS finding_id, f.feed_id, fd.title AS feed_title,
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
LIMIT $4 OFFSET $5;
```

`total` comes from a separate `COUNT(*)` query with the same WHERE (the Library
pattern).

**`LastSweepAt`**:

```sql
SELECT MAX(f.last_fetched_at)
FROM radar_feeds f
JOIN radar_feed_subscriptions s ON s.feed_id = f.id
WHERE s.user_id = $1 AND f.is_active;
```

**`UpdateMatchState`** — an UPDATE with an ownership filter:

```sql
UPDATE radar_topic_matches
SET state = $1
WHERE id = $2
  AND topic_id IN (SELECT id FROM radar_topics WHERE user_id = $3)
RETURNING id;
```

No rows affected → `ErrNotFound`.

**`DeleteTopic`**:

```sql
DELETE FROM radar_topics
WHERE id = $1 AND user_id = $2
RETURNING id;
```

The CASCADE takes the matches with it.

**`UpdateTopic`** — assembled dynamically from the non-nil fields passed (the
`library.UpdateItem` pattern). Returns `(*Topic, embeddingDirty, error)`; the
service decides what to do with `embeddingDirty`.

## 5. The service layer

We extend `internal/radar/service.go`:

```go
ListTopics(ctx, userID int64) ([]TopicWithStats, error)
GetTopic(ctx, userID, topicID int64) (*TopicWithStats, error)
UpdateTopic(ctx, userID, topicID int64, req UpdateTopicRequest) (*Topic, error)
DeleteTopic(ctx, userID, topicID int64) error

ListMatches(ctx, userID int64, p ListMatchesParams) (*MatchList, error)
SetMatchState(ctx, userID, matchID int64, state string) error

ListFeeds(ctx, p ListFeedsParams) (*FeedList, error)
LastSweep(ctx, userID int64) (*time.Time, error)
```

### Semantics

**`UpdateTopic`:**
- Validation: the same rules as in `CreateTopic` for each field passed — `name`
  1..200, `description` 10..2000, `match_threshold` in `[0,1]`. At least one
  field must be in the patch, otherwise `ErrInvalidInput "no fields to update"`.
- The flow (not atomic, mirroring `CreateTopic`):
  1. `store.UpdateTopic` — writes the fields passed.
  2. If `description` was in the patch →
     `embedder.Embed(name + ": " + description)` → `store.UpdateTopicEmbedding`.
  3. If the embedder is down, return `ErrEmbedderUnavailable`. The fields are
     already updated and the embedding is stale. That matches `CreateTopic`'s
     behaviour (a topic is left without a current embedding on failure there
     too); the frontend can retry the PATCH with the same fields. Idempotent.
- The "an embed is needed" decision is made from the presence of `description` in
  the patch — we do not compare against the old value. A wasted recompute when
  `description = "the same text"` is a rare case and not worth an extra round
  trip to the database.
- `is_active=false` — matches are not deleted; the matching worker already
  filters `WHERE rt.is_active` in `store.MatchFindingToTopics`, so new findings
  stop matching this topic.

**`SetMatchState`:** enum validation, then a transparent store call.

**`ListMatches`:** validate `limit` (1–100, default 50 — the Library pattern),
`offset` (≥0, default 0), `state` (an enum or nil), `topic_id` (an int64 or nil).
Another user's topic gives an empty result through the `t.user_id` filter in the
WHERE, with no separate check.

**`ListTopics` / `GetTopic`:** thin wrappers over the store.

**`ListFeeds`:** validate `limit`/`offset`, then a transparent store call. The
admin check is middleware at the router level.

**`LastSweep`:** a transparent store call.

## 6. The HTTP layer

We extend `internal/radar/http.go` with eight handlers in the existing pattern
(an `XxxHandler() http.HandlerFunc` getter plus a private handler). Query params
are parsed through `r.URL.Query()` (as in `library.ListHandler`). Path params go
through `chi.URLParam`.

Wiring in `internal/server/server.go`, inside the existing
`r.Route("/radar", …)`:

```go
r.Get("/topics", radarHTTP.ListTopicsHandler())
r.Get("/topics/{id}", radarHTTP.GetTopicHandler())
r.Patch("/topics/{id}", radarHTTP.UpdateTopicHandler())
r.Delete("/topics/{id}", radarHTTP.DeleteTopicHandler())

r.Get("/matches", radarHTTP.ListMatchesHandler())
r.Patch("/matches/{id}", radarHTTP.UpdateMatchHandler())

r.Get("/status", radarHTTP.StatusHandler())

r.Group(func(r chi.Router) {
    r.Use(coreauth.RequireAdmin)
    r.Get("/feeds", radarHTTP.ListFeedsHandler())
})
```

The existing `DisabledHandler` under the `/radar/*` wildcard automatically
covers the new routes when `LINKTHECA_RADAR_ENABLED=false`.

## 7. Testing

We follow the existing pattern (`store_test.go`, `service_test.go`,
`http_test.go`, `integration_test.go`). No new frameworks. Tests go next to the
existing ones.

### Store tests (testcontainers plus a real pg+pgvector)

| Test | What it checks |
|---|---|
| `ListTopicsWithStats_empty` | a user with no topics → `[]` |
| `ListTopicsWithStats_aggregates` | two topics with different counts/sources/states → correct aggregates |
| `ListTopicsWithStats_isolation` | another user's topics do not leak |
| `GetTopicWithStats_notFound` | someone else's topic → `ErrNotFound` |
| `UpdateTopic_partial` | `name` only — `description` and the embedding are unchanged in the database |
| `UpdateTopic_allFields` | every field passed — every field updated |
| `DeleteTopic_cascades` | deletion takes the related matches with it |
| `ListMatches_filters` | `topic_id` and `state` filter (a matrix of four cases) |
| `ListMatches_isolation` | matches of other people's topics are not returned even with an explicit `topic_id` |
| `ListMatches_pagination` | offset/limit plus total |
| `ListMatches_ordering` | `matched_at DESC` |
| `UpdateMatchState_ownership` | a match on someone else's topic → `ErrNotFound` |
| `UpdateMatchState_idempotent` | `seen → seen` is fine |
| `LastSweepAt_noSubs` | no subscriptions → `nil` |
| `LastSweepAt_picksMax` | two subscriptions → the max |
| `ListFeeds_pagination` | offset/limit plus total |

### Service tests (a mock store plus a mock embedder)

| Test | What it checks |
|---|---|
| `UpdateTopic_descriptionTriggersEmbed` | description in the patch → store.UpdateTopic is called, then the embedder, then store.UpdateTopicEmbedding |
| `UpdateTopic_embedderUnavailable` | the embedder is down → `ErrEmbedderUnavailable`; store.UpdateTopic has already been called (fields updated, embedding not) |
| `UpdateTopic_nameOnly_noEmbed` | `name` only → store.UpdateTopic is called, the embedder is NOT |
| `UpdateTopic_thresholdValidation` | `-0.1`, `1.5` → `ErrInvalidInput` |
| `UpdateTopic_noFields` | an empty patch → `ErrInvalidInput "no fields to update"` |
| `SetMatchState_validation` | `"foo"` → `ErrInvalidInput` |
| `ListMatches_clampLimit` | `limit=200` → 100; `limit=0` → 50 (the default) |

### HTTP tests (httptest plus a mock service)

For every new handler: the happy path, `not_found`, `bad_request`, and
authorization. A separate test for the admin guard on `/feeds` (a non-admin user
→ 403). Query-param decoding (`limit`, `offset`, `state`, `topic_id`) is checked
explicitly.

### Integration test (one scenario, extending `integration_test.go`)

1. Register a user, create a topic, subscribe to a feed.
2. Directly `INSERT` a finding and a match (bypassing the crawler).
3. `GET /radar/topics` → check the `stats` aggregates.
4. `GET /radar/matches?state=new` → one item with denormalized `topic_name` and
   `feed_title`.
5. `PATCH /radar/matches/{id}` with `{state:"seen"}`.
6. `GET /radar/topics` → `new_count` has dropped.
7. `DELETE /radar/topics/{id}` → 204.
8. `GET /radar/matches` → empty (the CASCADE fired).

### What we do NOT test

- Performance and load.
- HNSW recall (covered in the pipeline plan).
- The embedder itself (mocked).
- The `LINKTHECA_RADAR_ENABLED=false` wiring — already covered by existing tests
  through the wildcard.

## 8. Edge cases

- **A topic with no embedding** (the embedder failed during `CreateTopic` or an
  earlier `UpdateTopic`) — `has_embedding=false` in the response. The frontend can
  repeat the PATCH with `description` to trigger a recompute. The embed is
  synchronous, not through the job queue.
- **A match on someone else's topic** — 404, not 403. Likewise for DELETE/PATCH
  on a topic.
- **Concurrent PATCHes** — last writer wins; optimistic locking is not needed.
- **DELETE of a topic during a PATCH with a re-embed** — a race is theoretically
  possible: a simultaneous DELETE and a description PATCH. If the PATCH lands
  first, the DELETE clears everything through the CASCADE. If it lands between
  the field UPDATE and `UpdateTopicEmbedding`, then `UpdateTopicEmbedding`
  returns `ErrNotFound` and the service answers 404. Acceptable.
- **`limit=0`** — the service substitutes the default of 50 (the Library
  pattern).
- **The match worker already filters `is_active`** — verified in
  `store.MatchFindingToTopics` (line 243, `AND rt.is_active`). No bug fix needed.

## 9. Explicitly out of scope

- The frontend (Radar UI) — the next plan.
- Cursor pagination.
- `GET /radar/findings` — the UI does not show raw findings.
- A per-topic threshold slider with live preview — deferred
  (`project_radar_threshold_slider_deferred.md`).
- User-added feeds and unsubscribing from a feed — deferred
  (`project_user_added_feeds_deferred.md`).
- Deleting or deactivating feeds (admin).
- Search across matches and findings.
- Search across topics.
- Notifications about new matches.

## Next steps

After this document is approved — move to `superpowers:writing-plans` for a
single plan:

- `2026-05-XX-radar-read-api.md` — the store/service/http methods, no migrations
  needed, tests, and the `server.go` extension.

After that comes a separate frontend plan, `radar-ui.md` (the `/radar` and
`/radar/:id` pages, AddTopicDialog, edit/delete, and marking matches as seen).
