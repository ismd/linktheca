# Radar Frontend (Topic UI) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Превратить Radar из заглушенного пункта меню в полноценный раздел: список топиков с агрегатами, Topic view с лентой матчей, match reader, CRUD топиков (Create / Edit / Pause / Delete), auto mark-seen при открытии reader'а.

**Architecture:** Frontend-расширение зеркалит `features/library/*` (api/schemas/types/use-hooks/components) + новые routes под существующим `_app.tsx`. Один точечный backend-довесок: `GET /radar/matches/{id}` симметрично `GetTopic`. Без миграций, без новых пакетов на бэке.

**Tech Stack:** Frontend — TypeScript, React Router v7 file-based, TanStack Query v5, Zod, react-hook-form + zodResolver, shadcn-style UI (`@/shared/ui/*`), Tailwind, sonner toasts, Vitest + Testing Library + MSW. Backend (one task) — Go 1.22+, `chi/v5`, `pgx/v5`, `testify`, `testcontainers-go` (через `internal/testing/testdb`).

**Spec:** `docs/superpowers/specs/2026-05-18-radar-frontend-design.md`

---

## API contract (locked here)

| Method | Path | Auth | Что нужно от UI |
|---|---|---|---|
| `GET` | `/radar/topics` | user | существует |
| `GET` | `/radar/topics/{id}` | user | существует |
| `POST` | `/radar/topics` | user | существует, `{name, description, match_threshold?}` |
| `PATCH` | `/radar/topics/{id}` | user | существует, `{name?, description?, match_threshold?, is_active?}` |
| `DELETE` | `/radar/topics/{id}` | user | существует |
| `GET` | `/radar/matches` | user | существует, query `topic_id?, state?, limit, offset` |
| `GET` | `/radar/matches/{id}` | user | **новый** — Task 1-5 |
| `PATCH` | `/radar/matches/{id}` | user | существует, `{state: "new"\|"seen"}` |
| `GET` | `/radar/status` | user | существует |

Ответы snake_case; UI маппит в camelCase. Detailed shapes см. `internal/radar/types.go` и spec read-API.

---

## Files touched

### Backend (Phase 1)
| Path | Изменение |
|---|---|
| `internal/radar/store.go` | extend — добавить `GetMatch` |
| `internal/radar/service.go` | extend — `StoreAPI.GetMatch`, `Service.GetMatch` |
| `internal/radar/http.go` | extend — `GetMatchHandler` |
| `internal/radar/store_test.go` | extend — 3 test cases |
| `internal/radar/service_test.go` | extend — mockStore `GetMatch` + 2 tests |
| `internal/radar/http_test.go` | extend — 3 test cases |
| `internal/radar/integration_test.go` | extend — добавить шаг в end-to-end |
| `internal/server/server.go` | extend — wire route |

### Frontend
| Path | Изменение |
|---|---|
| `web/src/features/radar/types.ts` | create |
| `web/src/features/radar/schemas.ts` | create |
| `web/src/features/radar/schemas.test.ts` | create |
| `web/src/features/radar/api.ts` | create |
| `web/src/features/radar/api.test.ts` | create |
| `web/src/features/radar/use-radar.tsx` | create |
| `web/src/features/radar/use-radar.test.tsx` | create |
| `web/src/features/radar/use-mutations.tsx` | create |
| `web/src/features/radar/use-mutations.test.tsx` | create |
| `web/src/features/radar/time.ts` | create — `fmtSweep`, `fmtLastMatch` |
| `web/src/features/radar/components/TopicCard.tsx` | create |
| `web/src/features/radar/components/TopicCard.test.tsx` | create |
| `web/src/features/radar/components/TopicGrid.tsx` | create |
| `web/src/features/radar/components/MatchCard.tsx` | create |
| `web/src/features/radar/components/MatchCard.test.tsx` | create |
| `web/src/features/radar/components/MatchGrid.tsx` | create |
| `web/src/features/radar/components/StatsLine.tsx` | create |
| `web/src/features/radar/components/TopicHeader.tsx` | create |
| `web/src/features/radar/components/EmptyTopicList.tsx` | create |
| `web/src/features/radar/components/EmptyTopicMatches.tsx` | create |
| `web/src/features/radar/components/SkeletonCard.tsx` | create |
| `web/src/features/radar/components/NewTopicDialog.tsx` | create |
| `web/src/features/radar/components/NewTopicDialog.test.tsx` | create |
| `web/src/features/radar/components/EditTopicDialog.tsx` | create |
| `web/src/features/radar/components/EditTopicDialog.test.tsx` | create |
| `web/src/features/radar/components/DeleteTopicConfirm.tsx` | create |
| `web/src/features/radar/components/MatchReader.tsx` | create — body of reader route |
| `web/src/features/radar/components/MatchReader.test.tsx` | create — auto-mark-seen check |
| `web/src/features/radar/use-new-topic-store.ts` | create — zustand toggle (как `use-add-link-store`) |
| `web/src/routes/radar._index.tsx` | create |
| `web/src/routes/radar.$topicId.tsx` | create |
| `web/src/routes/radar.matches.$matchId.tsx` | create |
| `web/src/App.tsx` | extend — register radar routes |
| `web/src/shared/layout/Sidebar.tsx` | modify — снять `disabled: true` с Radar; mount `NewTopicDialog` |
| `web/src/routes/__app.tsx` | modify — mount `<NewTopicDialog />` рядом с `<AddLinkDialog />` |

---

# Phase 1 — Backend: `GET /radar/matches/{id}`

## Task 1: Store — `GetMatch`

**Files:**
- Modify: `internal/radar/store.go`
- Test: `internal/radar/store_test.go`

- [ ] **Step 1: Write failing test in `store_test.go`** (append after the last `TestStore_*` test):

```go
func TestStore_GetMatch_ok(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)
	topicID := seedTopic(t, pool, userID, "A", "desc", 0.55, true)
	feedID := seedFeed(t, pool, "https://f.example/rss", "F1")
	findingID := seedFinding(t, pool, feedID, "https://x.example/1", "t1")
	matchID := seedMatch(t, pool, topicID, findingID, "new", 0.7)

	mv, err := store.GetMatch(ctx, userID, matchID)
	require.NoError(t, err)
	require.Equal(t, matchID, mv.ID)
	require.Equal(t, topicID, mv.TopicID)
	require.Equal(t, "A", mv.TopicName)
	require.Equal(t, float32(0.7), mv.Similarity)
	require.Equal(t, "new", mv.State)
	require.Equal(t, findingID, mv.Finding.ID)
	require.Equal(t, "https://x.example/1", mv.Finding.URL)
	require.NotNil(t, mv.Finding.FeedTitle)
	require.Equal(t, "F1", *mv.Finding.FeedTitle)
}

func TestStore_GetMatch_notFound(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)

	_, err := store.GetMatch(ctx, userID, 99999)
	require.ErrorIs(t, err, radar.ErrNotFound)
}

func TestStore_GetMatch_otherUser(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userA := seedUser(t, pool)
	userB := seedUser(t, pool)
	topicB := seedTopic(t, pool, userB, "B", "desc-b", 0.55, true)
	feedID := seedFeed(t, pool, "https://f.example/rss2", "F2")
	findingID := seedFinding(t, pool, feedID, "https://x.example/2", "t2")
	matchID := seedMatch(t, pool, topicB, findingID, "new", 0.7)

	_, err := store.GetMatch(ctx, userA, matchID)
	require.ErrorIs(t, err, radar.ErrNotFound)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/ismd/coding/linktheca
go test ./internal/radar -run 'TestStore_GetMatch' -count=1
```

Expected: FAIL — `store.GetMatch undefined`.

- [ ] **Step 3: Implement `GetMatch` in `store.go`** (add after the existing `ListMatches`):

```go
func (s *Store) GetMatch(ctx context.Context, userID, matchID int64) (*MatchView, error) {
	row := s.db.QueryRow(ctx, `
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
		WHERE m.id = $1 AND t.user_id = $2`,
		matchID, userID)

	var mv MatchView
	if err := row.Scan(
		&mv.ID, &mv.TopicID, &mv.TopicName,
		&mv.Similarity, &mv.State, &mv.MatchedAt,
		&mv.Finding.ID, &mv.Finding.FeedID, &mv.Finding.FeedTitle,
		&mv.Finding.URL, &mv.Finding.Title, &mv.Finding.Summary,
		&mv.Finding.PublishedAt, &mv.Finding.DiscoveredAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get match: %w", err)
	}
	return &mv, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/radar -run 'TestStore_GetMatch' -count=1 -v
```

Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/radar/store.go internal/radar/store_test.go
git commit -m "feat(radar): add Store.GetMatch for single-match read"
```

---

## Task 2: Service — `GetMatch`

**Files:**
- Modify: `internal/radar/service.go` (StoreAPI interface + Service method)
- Modify: `internal/radar/service_test.go` (mockStore + tests)

- [ ] **Step 1: Extend `StoreAPI` interface in `service.go`** (in the `type StoreAPI interface {…}` block, in the read-API extensions group):

```go
	GetMatch(ctx context.Context, userID, matchID int64) (*MatchView, error)
```

Add directly after the `ListMatches` line in the interface.

- [ ] **Step 2: Add `mockStore.GetMatch` to `service_test.go`** (after the existing `ListMatches` mock method):

```go
func (m *mockStore) GetMatch(_ context.Context, _, _ int64) (*radar.MatchView, error) {
	m.getMatchCalled = true
	return m.getMatchResult, m.getMatchErr
}
```

And add the recording fields to the `mockStore` struct (in the "Read-API recording / overrides:" group):

```go
	getMatchResult *radar.MatchView
	getMatchErr    error
	getMatchCalled bool
```

- [ ] **Step 3: Write failing service tests in `service_test.go`** (append after `TestService_ListMatches_*`):

```go
func TestService_GetMatch_passesThrough(t *testing.T) {
	store := newMockStore()
	want := &radar.MatchView{ID: 7, TopicID: 3, TopicName: "T"}
	store.getMatchResult = want
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	got, err := svc.GetMatch(context.Background(), 1, 7)
	require.NoError(t, err)
	require.Equal(t, want, got)
	require.True(t, store.getMatchCalled)
}

func TestService_GetMatch_notFound(t *testing.T) {
	store := newMockStore()
	store.getMatchErr = radar.ErrNotFound
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	_, err := svc.GetMatch(context.Background(), 1, 7)
	require.True(t, errors.Is(err, radar.ErrNotFound))
}
```

- [ ] **Step 4: Run tests to verify they fail**

```bash
go test ./internal/radar -run 'TestService_GetMatch' -count=1
```

Expected: FAIL — `svc.GetMatch undefined`.

- [ ] **Step 5: Implement `Service.GetMatch`** in `service.go` (add after `SetMatchState`):

```go
// GetMatch returns a single denormalized match owned by the user.
func (s *Service) GetMatch(ctx context.Context, userID, matchID int64) (*MatchView, error) {
	return s.store.GetMatch(ctx, userID, matchID)
}
```

- [ ] **Step 6: Run tests to verify they pass**

```bash
go test ./internal/radar -run 'TestService_GetMatch' -count=1 -v
```

Expected: PASS (2 tests).

- [ ] **Step 7: Commit**

```bash
git add internal/radar/service.go internal/radar/service_test.go
git commit -m "feat(radar): add Service.GetMatch and StoreAPI extension"
```

---

## Task 3: HTTP handler — `GET /radar/matches/{id}`

**Files:**
- Modify: `internal/radar/http.go`
- Test: `internal/radar/http_test.go`

- [ ] **Step 1: Write failing tests in `http_test.go`** (append after the existing `TestHTTP_UpdateMatch_*` tests):

```go
func TestHTTP_GetMatch_ok(t *testing.T) {
	svc := &fakeService{getMatchResult: &radar.MatchView{
		ID: 42, TopicID: 7, TopicName: "T",
		Similarity: 0.7, State: "new", MatchedAt: time.Now(),
	}}
	h := radar.NewHTTP(svc)
	r := chi.NewRouter()
	r.Use(injectUser(11))
	r.Get("/radar/matches/{id}", h.GetMatchHandler())

	req := httptest.NewRequest(http.MethodGet, "/radar/matches/42", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"id":42`)
}

func TestHTTP_GetMatch_badID(t *testing.T) {
	svc := &fakeService{}
	h := radar.NewHTTP(svc)
	r := chi.NewRouter()
	r.Use(injectUser(11))
	r.Get("/radar/matches/{id}", h.GetMatchHandler())

	req := httptest.NewRequest(http.MethodGet, "/radar/matches/abc", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHTTP_GetMatch_notFound(t *testing.T) {
	svc := &fakeService{getMatchErr: radar.ErrNotFound}
	h := radar.NewHTTP(svc)
	r := chi.NewRouter()
	r.Use(injectUser(11))
	r.Get("/radar/matches/{id}", h.GetMatchHandler())

	req := httptest.NewRequest(http.MethodGet, "/radar/matches/42", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}
```

**Note:** if `fakeService` is the test double for the HTTP layer (check existing tests). Add fields to it:

```go
	getMatchResult *radar.MatchView
	getMatchErr    error
```

And the method:

```go
func (f *fakeService) GetMatch(_ context.Context, _, _ int64) (*radar.MatchView, error) {
	return f.getMatchResult, f.getMatchErr
}
```

(Patterns: match the layout of existing `fakeService` methods like `ListMatches`/`SetMatchState`.)

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./internal/radar -run 'TestHTTP_GetMatch' -count=1
```

Expected: FAIL — `h.GetMatchHandler undefined`.

- [ ] **Step 3: Add handler in `http.go`** (next to `ListMatchesHandler` getter and impl):

```go
// GetMatchHandler returns the http.HandlerFunc for GET /radar/matches/{id}.
func (h *HTTP) GetMatchHandler() http.HandlerFunc { return h.getMatch }

func (h *HTTP) getMatch(w http.ResponseWriter, r *http.Request) {
	userID := coreauth.UserID(r.Context())
	id, err := parseRadarID(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid id")
		return
	}
	mv, err := h.svc.GetMatch(r.Context(), userID, id)
	if err != nil {
		writeRadarError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, mv)
}
```

Also extend the service interface inside `http.go` (the unexported `serviceAPI` or analogue — check around the top of the file for the local interface name). Add `GetMatch(ctx context.Context, userID, matchID int64) (*MatchView, error)` to it.

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/radar -run 'TestHTTP_GetMatch' -count=1 -v
```

Expected: PASS (3 tests).

- [ ] **Step 5: Run full radar package tests**

```bash
go test ./internal/radar -count=1
```

Expected: all pass (no regressions in other tests).

- [ ] **Step 6: Commit**

```bash
git add internal/radar/http.go internal/radar/http_test.go
git commit -m "feat(radar): add GET /radar/matches/{id} handler"
```

---

## Task 4: Wire route in `server.go`

**Files:**
- Modify: `internal/server/server.go`

- [ ] **Step 1: Add route inside existing `r.Route("/radar", …)`** (immediately after the existing `r.Get("/matches", radarHTTP.ListMatchesHandler())` and before `r.Patch("/matches/{id}", …)`):

```go
			r.Get("/matches/{id}", radarHTTP.GetMatchHandler())
```

- [ ] **Step 2: Compile + run all server tests**

```bash
go build ./...
go test ./internal/server/... -count=1
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/server/server.go
git commit -m "feat(server): wire GET /radar/matches/{id}"
```

---

## Task 5: Integration test — end-to-end `GET /radar/matches/{id}`

**Files:**
- Modify: `internal/radar/integration_test.go`

- [ ] **Step 1: Find existing end-to-end test** (search for `GET /radar/matches?state=new` block — that's where a match is fetched from the list).

- [ ] **Step 2: After the existing list-matches assertion, add a single-match fetch + assertion**:

```go
	// Single-match fetch by id (new endpoint).
	rec, _ = doJSON(http.MethodGet, fmt.Sprintf("/radar/matches/%d", matchID), nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var single radar.MatchView
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &single))
	require.Equal(t, matchID, single.ID)
	require.NotEmpty(t, single.TopicName)
	require.NotEmpty(t, single.Finding.URL)

	// Direct-URL access by a stranger 404s (no leak).
	rec, _ = doJSON404(http.MethodGet, fmt.Sprintf("/radar/matches/%d", matchID), nil, otherUserToken)
	require.Equal(t, http.StatusNotFound, rec.Code)
```

If the existing test does not already have a second user / stranger setup, skip the "stranger" assertion (the unit-level coverage in `store_test.go::TestStore_GetMatch_otherUser` is sufficient). Match the existing helper signatures (`doJSON`).

- [ ] **Step 3: Run integration test**

```bash
go test ./internal/radar -run 'Integration' -count=1 -v
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/radar/integration_test.go
git commit -m "test(radar): cover GET /radar/matches/{id} in integration test"
```

Backend phase complete. Frontend can now proceed.

---

# Phase 2 — Frontend feature module

## Task 6: Types

**Files:**
- Create: `web/src/features/radar/types.ts`

- [ ] **Step 1: Create file with all camelCase types**

```ts
export type MatchState = "new" | "seen";

export type TopicStats = {
  newCount: number;
  totalCount: number;
  sourceCount: number;
  lastMatchAt: Date | null;
};

export type Topic = {
  id: number;
  userId: number;
  name: string;
  description: string;
  matchThreshold: number;
  isActive: boolean;
  hasEmbedding: boolean;
  createdAt: Date;
  updatedAt: Date;
};

export type TopicWithStats = Topic & {
  stats: TopicStats;
};

export type MatchFinding = {
  id: number;
  feedId: number;
  feedTitle: string | null;
  url: string;
  title: string | null;
  summary: string | null;
  publishedAt: Date | null;
  discoveredAt: Date;
};

export type MatchView = {
  id: number;
  topicId: number;
  topicName: string;
  similarity: number;
  state: MatchState;
  matchedAt: Date;
  finding: MatchFinding;
};

export type MatchList = {
  items: MatchView[];
  total: number;
};

export type RadarStatus = {
  lastSweepAt: Date | null;
};

export type MatchFilters = {
  topicId?: number;
  state?: MatchState;
};

export const PAGE_SIZE = 20;
```

- [ ] **Step 2: Commit**

```bash
git add web/src/features/radar/types.ts
git commit -m "feat(radar/web): add radar feature types"
```

---

## Task 7: Schemas (Zod) + mappers

**Files:**
- Create: `web/src/features/radar/schemas.ts`
- Create: `web/src/features/radar/schemas.test.ts`

- [ ] **Step 1: Write failing test in `schemas.test.ts`**

```ts
import { describe, it, expect } from "vitest";
import {
  RawTopicWithStatsSchema,
  RawMatchViewSchema,
  RawMatchListSchema,
  RawRadarStatusSchema,
  mapTopicWithStats,
  mapMatchView,
  mapMatchList,
  mapRadarStatus,
} from "./schemas";

describe("radar schemas", () => {
  it("mapTopicWithStats converts snake_case + dates", () => {
    const raw = RawTopicWithStatsSchema.parse({
      id: 1,
      user_id: 7,
      name: "Topic",
      description: "Desc",
      match_threshold: 0.55,
      is_active: true,
      has_embedding: true,
      created_at: "2026-05-01T10:00:00Z",
      updated_at: "2026-05-02T10:00:00Z",
      stats: {
        new_count: 3,
        total_count: 10,
        source_count: 2,
        last_match_at: "2026-05-18T09:00:00Z",
      },
    });
    const t = mapTopicWithStats(raw);
    expect(t.id).toBe(1);
    expect(t.userId).toBe(7);
    expect(t.matchThreshold).toBe(0.55);
    expect(t.isActive).toBe(true);
    expect(t.hasEmbedding).toBe(true);
    expect(t.createdAt).toBeInstanceOf(Date);
    expect(t.stats.newCount).toBe(3);
    expect(t.stats.totalCount).toBe(10);
    expect(t.stats.sourceCount).toBe(2);
    expect(t.stats.lastMatchAt).toBeInstanceOf(Date);
  });

  it("mapTopicWithStats handles null last_match_at", () => {
    const raw = RawTopicWithStatsSchema.parse({
      id: 1, user_id: 7, name: "T", description: "D",
      match_threshold: 0.55, is_active: true, has_embedding: false,
      created_at: "2026-05-01T10:00:00Z", updated_at: "2026-05-02T10:00:00Z",
      stats: { new_count: 0, total_count: 0, source_count: 0, last_match_at: null },
    });
    expect(mapTopicWithStats(raw).stats.lastMatchAt).toBeNull();
  });

  it("mapMatchView converts nested finding + dates", () => {
    const raw = RawMatchViewSchema.parse({
      id: 42, topic_id: 7, topic_name: "T",
      similarity: 0.7, state: "new",
      matched_at: "2026-05-18T10:00:00Z",
      finding: {
        id: 100, feed_id: 5, feed_title: "Feed",
        url: "https://x.example/a", title: "Title", summary: "Summary",
        published_at: "2026-05-17T10:00:00Z",
        discovered_at: "2026-05-18T09:00:00Z",
      },
    });
    const m = mapMatchView(raw);
    expect(m.id).toBe(42);
    expect(m.topicName).toBe("T");
    expect(m.matchedAt).toBeInstanceOf(Date);
    expect(m.finding.feedTitle).toBe("Feed");
    expect(m.finding.publishedAt).toBeInstanceOf(Date);
    expect(m.finding.discoveredAt).toBeInstanceOf(Date);
  });

  it("mapMatchList maps items + total", () => {
    const raw = RawMatchListSchema.parse({
      items: [],
      total: 0,
    });
    const list = mapMatchList(raw);
    expect(list.total).toBe(0);
    expect(list.items).toEqual([]);
  });

  it("mapRadarStatus handles null", () => {
    expect(mapRadarStatus(RawRadarStatusSchema.parse({ last_sweep_at: null }))
      .lastSweepAt).toBeNull();
    const filled = mapRadarStatus(RawRadarStatusSchema.parse({
      last_sweep_at: "2026-05-18T10:00:00Z",
    }));
    expect(filled.lastSweepAt).toBeInstanceOf(Date);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /home/ismd/coding/linktheca/web
npm test -- --run src/features/radar/schemas.test.ts
```

Expected: FAIL (module not found).

- [ ] **Step 3: Create `schemas.ts`**

```ts
import { z } from "zod";
import type {
  TopicWithStats,
  MatchView,
  MatchList,
  RadarStatus,
} from "./types";

export const RawTopicStatsSchema = z.object({
  new_count: z.number().int(),
  total_count: z.number().int(),
  source_count: z.number().int(),
  last_match_at: z.string().nullable(),
});

export const RawTopicWithStatsSchema = z.object({
  id: z.number().int(),
  user_id: z.number().int(),
  name: z.string(),
  description: z.string(),
  match_threshold: z.number(),
  is_active: z.boolean(),
  has_embedding: z.boolean(),
  created_at: z.string(),
  updated_at: z.string(),
  stats: RawTopicStatsSchema,
});

export const RawMatchFindingSchema = z.object({
  id: z.number().int(),
  feed_id: z.number().int(),
  feed_title: z.string().nullable(),
  url: z.string(),
  title: z.string().nullable(),
  summary: z.string().nullable(),
  published_at: z.string().nullable(),
  discovered_at: z.string(),
});

export const RawMatchViewSchema = z.object({
  id: z.number().int(),
  topic_id: z.number().int(),
  topic_name: z.string(),
  similarity: z.number(),
  state: z.enum(["new", "seen"]),
  matched_at: z.string(),
  finding: RawMatchFindingSchema,
});

export const RawMatchListSchema = z.object({
  items: z.array(RawMatchViewSchema),
  total: z.number().int(),
});

export const RawTopicsListSchema = z.object({
  items: z.array(RawTopicWithStatsSchema),
});

export const RawRadarStatusSchema = z.object({
  last_sweep_at: z.string().nullable(),
});

export type RawTopicWithStats = z.infer<typeof RawTopicWithStatsSchema>;
export type RawMatchView = z.infer<typeof RawMatchViewSchema>;

export function mapTopicWithStats(raw: RawTopicWithStats): TopicWithStats {
  return {
    id: raw.id,
    userId: raw.user_id,
    name: raw.name,
    description: raw.description,
    matchThreshold: raw.match_threshold,
    isActive: raw.is_active,
    hasEmbedding: raw.has_embedding,
    createdAt: new Date(raw.created_at),
    updatedAt: new Date(raw.updated_at),
    stats: {
      newCount: raw.stats.new_count,
      totalCount: raw.stats.total_count,
      sourceCount: raw.stats.source_count,
      lastMatchAt: raw.stats.last_match_at ? new Date(raw.stats.last_match_at) : null,
    },
  };
}

export function mapMatchView(raw: RawMatchView): MatchView {
  return {
    id: raw.id,
    topicId: raw.topic_id,
    topicName: raw.topic_name,
    similarity: raw.similarity,
    state: raw.state,
    matchedAt: new Date(raw.matched_at),
    finding: {
      id: raw.finding.id,
      feedId: raw.finding.feed_id,
      feedTitle: raw.finding.feed_title,
      url: raw.finding.url,
      title: raw.finding.title,
      summary: raw.finding.summary,
      publishedAt: raw.finding.published_at ? new Date(raw.finding.published_at) : null,
      discoveredAt: new Date(raw.finding.discovered_at),
    },
  };
}

export function mapMatchList(raw: z.infer<typeof RawMatchListSchema>): MatchList {
  return {
    items: raw.items.map(mapMatchView),
    total: raw.total,
  };
}

export function mapRadarStatus(raw: z.infer<typeof RawRadarStatusSchema>): RadarStatus {
  return { lastSweepAt: raw.last_sweep_at ? new Date(raw.last_sweep_at) : null };
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
npm test -- --run src/features/radar/schemas.test.ts
```

Expected: PASS (5 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/features/radar/schemas.ts web/src/features/radar/schemas.test.ts
git commit -m "feat(radar/web): add Zod schemas and snake→camel mappers"
```

---

## Task 8: API functions

**Files:**
- Create: `web/src/features/radar/api.ts`
- Create: `web/src/features/radar/api.test.ts`

- [ ] **Step 1: Write failing test in `api.test.ts`**

```ts
import { describe, it, expect, beforeEach } from "vitest";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import {
  listTopics,
  getTopic,
  createTopic,
  updateTopic,
  deleteTopic,
  listMatches,
  getMatch,
  updateMatch,
  getStatus,
} from "./api";

const rawTopic = (overrides: Record<string, unknown> = {}) => ({
  id: 1,
  user_id: 7,
  name: "T",
  description: "Desc",
  match_threshold: 0.55,
  is_active: true,
  has_embedding: true,
  created_at: "2026-05-01T10:00:00Z",
  updated_at: "2026-05-02T10:00:00Z",
  stats: {
    new_count: 0,
    total_count: 0,
    source_count: 0,
    last_match_at: null,
  },
  ...overrides,
});

const rawMatch = (overrides: Record<string, unknown> = {}) => ({
  id: 100,
  topic_id: 1,
  topic_name: "T",
  similarity: 0.7,
  state: "new",
  matched_at: "2026-05-18T10:00:00Z",
  finding: {
    id: 200, feed_id: 5, feed_title: "Feed",
    url: "https://x.example/a", title: "Title", summary: "Summary",
    published_at: "2026-05-17T10:00:00Z",
    discovered_at: "2026-05-18T09:00:00Z",
  },
  ...overrides,
});

beforeEach(() => {
  useAuthStore.getState().setSession("access", {
    id: 1, email: "u@x.co", displayName: "U", isAdmin: false,
  });
});

describe("radar api", () => {
  it("listTopics maps items[]", async () => {
    server.use(
      http.get("/api/radar/topics", () =>
        HttpResponse.json({ items: [rawTopic(), rawTopic({ id: 2 })] })),
    );
    const ts = await listTopics();
    expect(ts).toHaveLength(2);
    expect(ts[0]!.createdAt).toBeInstanceOf(Date);
    expect(ts[0]!.stats.lastMatchAt).toBeNull();
  });

  it("getTopic maps response", async () => {
    server.use(
      http.get("/api/radar/topics/42", () =>
        HttpResponse.json(rawTopic({ id: 42 }))),
    );
    const t = await getTopic(42);
    expect(t.id).toBe(42);
  });

  it("createTopic POSTs body", async () => {
    let captured: unknown = null;
    server.use(
      http.post("/api/radar/topics", async ({ request }) => {
        captured = await request.json();
        return HttpResponse.json(rawTopic({ id: 99, name: "New" }), { status: 201 });
      }),
    );
    const t = await createTopic({ name: "New", description: "Desc longer than 10 chars" });
    expect(captured).toEqual({ name: "New", description: "Desc longer than 10 chars" });
    expect(t.id).toBe(99);
  });

  it("updateTopic PATCHes only provided fields", async () => {
    let captured: unknown = null;
    server.use(
      http.patch("/api/radar/topics/3", async ({ request }) => {
        captured = await request.json();
        return HttpResponse.json(rawTopic({ id: 3, is_active: false }));
      }),
    );
    await updateTopic(3, { isActive: false });
    expect(captured).toEqual({ is_active: false });
  });

  it("deleteTopic DELETEs", async () => {
    let called = false;
    server.use(
      http.delete("/api/radar/topics/9", () => {
        called = true;
        return new HttpResponse(null, { status: 204 });
      }),
    );
    await deleteTopic(9);
    expect(called).toBe(true);
  });

  it("listMatches sends query params", async () => {
    let capturedUrl = "";
    server.use(
      http.get("/api/radar/matches", ({ request }) => {
        capturedUrl = request.url;
        return HttpResponse.json({ items: [rawMatch()], total: 1 });
      }),
    );
    const page = await listMatches({ topicId: 5, state: "new", limit: 20, offset: 40 });
    expect(capturedUrl).toContain("topic_id=5");
    expect(capturedUrl).toContain("state=new");
    expect(capturedUrl).toContain("limit=20");
    expect(capturedUrl).toContain("offset=40");
    expect(page.total).toBe(1);
  });

  it("listMatches omits filters when not set", async () => {
    let capturedUrl = "";
    server.use(
      http.get("/api/radar/matches", ({ request }) => {
        capturedUrl = request.url;
        return HttpResponse.json({ items: [], total: 0 });
      }),
    );
    await listMatches({ limit: 20, offset: 0 });
    expect(capturedUrl).not.toContain("topic_id=");
    expect(capturedUrl).not.toContain("state=");
  });

  it("getMatch maps response", async () => {
    server.use(
      http.get("/api/radar/matches/42", () => HttpResponse.json(rawMatch({ id: 42 }))),
    );
    const m = await getMatch(42);
    expect(m.id).toBe(42);
    expect(m.matchedAt).toBeInstanceOf(Date);
  });

  it("updateMatch PATCHes state and returns 204", async () => {
    let captured: unknown = null;
    server.use(
      http.patch("/api/radar/matches/42", async ({ request }) => {
        captured = await request.json();
        return new HttpResponse(null, { status: 204 });
      }),
    );
    await updateMatch(42, { state: "seen" });
    expect(captured).toEqual({ state: "seen" });
  });

  it("getStatus maps response", async () => {
    server.use(
      http.get("/api/radar/status", () =>
        HttpResponse.json({ last_sweep_at: "2026-05-18T10:00:00Z" })),
    );
    const s = await getStatus();
    expect(s.lastSweepAt).toBeInstanceOf(Date);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
npm test -- --run src/features/radar/api.test.ts
```

Expected: FAIL (module not found).

- [ ] **Step 3: Create `api.ts`**

```ts
import { apiFetch } from "@/shared/api/client";
import {
  RawTopicsListSchema,
  RawTopicWithStatsSchema,
  RawMatchListSchema,
  RawMatchViewSchema,
  RawRadarStatusSchema,
  mapTopicWithStats,
  mapMatchList,
  mapMatchView,
  mapRadarStatus,
} from "./schemas";
import type {
  TopicWithStats,
  MatchList,
  MatchView,
  RadarStatus,
  MatchState,
} from "./types";

function parseInDev<T>(schema: { parse: (x: unknown) => T }, data: unknown): T {
  if (import.meta.env.DEV || import.meta.env.MODE === "test") {
    return schema.parse(data);
  }
  return data as T;
}

export async function listTopics(): Promise<TopicWithStats[]> {
  const raw = await apiFetch<unknown>(`/radar/topics`);
  const parsed = parseInDev(RawTopicsListSchema, raw);
  return parsed.items.map(mapTopicWithStats);
}

export async function getTopic(id: number): Promise<TopicWithStats> {
  const raw = await apiFetch<unknown>(`/radar/topics/${id}`);
  return mapTopicWithStats(parseInDev(RawTopicWithStatsSchema, raw));
}

export type CreateTopicInput = {
  name: string;
  description: string;
  matchThreshold?: number;
};

export async function createTopic(input: CreateTopicInput): Promise<TopicWithStats> {
  const body: Record<string, unknown> = {
    name: input.name,
    description: input.description,
  };
  if (input.matchThreshold !== undefined) body.match_threshold = input.matchThreshold;
  const raw = await apiFetch<unknown>(`/radar/topics`, {
    method: "POST",
    body: JSON.stringify(body),
  });
  return mapTopicWithStats(parseInDev(RawTopicWithStatsSchema, raw));
}

export type UpdateTopicInput = {
  name?: string;
  description?: string;
  matchThreshold?: number;
  isActive?: boolean;
};

export async function updateTopic(
  id: number,
  input: UpdateTopicInput,
): Promise<TopicWithStats> {
  const body: Record<string, unknown> = {};
  if (input.name !== undefined) body.name = input.name;
  if (input.description !== undefined) body.description = input.description;
  if (input.matchThreshold !== undefined) body.match_threshold = input.matchThreshold;
  if (input.isActive !== undefined) body.is_active = input.isActive;
  const raw = await apiFetch<unknown>(`/radar/topics/${id}`, {
    method: "PATCH",
    body: JSON.stringify(body),
  });
  return mapTopicWithStats(parseInDev(RawTopicWithStatsSchema, raw));
}

export async function deleteTopic(id: number): Promise<void> {
  await apiFetch<void>(`/radar/topics/${id}`, { method: "DELETE" });
}

export type ListMatchesArgs = {
  topicId?: number;
  state?: MatchState;
  limit: number;
  offset: number;
};

function buildMatchesQuery(args: ListMatchesArgs): string {
  const p = new URLSearchParams();
  p.set("limit", String(args.limit));
  p.set("offset", String(args.offset));
  if (args.topicId !== undefined) p.set("topic_id", String(args.topicId));
  if (args.state) p.set("state", args.state);
  return p.toString();
}

export async function listMatches(args: ListMatchesArgs): Promise<MatchList> {
  const raw = await apiFetch<unknown>(`/radar/matches?${buildMatchesQuery(args)}`);
  return mapMatchList(parseInDev(RawMatchListSchema, raw));
}

export async function getMatch(id: number): Promise<MatchView> {
  const raw = await apiFetch<unknown>(`/radar/matches/${id}`);
  return mapMatchView(parseInDev(RawMatchViewSchema, raw));
}

export async function updateMatch(
  id: number,
  input: { state: MatchState },
): Promise<void> {
  await apiFetch<void>(`/radar/matches/${id}`, {
    method: "PATCH",
    body: JSON.stringify({ state: input.state }),
  });
}

export async function getStatus(): Promise<RadarStatus> {
  const raw = await apiFetch<unknown>(`/radar/status`);
  return mapRadarStatus(parseInDev(RawRadarStatusSchema, raw));
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
npm test -- --run src/features/radar/api.test.ts
```

Expected: PASS (10 tests).

- [ ] **Step 5: Commit**

```bash
git add web/src/features/radar/api.ts web/src/features/radar/api.test.ts
git commit -m "feat(radar/web): add API client functions"
```

---

## Task 9: Query hooks

**Files:**
- Create: `web/src/features/radar/use-radar.tsx`
- Create: `web/src/features/radar/use-radar.test.tsx`

- [ ] **Step 1: Write failing tests in `use-radar.test.tsx`**

```tsx
import { describe, it, expect, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import {
  useTopicsQuery,
  useTopicQuery,
  useMatchesQuery,
  useMatchQuery,
  useRadarStatusQuery,
} from "./use-radar";

function wrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const Wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
  Wrapper.displayName = "TestWrapper";
  return Wrapper;
}

const rawTopic = (id: number) => ({
  id, user_id: 1, name: `T${id}`, description: "D",
  match_threshold: 0.55, is_active: true, has_embedding: true,
  created_at: "2026-05-01T10:00:00Z", updated_at: "2026-05-02T10:00:00Z",
  stats: { new_count: 0, total_count: 0, source_count: 0, last_match_at: null },
});

const rawMatch = (id: number) => ({
  id, topic_id: 1, topic_name: "T1", similarity: 0.7, state: "new",
  matched_at: "2026-05-18T10:00:00Z",
  finding: {
    id: 200, feed_id: 5, feed_title: "F",
    url: "https://x.example/a", title: "Title", summary: null,
    published_at: null, discovered_at: "2026-05-18T09:00:00Z",
  },
});

beforeEach(() => {
  useAuthStore.getState().setSession("access", {
    id: 1, email: "u@x.co", displayName: "U", isAdmin: false,
  });
});

describe("useTopicsQuery", () => {
  it("loads topics array", async () => {
    server.use(
      http.get("/api/radar/topics", () =>
        HttpResponse.json({ items: [rawTopic(1), rawTopic(2)] })),
    );
    const { result } = renderHook(() => useTopicsQuery(), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data).toHaveLength(2);
  });
});

describe("useTopicQuery", () => {
  it("loads single topic", async () => {
    server.use(
      http.get("/api/radar/topics/7", () => HttpResponse.json(rawTopic(7))),
    );
    const { result } = renderHook(() => useTopicQuery(7), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.id).toBe(7);
  });
});

describe("useMatchesQuery", () => {
  it("first page with topicId filter", async () => {
    let capturedUrl = "";
    server.use(
      http.get("/api/radar/matches", ({ request }) => {
        capturedUrl = request.url;
        return HttpResponse.json({
          items: [rawMatch(1), rawMatch(2)],
          total: 2,
        });
      }),
    );
    const { result } = renderHook(
      () => useMatchesQuery({ topicId: 1 }),
      { wrapper: wrapper() },
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(capturedUrl).toContain("topic_id=1");
    expect(result.current.items).toHaveLength(2);
    expect(result.current.hasMore).toBe(false);
  });

  it("computes hasMore when total > loaded", async () => {
    server.use(
      http.get("/api/radar/matches", () =>
        HttpResponse.json({
          items: Array.from({ length: 20 }, (_, i) => rawMatch(i + 1)),
          total: 55,
        })),
    );
    const { result } = renderHook(() => useMatchesQuery({}), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.hasMore).toBe(true);
  });
});

describe("useMatchQuery", () => {
  it("loads single match", async () => {
    server.use(
      http.get("/api/radar/matches/42", () => HttpResponse.json(rawMatch(42))),
    );
    const { result } = renderHook(() => useMatchQuery(42), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.id).toBe(42);
  });
});

describe("useRadarStatusQuery", () => {
  it("loads last_sweep_at", async () => {
    server.use(
      http.get("/api/radar/status", () =>
        HttpResponse.json({ last_sweep_at: "2026-05-18T10:00:00Z" })),
    );
    const { result } = renderHook(() => useRadarStatusQuery(), { wrapper: wrapper() });
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.lastSweepAt).toBeInstanceOf(Date);
  });
});
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
npm test -- --run src/features/radar/use-radar.test.tsx
```

Expected: FAIL (module not found).

- [ ] **Step 3: Create `use-radar.tsx`**

```tsx
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import {
  listTopics,
  getTopic,
  listMatches,
  getMatch,
  getStatus,
} from "./api";
import { PAGE_SIZE, type MatchFilters, type MatchList } from "./types";

export const radarKeys = {
  all: ["radar"] as const,
  topics: ["radar", "topics"] as const,
  topic: (id: number) => ["radar", "topic", id] as const,
  matches: (filters: MatchFilters) => ["radar", "matches", filters] as const,
  match: (id: number) => ["radar", "match", id] as const,
  status: ["radar", "status"] as const,
};

export function useTopicsQuery() {
  return useQuery({
    queryKey: radarKeys.topics,
    queryFn: listTopics,
  });
}

export function useTopicQuery(id: number) {
  return useQuery({
    queryKey: radarKeys.topic(id),
    queryFn: () => getTopic(id),
    enabled: Number.isFinite(id) && id > 0,
  });
}

export function useMatchesQuery(filters: MatchFilters) {
  const query = useInfiniteQuery({
    queryKey: radarKeys.matches(filters),
    queryFn: ({ pageParam }) =>
      listMatches({
        limit: PAGE_SIZE,
        offset: pageParam as number,
        topicId: filters.topicId,
        state: filters.state,
      }),
    initialPageParam: 0,
    getNextPageParam: (last: MatchList, all: MatchList[]) => {
      const loaded = all.reduce((s, p) => s + p.items.length, 0);
      return loaded < last.total ? loaded : undefined;
    },
  });

  const items = (query.data?.pages ?? []).flatMap((p) => p.items);
  const total = query.data?.pages?.[0]?.total ?? 0;
  const hasMore = query.hasNextPage ?? false;

  return {
    items,
    total,
    hasMore,
    isLoading: query.isLoading,
    isSuccess: query.isSuccess,
    isError: query.isError,
    error: query.error,
    isFetchingNextPage: query.isFetchingNextPage,
    fetchNextPage: query.fetchNextPage,
    refetch: query.refetch,
  };
}

export function useMatchQuery(id: number) {
  return useQuery({
    queryKey: radarKeys.match(id),
    queryFn: () => getMatch(id),
    enabled: Number.isFinite(id) && id > 0,
  });
}

export function useRadarStatusQuery() {
  return useQuery({
    queryKey: radarKeys.status,
    queryFn: getStatus,
  });
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
npm test -- --run src/features/radar/use-radar.test.tsx
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/radar/use-radar.tsx web/src/features/radar/use-radar.test.tsx
git commit -m "feat(radar/web): add query hooks (topics, matches, status)"
```

---

## Task 10: Mutation hooks

**Files:**
- Create: `web/src/features/radar/use-mutations.tsx`
- Create: `web/src/features/radar/use-mutations.test.tsx`

- [ ] **Step 1: Write failing tests in `use-mutations.test.tsx`**

```tsx
import { describe, it, expect, beforeEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import { ApiError } from "@/shared/api/errors";
import { radarKeys } from "./use-radar";
import {
  useCreateTopic,
  useUpdateTopic,
  useDeleteTopic,
  useMarkMatchSeen,
} from "./use-mutations";

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const wrapper = ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
  return { qc, wrapper };
}

const rawTopic = (id: number, overrides: Record<string, unknown> = {}) => ({
  id, user_id: 1, name: `T${id}`, description: "D",
  match_threshold: 0.55, is_active: true, has_embedding: true,
  created_at: "2026-05-01T10:00:00Z", updated_at: "2026-05-02T10:00:00Z",
  stats: { new_count: 0, total_count: 0, source_count: 0, last_match_at: null },
  ...overrides,
});

beforeEach(() => {
  useAuthStore.getState().setSession("access", {
    id: 1, email: "u@x.co", displayName: "U", isAdmin: false,
  });
});

describe("useCreateTopic", () => {
  it("on success invalidates topics list", async () => {
    server.use(
      http.post("/api/radar/topics", () =>
        HttpResponse.json(rawTopic(99), { status: 201 })),
    );
    const { qc, wrapper } = makeWrapper();
    qc.setQueryData(radarKeys.topics, []);

    const { result } = renderHook(() => useCreateTopic(), { wrapper });
    await act(async () => {
      await result.current.mutateAsync({ name: "X", description: "Y enough chars" });
    });
    await waitFor(() =>
      expect(qc.getQueryState(radarKeys.topics)?.isInvalidated).toBe(true),
    );
  });

  it("503 embedder_unavailable surfaces as ApiError with code", async () => {
    server.use(
      http.post("/api/radar/topics", () =>
        HttpResponse.json(
          { code: "embedder_unavailable", message: "embedding service is unavailable" },
          { status: 503 },
        )),
    );
    const { wrapper } = makeWrapper();
    const { result } = renderHook(() => useCreateTopic(), { wrapper });
    let caught: unknown;
    await act(async () => {
      try {
        await result.current.mutateAsync({ name: "X", description: "Y enough chars" });
      } catch (e) {
        caught = e;
      }
    });
    expect(caught).toBeInstanceOf(ApiError);
    expect((caught as ApiError).code).toBe("embedder_unavailable");
    expect((caught as ApiError).status).toBe(503);
  });
});

describe("useUpdateTopic (optimistic isActive toggle)", () => {
  it("optimistically updates topics list cache", async () => {
    let resolve!: (v: Response) => void;
    const slow = new Promise<Response>((r) => (resolve = r));
    server.use(http.patch("/api/radar/topics/1", () => slow));

    const { qc, wrapper } = makeWrapper();
    qc.setQueryData(radarKeys.topics, [
      mapTopic(rawTopic(1, { is_active: true })),
      mapTopic(rawTopic(2)),
    ]);

    const { result } = renderHook(() => useUpdateTopic(), { wrapper });
    act(() => {
      result.current.mutate({ id: 1, input: { isActive: false } });
    });

    await waitFor(() => {
      const list = qc.getQueryData<ReturnType<typeof mapTopic>[]>(radarKeys.topics);
      expect(list![0]!.isActive).toBe(false);
    });

    resolve(HttpResponse.json(rawTopic(1, { is_active: false })));
  });
});

describe("useDeleteTopic", () => {
  it("removes topic cache entries on success", async () => {
    server.use(
      http.delete("/api/radar/topics/5", () =>
        new HttpResponse(null, { status: 204 })),
    );
    const { qc, wrapper } = makeWrapper();
    qc.setQueryData(radarKeys.topic(5), mapTopic(rawTopic(5)));

    const { result } = renderHook(() => useDeleteTopic(), { wrapper });
    await act(async () => {
      await result.current.mutateAsync(5);
    });

    expect(qc.getQueryData(radarKeys.topic(5))).toBeUndefined();
  });
});

describe("useMarkMatchSeen", () => {
  it("on success invalidates matches + topics + match queries", async () => {
    server.use(
      http.patch("/api/radar/matches/42", () =>
        new HttpResponse(null, { status: 204 })),
    );
    const { qc, wrapper } = makeWrapper();
    qc.setQueryData(radarKeys.match(42), {});
    qc.setQueryData(radarKeys.matches({}), { pages: [{ items: [], total: 0 }], pageParams: [0] });
    qc.setQueryData(radarKeys.topics, []);

    const { result } = renderHook(() => useMarkMatchSeen(), { wrapper });
    await act(async () => {
      await result.current.mutateAsync(42);
    });

    await waitFor(() => {
      expect(qc.getQueryState(radarKeys.match(42))?.isInvalidated).toBe(true);
      expect(qc.getQueryState(radarKeys.topics)?.isInvalidated).toBe(true);
    });
  });
});

// helper duplicated locally to avoid coupling tests to mapper internals
function mapTopic(raw: ReturnType<typeof rawTopic>) {
  return {
    id: raw.id,
    userId: raw.user_id,
    name: raw.name,
    description: raw.description,
    matchThreshold: raw.match_threshold,
    isActive: raw.is_active,
    hasEmbedding: raw.has_embedding,
    createdAt: new Date(raw.created_at),
    updatedAt: new Date(raw.updated_at),
    stats: {
      newCount: raw.stats.new_count,
      totalCount: raw.stats.total_count,
      sourceCount: raw.stats.source_count,
      lastMatchAt: raw.stats.last_match_at ? new Date(raw.stats.last_match_at) : null,
    },
  };
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
npm test -- --run src/features/radar/use-mutations.test.tsx
```

Expected: FAIL (module not found).

- [ ] **Step 3: Create `use-mutations.tsx`**

```tsx
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  createTopic,
  updateTopic,
  deleteTopic,
  updateMatch,
  type CreateTopicInput,
  type UpdateTopicInput,
} from "./api";
import { radarKeys } from "./use-radar";
import type { TopicWithStats } from "./types";

type UpdateArgs = { id: number; input: UpdateTopicInput };

type RollbackCtx = {
  previousTopics: TopicWithStats[] | undefined;
  previousTopic: TopicWithStats | undefined;
};

export function useCreateTopic() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: CreateTopicInput) => createTopic(input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: radarKeys.topics });
    },
  });
}

export function useUpdateTopic() {
  const qc = useQueryClient();
  return useMutation<TopicWithStats, Error, UpdateArgs, RollbackCtx>({
    mutationFn: ({ id, input }) => updateTopic(id, input),
    onMutate: async ({ id, input }) => {
      await qc.cancelQueries({ queryKey: radarKeys.topics });
      const previousTopics = qc.getQueryData<TopicWithStats[]>(radarKeys.topics);
      const previousTopic = qc.getQueryData<TopicWithStats>(radarKeys.topic(id));

      if (previousTopics) {
        qc.setQueryData<TopicWithStats[]>(
          radarKeys.topics,
          previousTopics.map((t) => (t.id === id ? patchTopic(t, input) : t)),
        );
      }
      if (previousTopic) {
        qc.setQueryData<TopicWithStats>(
          radarKeys.topic(id),
          patchTopic(previousTopic, input),
        );
      }
      return { previousTopics, previousTopic };
    },
    onError: (_err, vars, ctx) => {
      if (ctx?.previousTopics !== undefined) {
        qc.setQueryData(radarKeys.topics, ctx.previousTopics);
      }
      if (ctx?.previousTopic !== undefined) {
        qc.setQueryData(radarKeys.topic(vars.id), ctx.previousTopic);
      }
    },
    onSettled: (_data, _err, vars) => {
      qc.invalidateQueries({ queryKey: radarKeys.topic(vars.id) });
      qc.invalidateQueries({ queryKey: radarKeys.topics });
    },
  });
}

function patchTopic(t: TopicWithStats, input: UpdateTopicInput): TopicWithStats {
  return {
    ...t,
    name: input.name ?? t.name,
    description: input.description ?? t.description,
    matchThreshold: input.matchThreshold ?? t.matchThreshold,
    isActive: input.isActive ?? t.isActive,
  };
}

export function useDeleteTopic() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => deleteTopic(id),
    onSuccess: (_data, id) => {
      qc.removeQueries({ queryKey: radarKeys.topic(id) });
      qc.removeQueries({ queryKey: ["radar", "matches"] });
      qc.invalidateQueries({ queryKey: radarKeys.topics });
    },
  });
}

export function useMarkMatchSeen() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: number) => updateMatch(id, { state: "seen" }),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: radarKeys.match(id) });
      qc.invalidateQueries({ queryKey: ["radar", "matches"] });
      qc.invalidateQueries({ queryKey: radarKeys.topics });
    },
  });
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
npm test -- --run src/features/radar/use-mutations.test.tsx
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/radar/use-mutations.tsx web/src/features/radar/use-mutations.test.tsx
git commit -m "feat(radar/web): add mutation hooks (create/update/delete topic, mark match seen)"
```

---

## Task 11: Time utilities

**Files:**
- Create: `web/src/features/radar/time.ts`

- [ ] **Step 1: Create file**

```ts
import { relativeFromNow } from "@/features/library/time";

export function fmtSweep(d: Date | null): string {
  return d ? `Last sweep · ${relativeFromNow(d)}` : "Awaiting first sweep";
}

export function fmtLastMatch(d: Date | null): string {
  return d ? relativeFromNow(d) : "—";
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/features/radar/time.ts
git commit -m "feat(radar/web): add time formatting helpers"
```

---

# Phase 3 — Components

## Task 12: `TopicCard` + test

**Files:**
- Create: `web/src/features/radar/components/TopicCard.tsx`
- Create: `web/src/features/radar/components/TopicCard.test.tsx`

- [ ] **Step 1: Write failing test**

```tsx
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { TopicCard } from "./TopicCard";
import type { TopicWithStats } from "../types";

const topic: TopicWithStats = {
  id: 7, userId: 1, name: "Local-first software", description: "CRDTs and beyond",
  matchThreshold: 0.55, isActive: true, hasEmbedding: true,
  createdAt: new Date("2026-04-01"), updatedAt: new Date("2026-05-01"),
  stats: { newCount: 3, totalCount: 21, sourceCount: 4, lastMatchAt: new Date() },
};

function r(node: React.ReactElement) {
  return render(<MemoryRouter>{node}</MemoryRouter>);
}

describe("TopicCard", () => {
  it("renders name, description, stats and link", () => {
    r(<TopicCard topic={topic} index={0} />);
    expect(screen.getByText("Local-first software")).toBeInTheDocument();
    expect(screen.getByText(/CRDTs and beyond/)).toBeInTheDocument();
    expect(screen.getByText("3 new")).toBeInTheDocument();
    expect(screen.getByText(/21 found/)).toBeInTheDocument();
    expect(screen.getByText(/4 sources/)).toBeInTheDocument();
    const link = screen.getByRole("link");
    expect(link).toHaveAttribute("href", "/radar/7");
  });

  it("renders dash when newCount is 0", () => {
    r(<TopicCard topic={{ ...topic, stats: { ...topic.stats, newCount: 0 } }} index={1} />);
    expect(screen.queryByText("0 new")).toBeNull();
    expect(screen.getByText("—")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
npm test -- --run src/features/radar/components/TopicCard.test.tsx
```

Expected: FAIL (module not found).

- [ ] **Step 3: Create `TopicCard.tsx`**

```tsx
import { Link } from "react-router";
import { fmtLastMatch } from "../time";
import type { TopicWithStats } from "../types";

type Props = {
  topic: TopicWithStats;
  index: number;
};

export function TopicCard({ topic, index }: Props) {
  const newCount = topic.stats.newCount;
  return (
    <Link
      to={`/radar/${topic.id}`}
      className={`topic-card block p-6 ${topic.isActive ? "" : "inactive"} animate-fade-in`}
    >
      <div className="flex items-start justify-between mb-3">
        <div className="label-sc text-muted-foreground">
          Topic · {String(index + 1).padStart(2, "0")}
        </div>
        {newCount > 0 ? (
          <div className="label-sc text-vermillion">{newCount} new</div>
        ) : (
          <div className="label-sc text-muted-foreground">—</div>
        )}
      </div>
      <h3 className="display-tight text-2xl text-ink leading-tight mb-3">
        {topic.name}
      </h3>
      <p className="font-body text-base text-muted-foreground leading-relaxed mb-5 line-clamp-2">
        {topic.description}
      </p>
      <div className="rule-dotted mb-4"></div>
      <div className="flex items-center justify-between">
        <div className="label-sc text-muted-foreground">
          {topic.stats.totalCount} found · {topic.stats.sourceCount} sources
        </div>
        <div className="label-sc text-muted-foreground">
          {fmtLastMatch(topic.stats.lastMatchAt)}
        </div>
      </div>
    </Link>
  );
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
npm test -- --run src/features/radar/components/TopicCard.test.tsx
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/radar/components/TopicCard.tsx web/src/features/radar/components/TopicCard.test.tsx
git commit -m "feat(radar/web): add TopicCard component"
```

---

## Task 13: `TopicGrid`, `EmptyTopicList`, `SkeletonCard`

**Files:**
- Create: `web/src/features/radar/components/TopicGrid.tsx`
- Create: `web/src/features/radar/components/EmptyTopicList.tsx`
- Create: `web/src/features/radar/components/SkeletonCard.tsx`

- [ ] **Step 1: Create `TopicGrid.tsx`**

```tsx
import type { TopicWithStats } from "../types";
import { TopicCard } from "./TopicCard";

type Props = {
  topics: TopicWithStats[];
  dim?: boolean;
};

export function TopicGrid({ topics, dim = false }: Props) {
  return (
    <div
      className={`grid grid-cols-1 md:grid-cols-2 gap-5 ${dim ? "opacity-60" : ""}`}
    >
      {topics.map((t, i) => (
        <TopicCard key={t.id} topic={t} index={i} />
      ))}
    </div>
  );
}
```

- [ ] **Step 2: Create `EmptyTopicList.tsx`**

```tsx
import { Button } from "@/shared/ui/button";

type Props = {
  onAdd: () => void;
};

export function EmptyTopicList({ onAdd }: Props) {
  return (
    <div className="text-center py-20 border border-dashed border-rule">
      <p className="display-tight text-3xl text-ink mb-3">Nothing on your radar yet</p>
      <p className="font-body italic text-muted-foreground mb-8">
        Add your first topic to start watching for new signals.
      </p>
      <Button onClick={onAdd}>+ New topic</Button>
    </div>
  );
}
```

- [ ] **Step 3: Create `SkeletonCard.tsx`** (used as placeholder during grid loading):

```tsx
export function SkeletonCard() {
  return (
    <div className="topic-card block p-6 animate-pulse">
      <div className="flex items-start justify-between mb-3">
        <div className="skeleton h-3 w-20" />
        <div className="skeleton h-3 w-12" />
      </div>
      <div className="skeleton h-7 w-2/3 mb-3" />
      <div className="skeleton h-4 w-full mb-2" />
      <div className="skeleton h-4 w-5/6 mb-5" />
      <div className="rule-dotted mb-4" />
      <div className="flex items-center justify-between">
        <div className="skeleton h-3 w-1/2" />
        <div className="skeleton h-3 w-1/4" />
      </div>
    </div>
  );
}
```

- [ ] **Step 4: Commit**

```bash
git add web/src/features/radar/components/TopicGrid.tsx \
        web/src/features/radar/components/EmptyTopicList.tsx \
        web/src/features/radar/components/SkeletonCard.tsx
git commit -m "feat(radar/web): add TopicGrid, EmptyTopicList, SkeletonCard"
```

---

## Task 14: `MatchCard` + test, `MatchGrid`, `EmptyTopicMatches`

**Files:**
- Create: `web/src/features/radar/components/MatchCard.tsx`
- Create: `web/src/features/radar/components/MatchCard.test.tsx`
- Create: `web/src/features/radar/components/MatchGrid.tsx`
- Create: `web/src/features/radar/components/EmptyTopicMatches.tsx`

- [ ] **Step 1: Write failing test for `MatchCard`**

```tsx
import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { MatchCard } from "./MatchCard";
import type { MatchView } from "../types";

const match: MatchView = {
  id: 42, topicId: 7, topicName: "Local-first", similarity: 0.7,
  state: "new", matchedAt: new Date("2026-05-18T10:00:00Z"),
  finding: {
    id: 100, feedId: 5, feedTitle: "Ink & Switch",
    url: "https://inkandswitch.com/local-first/",
    title: "Local-First Software", summary: "A great essay…",
    publishedAt: new Date("2026-05-17T10:00:00Z"),
    discoveredAt: new Date("2026-05-18T09:00:00Z"),
  },
};

function r(n: React.ReactElement) {
  return render(<MemoryRouter>{n}</MemoryRouter>);
}

describe("MatchCard", () => {
  it("renders title, source, new-stamp, and link", () => {
    r(<MatchCard match={match} index={0} />);
    expect(screen.getByText("Local-First Software")).toBeInTheDocument();
    expect(screen.getByText(/Ink & Switch/)).toBeInTheDocument();
    expect(screen.getByText("new")).toBeInTheDocument();
    const link = screen.getByRole("link");
    expect(link).toHaveAttribute("href", "/radar/matches/42");
  });

  it("hides new-stamp when state is seen", () => {
    r(<MatchCard match={{ ...match, state: "seen" }} index={0} />);
    expect(screen.queryByText("new")).toBeNull();
  });

  it("falls back to URL when finding.title is null", () => {
    r(<MatchCard match={{ ...match, finding: { ...match.finding, title: null } }} index={0} />);
    expect(screen.getByText(/inkandswitch.com/)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
npm test -- --run src/features/radar/components/MatchCard.test.tsx
```

Expected: FAIL.

- [ ] **Step 3: Create `MatchCard.tsx`**

```tsx
import { Link } from "react-router";
import { relativeFromNow } from "@/features/library/time";
import type { MatchView } from "../types";

function host(u: string): string {
  try {
    return new URL(u).host.replace(/^www\./, "");
  } catch {
    return u;
  }
}

type Props = {
  match: MatchView;
  index: number;
};

export function MatchCard({ match, index }: Props) {
  const f = match.finding;
  const title = f.title ?? host(f.url);
  const source = f.feedTitle ?? host(f.url);
  const stamp = match.state === "new";
  const when = f.publishedAt ?? f.discoveredAt;
  return (
    <Link to={`/radar/matches/${match.id}`} className="feed-card group block">
      <article className="flex flex-col h-full p-5 border border-rule">
        <div className="flex items-center gap-2 mb-3 flex-wrap">
          {stamp && <span className="stamp text-vermillion stamp-flat">new</span>}
          <span className="label-sc text-muted-foreground">{source}</span>
          <span className="label-sc text-muted-foreground">·</span>
          <span className="label-sc text-muted-foreground">
            {relativeFromNow(when)}
          </span>
          <span className="label-sc text-muted-foreground ml-auto">
            {String(index + 1).padStart(2, "0")}
          </span>
        </div>
        <h2 className="display-tight text-xl text-ink leading-tight mb-3 line-clamp-2">
          {title}
        </h2>
        {f.summary && (
          <p className="font-body text-base text-muted-foreground leading-relaxed line-clamp-3">
            {f.summary}
          </p>
        )}
      </article>
    </Link>
  );
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
npm test -- --run src/features/radar/components/MatchCard.test.tsx
```

Expected: PASS.

- [ ] **Step 5: Create `MatchGrid.tsx`**

```tsx
import type { MatchView } from "../types";
import { MatchCard } from "./MatchCard";

type Props = {
  matches: MatchView[];
};

export function MatchGrid({ matches }: Props) {
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
      {matches.map((m, i) => (
        <MatchCard key={m.id} match={m} index={i} />
      ))}
    </div>
  );
}
```

- [ ] **Step 6: Create `EmptyTopicMatches.tsx`**

```tsx
export function EmptyTopicMatches() {
  return (
    <div className="text-center py-16 border border-dashed border-rule">
      <p className="label-sc text-muted-foreground mb-3">Nothing yet</p>
      <p className="font-body italic text-muted-foreground">
        Standing watch. New entries will appear here as they are found.
      </p>
    </div>
  );
}
```

- [ ] **Step 7: Commit**

```bash
git add web/src/features/radar/components/MatchCard.tsx \
        web/src/features/radar/components/MatchCard.test.tsx \
        web/src/features/radar/components/MatchGrid.tsx \
        web/src/features/radar/components/EmptyTopicMatches.tsx
git commit -m "feat(radar/web): add MatchCard, MatchGrid, EmptyTopicMatches"
```

---

## Task 15: `StatsLine`, `TopicHeader`

**Files:**
- Create: `web/src/features/radar/components/StatsLine.tsx`
- Create: `web/src/features/radar/components/TopicHeader.tsx`

- [ ] **Step 1: Create `StatsLine.tsx`**

```tsx
import type { TopicWithStats } from "../types";

function fmtDate(d: Date): string {
  return d.toISOString().slice(0, 10);
}

type Props = {
  topic: TopicWithStats;
};

export function StatsLine({ topic }: Props) {
  const s = topic.stats;
  return (
    <div className="flex flex-wrap items-center gap-x-5 gap-y-1 mt-6 mb-4">
      <span className="label-sc text-ink">
        <span className="text-vermillion font-bold">{s.totalCount}</span> found
      </span>
      <span className="label-sc text-muted-foreground">·</span>
      <span
        className={`label-sc ${s.newCount > 0 ? "text-vermillion" : "text-muted-foreground"}`}
      >
        {s.newCount} unread
      </span>
      <span className="label-sc text-muted-foreground">·</span>
      <span className="label-sc text-ink">{s.sourceCount} sources</span>
      <span className="label-sc text-muted-foreground">·</span>
      <span className="label-sc text-muted-foreground">
        created {fmtDate(topic.createdAt)}
      </span>
    </div>
  );
}
```

- [ ] **Step 2: Create `TopicHeader.tsx`**

```tsx
import { Button } from "@/shared/ui/button";
import type { TopicWithStats } from "../types";

type Props = {
  topic: TopicWithStats;
  onEdit: () => void;
  onTogglePause: () => void;
  onDelete: () => void;
  togglePending: boolean;
};

export function TopicHeader({
  topic,
  onEdit,
  onTogglePause,
  onDelete,
  togglePending,
}: Props) {
  return (
    <div className="flex items-start justify-between gap-6 mb-4">
      <div className="flex-1 min-w-0">
        <div className="label-sc-lg text-muted-foreground mb-3">
          {topic.isActive ? "Standing watch" : "Paused"}
        </div>
        <h1 className="display-tight text-4xl md:text-5xl text-ink leading-tight">
          {topic.name}
        </h1>
        <p className="mt-4 font-body text-lg text-ink-3 leading-relaxed max-w-[680px]">
          {topic.description}
        </p>
      </div>
      <div className="hidden md:flex gap-2 flex-shrink-0 pt-1">
        <Button variant="outline" onClick={onEdit} aria-label="Edit topic">
          Edit
        </Button>
        <Button
          variant="ghost"
          onClick={onTogglePause}
          disabled={togglePending}
          aria-label={topic.isActive ? "Pause topic" : "Resume topic"}
        >
          {topic.isActive ? "Pause" : "Resume"}
        </Button>
        <Button variant="ghost" onClick={onDelete} aria-label="Delete topic">
          Delete
        </Button>
      </div>
    </div>
  );
}
```

- [ ] **Step 3: Commit**

```bash
git add web/src/features/radar/components/StatsLine.tsx \
        web/src/features/radar/components/TopicHeader.tsx
git commit -m "feat(radar/web): add StatsLine and TopicHeader components"
```

---

## Task 16: `NewTopicDialog` (modal + zustand store) + tests

**Files:**
- Create: `web/src/features/radar/use-new-topic-store.ts`
- Create: `web/src/features/radar/components/NewTopicDialog.tsx`
- Create: `web/src/features/radar/components/NewTopicDialog.test.tsx`

- [ ] **Step 1: Create `use-new-topic-store.ts`**

```ts
import { create } from "zustand";

type State = {
  isOpen: boolean;
  open: () => void;
  close: () => void;
};

export const useNewTopicStore = create<State>((set) => ({
  isOpen: false,
  open: () => set({ isOpen: true }),
  close: () => set({ isOpen: false }),
}));
```

- [ ] **Step 2: Write failing test in `NewTopicDialog.test.tsx`**

```tsx
import { describe, it, expect, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import { Toaster } from "@/shared/ui/sonner";
import { useNewTopicStore } from "../use-new-topic-store";
import { NewTopicDialog } from "./NewTopicDialog";

function Wrap({ children }: { children: React.ReactNode }) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return (
    <QueryClientProvider client={qc}>
      {children}
      <Toaster />
    </QueryClientProvider>
  );
}

const rawTopic = {
  id: 1, user_id: 1, name: "X", description: "Long enough description.",
  match_threshold: 0.55, is_active: true, has_embedding: true,
  created_at: "2026-05-01T10:00:00Z", updated_at: "2026-05-02T10:00:00Z",
  stats: { new_count: 0, total_count: 0, source_count: 0, last_match_at: null },
};

beforeEach(() => {
  useAuthStore.getState().setSession("access", {
    id: 1, email: "u@x.co", displayName: "U", isAdmin: false,
  });
  useNewTopicStore.getState().open();
});

describe("NewTopicDialog", () => {
  it("submits create and closes on success", async () => {
    server.use(
      http.post("/api/radar/topics", () =>
        HttpResponse.json(rawTopic, { status: 201 })),
    );
    const user = userEvent.setup();
    render(
      <Wrap>
        <NewTopicDialog />
      </Wrap>,
    );
    await user.type(screen.getByLabelText(/name/i), "Local-first");
    await user.type(
      screen.getByLabelText(/description/i),
      "CRDTs and offline-first tooling, the user-owned data movement.",
    );
    await user.click(screen.getByRole("button", { name: /save/i }));

    // dialog closes
    await screen.findByText(/saved/i);
    expect(useNewTopicStore.getState().isOpen).toBe(false);
  });

  it("shows specific error on 503 embedder_unavailable and stays open", async () => {
    server.use(
      http.post("/api/radar/topics", () =>
        HttpResponse.json(
          { code: "embedder_unavailable", message: "embedding service is unavailable" },
          { status: 503 },
        )),
    );
    const user = userEvent.setup();
    render(
      <Wrap>
        <NewTopicDialog />
      </Wrap>,
    );
    await user.type(screen.getByLabelText(/name/i), "X");
    await user.type(screen.getByLabelText(/description/i), "Long enough description for radar.");
    await user.click(screen.getByRole("button", { name: /save/i }));

    expect(await screen.findByText(/embedder/i)).toBeInTheDocument();
    expect(useNewTopicStore.getState().isOpen).toBe(true);
  });

  it("validates name and description before submit", async () => {
    const user = userEvent.setup();
    render(
      <Wrap>
        <NewTopicDialog />
      </Wrap>,
    );
    await user.click(screen.getByRole("button", { name: /save/i }));
    expect(await screen.findByText(/name is required/i)).toBeInTheDocument();
  });
});
```

- [ ] **Step 3: Run test to verify it fails**

```bash
npm test -- --run src/features/radar/components/NewTopicDialog.test.tsx
```

Expected: FAIL.

- [ ] **Step 4: Create `NewTopicDialog.tsx`**

```tsx
import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogDescription,
} from "@/shared/ui/dialog";
import { Input } from "@/shared/ui/input";
import { Label } from "@/shared/ui/label";
import { Button } from "@/shared/ui/button";
import { ApiError } from "@/shared/api/errors";
import { useNewTopicStore } from "../use-new-topic-store";
import { useCreateTopic } from "../use-mutations";

const schema = z.object({
  name: z.string().min(1, "Name is required").max(200, "Name too long"),
  description: z
    .string()
    .min(10, "Description must be at least 10 characters")
    .max(2000, "Description too long"),
});
type FormValues = z.infer<typeof schema>;

function mapError(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.code === "embedder_unavailable") {
      return "Embedder is currently unavailable. Try again in a moment.";
    }
    if (err.status === 400) {
      return err.message || "Invalid input";
    }
    return "Could not save — please try again";
  }
  return "Could not save — please try again";
}

function NewTopicForm({ onClose }: { onClose: () => void }) {
  const create = useCreateTopic();
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { name: "", description: "" },
  });
  const [topError, setTopError] = useState<string | null>(null);

  const onSubmit = handleSubmit(async ({ name, description }) => {
    setTopError(null);
    try {
      await create.mutateAsync({ name, description });
      toast.success("Saved");
      onClose();
    } catch (err) {
      setTopError(mapError(err));
    }
  });

  return (
    <form onSubmit={onSubmit} noValidate className="flex flex-col gap-4">
      {topError && (
        <div
          role="alert"
          className="border border-vermillion bg-paper-2 px-3 py-2 text-sm text-vermillion-dark"
        >
          {topError}
        </div>
      )}

      <div className="flex flex-col gap-2">
        <Label htmlFor="new-topic-name" className="label-sc text-ink-3">Name</Label>
        <Input
          id="new-topic-name"
          aria-invalid={errors.name ? "true" : "false"}
          disabled={create.isPending}
          {...register("name")}
        />
        {errors.name && (
          <p className="text-sm text-vermillion-dark">{errors.name.message}</p>
        )}
      </div>

      <div className="flex flex-col gap-2">
        <Label htmlFor="new-topic-desc" className="label-sc text-ink-3">Description</Label>
        <textarea
          id="new-topic-desc"
          rows={4}
          className="border border-rule bg-paper px-3 py-2 font-body text-base focus:outline-none focus:ring-2 focus:ring-ink/10"
          aria-invalid={errors.description ? "true" : "false"}
          disabled={create.isPending}
          {...register("description")}
        />
        {errors.description && (
          <p className="text-sm text-vermillion-dark">{errors.description.message}</p>
        )}
      </div>

      <DialogFooter>
        <Button type="button" variant="outline" onClick={onClose} disabled={create.isPending}>
          Cancel
        </Button>
        <Button type="submit" disabled={create.isPending}>
          {create.isPending ? "Saving…" : "Save"}
        </Button>
      </DialogFooter>
    </form>
  );
}

export function NewTopicDialog() {
  const isOpen = useNewTopicStore((s) => s.isOpen);
  const close = useNewTopicStore((s) => s.close);

  return (
    <Dialog
      open={isOpen}
      onOpenChange={(o) => {
        if (!o) close();
      }}
    >
      <DialogContent className="paper-surface">
        <DialogHeader>
          <DialogTitle className="display-tight text-3xl">New topic</DialogTitle>
          <DialogDescription className="label-sc text-muted-foreground">
            Describe what you want to watch for; Radar will keep an eye out.
          </DialogDescription>
        </DialogHeader>
        {isOpen && <NewTopicForm onClose={close} />}
      </DialogContent>
    </Dialog>
  );
}
```

- [ ] **Step 5: Run test to verify it passes**

```bash
npm test -- --run src/features/radar/components/NewTopicDialog.test.tsx
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/features/radar/use-new-topic-store.ts \
        web/src/features/radar/components/NewTopicDialog.tsx \
        web/src/features/radar/components/NewTopicDialog.test.tsx
git commit -m "feat(radar/web): add NewTopicDialog with embedder-503 handling"
```

---

## Task 17: `EditTopicDialog` + tests

**Files:**
- Create: `web/src/features/radar/components/EditTopicDialog.tsx`
- Create: `web/src/features/radar/components/EditTopicDialog.test.tsx`

- [ ] **Step 1: Write failing test in `EditTopicDialog.test.tsx`**

```tsx
import { describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import { Toaster } from "@/shared/ui/sonner";
import { EditTopicDialog } from "./EditTopicDialog";
import type { TopicWithStats } from "../types";

const topic: TopicWithStats = {
  id: 7, userId: 1, name: "Old name", description: "Old description, long enough.",
  matchThreshold: 0.55, isActive: true, hasEmbedding: true,
  createdAt: new Date(), updatedAt: new Date(),
  stats: { newCount: 0, totalCount: 0, sourceCount: 0, lastMatchAt: null },
};

function Wrap({ children }: { children: React.ReactNode }) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return (
    <QueryClientProvider client={qc}>
      {children}
      <Toaster />
    </QueryClientProvider>
  );
}

beforeEach(() => {
  useAuthStore.getState().setSession("access", {
    id: 1, email: "u@x.co", displayName: "U", isAdmin: false,
  });
});

describe("EditTopicDialog", () => {
  it("populates initial values and PATCHes only changed fields", async () => {
    let captured: Record<string, unknown> | null = null;
    server.use(
      http.patch("/api/radar/topics/7", async ({ request }) => {
        captured = (await request.json()) as Record<string, unknown>;
        return HttpResponse.json({
          id: 7, user_id: 1, name: "Old name", description: "New description, long enough.",
          match_threshold: 0.55, is_active: true, has_embedding: true,
          created_at: "2026-05-01T10:00:00Z", updated_at: "2026-05-02T10:00:00Z",
          stats: { new_count: 0, total_count: 0, source_count: 0, last_match_at: null },
        });
      }),
    );

    const user = userEvent.setup();
    const onClose = () => {};
    render(
      <Wrap>
        <EditTopicDialog topic={topic} open={true} onOpenChange={onClose} />
      </Wrap>,
    );

    const descField = screen.getByLabelText(/description/i) as HTMLTextAreaElement;
    expect(descField.value).toBe("Old description, long enough.");
    await user.clear(descField);
    await user.type(descField, "New description, long enough.");
    await user.click(screen.getByRole("button", { name: /save/i }));

    await waitFor(() => expect(captured).not.toBeNull());
    expect(captured).toEqual({ description: "New description, long enough." });
  });

  it("shows specific error on 503 embedder and stays open", async () => {
    server.use(
      http.patch("/api/radar/topics/7", () =>
        HttpResponse.json(
          { code: "embedder_unavailable", message: "embedding service is unavailable" },
          { status: 503 },
        )),
    );

    let closed = false;
    const user = userEvent.setup();
    render(
      <Wrap>
        <EditTopicDialog
          topic={topic}
          open={true}
          onOpenChange={(o) => { if (!o) closed = true; }}
        />
      </Wrap>,
    );
    const desc = screen.getByLabelText(/description/i);
    await user.clear(desc);
    await user.type(desc, "Changed description, long enough.");
    await user.click(screen.getByRole("button", { name: /save/i }));

    expect(await screen.findByText(/embedder/i)).toBeInTheDocument();
    expect(closed).toBe(false);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
npm test -- --run src/features/radar/components/EditTopicDialog.test.tsx
```

Expected: FAIL.

- [ ] **Step 3: Create `EditTopicDialog.tsx`**

```tsx
import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogDescription,
} from "@/shared/ui/dialog";
import { Input } from "@/shared/ui/input";
import { Label } from "@/shared/ui/label";
import { Button } from "@/shared/ui/button";
import { ApiError } from "@/shared/api/errors";
import { useUpdateTopic } from "../use-mutations";
import type { TopicWithStats } from "../types";
import type { UpdateTopicInput } from "../api";

const schema = z.object({
  name: z.string().min(1, "Name is required").max(200),
  description: z.string().min(10, "Description must be at least 10 characters").max(2000),
});
type FormValues = z.infer<typeof schema>;

function mapError(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.code === "embedder_unavailable") {
      return "Embedder is currently unavailable. Try again in a moment.";
    }
    if (err.status === 400) return err.message || "Invalid input";
    return "Could not save — please try again";
  }
  return "Could not save — please try again";
}

type Props = {
  topic: TopicWithStats;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

function EditTopicForm({ topic, onClose }: { topic: TopicWithStats; onClose: () => void }) {
  const update = useUpdateTopic();
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { name: topic.name, description: topic.description },
  });
  const [topError, setTopError] = useState<string | null>(null);

  const onSubmit = handleSubmit(async (values) => {
    setTopError(null);
    const patch: UpdateTopicInput = {};
    if (values.name !== topic.name) patch.name = values.name;
    if (values.description !== topic.description) patch.description = values.description;
    if (Object.keys(patch).length === 0) {
      onClose();
      return;
    }
    try {
      await update.mutateAsync({ id: topic.id, input: patch });
      toast.success("Saved");
      onClose();
    } catch (err) {
      setTopError(mapError(err));
    }
  });

  return (
    <form onSubmit={onSubmit} noValidate className="flex flex-col gap-4">
      {topError && (
        <div
          role="alert"
          className="border border-vermillion bg-paper-2 px-3 py-2 text-sm text-vermillion-dark"
        >
          {topError}
        </div>
      )}
      <div className="flex flex-col gap-2">
        <Label htmlFor="edit-topic-name" className="label-sc text-ink-3">Name</Label>
        <Input
          id="edit-topic-name"
          aria-invalid={errors.name ? "true" : "false"}
          disabled={update.isPending}
          {...register("name")}
        />
        {errors.name && (
          <p className="text-sm text-vermillion-dark">{errors.name.message}</p>
        )}
      </div>
      <div className="flex flex-col gap-2">
        <Label htmlFor="edit-topic-desc" className="label-sc text-ink-3">Description</Label>
        <textarea
          id="edit-topic-desc"
          rows={4}
          className="border border-rule bg-paper px-3 py-2 font-body text-base focus:outline-none focus:ring-2 focus:ring-ink/10"
          aria-invalid={errors.description ? "true" : "false"}
          disabled={update.isPending}
          {...register("description")}
        />
        {errors.description && (
          <p className="text-sm text-vermillion-dark">{errors.description.message}</p>
        )}
      </div>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onClose} disabled={update.isPending}>
          Cancel
        </Button>
        <Button type="submit" disabled={update.isPending}>
          {update.isPending ? "Saving…" : "Save"}
        </Button>
      </DialogFooter>
    </form>
  );
}

export function EditTopicDialog({ topic, open, onOpenChange }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="paper-surface">
        <DialogHeader>
          <DialogTitle className="display-tight text-3xl">Edit topic</DialogTitle>
          <DialogDescription className="label-sc text-muted-foreground">
            Changing the description will re-embed the topic.
          </DialogDescription>
        </DialogHeader>
        {open && <EditTopicForm topic={topic} onClose={() => onOpenChange(false)} />}
      </DialogContent>
    </Dialog>
  );
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
npm test -- --run src/features/radar/components/EditTopicDialog.test.tsx
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/radar/components/EditTopicDialog.tsx \
        web/src/features/radar/components/EditTopicDialog.test.tsx
git commit -m "feat(radar/web): add EditTopicDialog with partial-PATCH semantics"
```

---

## Task 18: `DeleteTopicConfirm`

**Files:**
- Create: `web/src/features/radar/components/DeleteTopicConfirm.tsx`

- [ ] **Step 1: Create file**

```tsx
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/shared/ui/alert-dialog";

type Props = {
  open: boolean;
  topicName: string;
  pending: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
};

export function DeleteTopicConfirm({
  open,
  topicName,
  pending,
  onOpenChange,
  onConfirm,
}: Props) {
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent className="paper-surface">
        <AlertDialogHeader>
          <AlertDialogTitle className="display-tight text-2xl">
            Delete topic "{topicName}"?
          </AlertDialogTitle>
          <AlertDialogDescription className="font-body text-muted-foreground">
            All matches for this topic will be removed. Findings are kept.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>Cancel</AlertDialogCancel>
          <AlertDialogAction onClick={onConfirm} disabled={pending}>
            {pending ? "Deleting…" : "Delete"}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/features/radar/components/DeleteTopicConfirm.tsx
git commit -m "feat(radar/web): add DeleteTopicConfirm alert-dialog"
```

---

## Task 19: `MatchReader` + auto-mark-seen test

**Files:**
- Create: `web/src/features/radar/components/MatchReader.tsx`
- Create: `web/src/features/radar/components/MatchReader.test.tsx`

- [ ] **Step 1: Write failing test in `MatchReader.test.tsx`** — critical: verify auto-mark-seen fires exactly once for `state === "new"`:

```tsx
import { describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { http, HttpResponse } from "msw";
import { server } from "@/test/setup";
import { useAuthStore } from "@/features/auth/store";
import { Toaster } from "@/shared/ui/sonner";
import { MatchReader } from "./MatchReader";

const rawMatch = (state: "new" | "seen") => ({
  id: 42, topic_id: 7, topic_name: "Local-first",
  similarity: 0.7, state, matched_at: "2026-05-18T10:00:00Z",
  finding: {
    id: 100, feed_id: 5, feed_title: "Ink & Switch",
    url: "https://x.example/a", title: "Title", summary: "Summary text",
    published_at: "2026-05-17T10:00:00Z",
    discovered_at: "2026-05-18T09:00:00Z",
  },
});

function Wrap({ children }: { children: React.ReactNode }) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return (
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        {children}
        <Toaster />
      </MemoryRouter>
    </QueryClientProvider>
  );
}

beforeEach(() => {
  useAuthStore.getState().setSession("access", {
    id: 1, email: "u@x.co", displayName: "U", isAdmin: false,
  });
});

describe("MatchReader", () => {
  it("auto-marks state=new as seen on mount (PATCH called exactly once)", async () => {
    server.use(
      http.get("/api/radar/matches/42", () => HttpResponse.json(rawMatch("new"))),
    );
    let patchCount = 0;
    server.use(
      http.patch("/api/radar/matches/42", () => {
        patchCount += 1;
        return new HttpResponse(null, { status: 204 });
      }),
    );

    render(<Wrap><MatchReader matchId={42} /></Wrap>);
    await screen.findByText("Title");
    await waitFor(() => expect(patchCount).toBe(1));
  });

  it("does NOT mark already-seen match", async () => {
    server.use(
      http.get("/api/radar/matches/42", () => HttpResponse.json(rawMatch("seen"))),
    );
    let patched = false;
    server.use(
      http.patch("/api/radar/matches/42", () => {
        patched = true;
        return new HttpResponse(null, { status: 204 });
      }),
    );

    render(<Wrap><MatchReader matchId={42} /></Wrap>);
    await screen.findByText("Title");
    // give effects time to run
    await new Promise((r) => setTimeout(r, 50));
    expect(patched).toBe(false);
  });

  it("falls back when summary is empty", async () => {
    server.use(
      http.get("/api/radar/matches/42", () =>
        HttpResponse.json({ ...rawMatch("seen"), finding: { ...rawMatch("seen").finding, summary: null } })),
    );
    render(<Wrap><MatchReader matchId={42} /></Wrap>);
    expect(await screen.findByText(/no summary captured/i)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
npm test -- --run src/features/radar/components/MatchReader.test.tsx
```

Expected: FAIL.

- [ ] **Step 3: Create `MatchReader.tsx`**

```tsx
import { useEffect, useRef, useState } from "react";
import { Link } from "react-router";
import { toast } from "sonner";
import { Button } from "@/shared/ui/button";
import { ApiError } from "@/shared/api/errors";
import { saveLink } from "@/features/library/api";
import { useMatchQuery } from "../use-radar";
import { useMarkMatchSeen } from "../use-mutations";
import { relativeFromNow } from "@/features/library/time";

function host(u: string): string {
  try {
    return new URL(u).host.replace(/^www\./, "");
  } catch {
    return u;
  }
}

type Props = { matchId: number };

export function MatchReader({ matchId }: Props) {
  const q = useMatchQuery(matchId);
  const mark = useMarkMatchSeen();
  const marked = useRef(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (q.data && q.data.state === "new" && !marked.current) {
      marked.current = true;
      mark.mutate(matchId);
    }
  }, [q.data, mark, matchId]);

  if (q.isLoading) {
    return (
      <div className="max-w-[720px] mx-auto px-4 pt-10" aria-label="Loading match">
        <div className="skeleton h-10 w-3/4 mb-6" />
        <div className="skeleton h-4 w-1/2 mb-10" />
        <div className="skeleton h-4 w-full mb-3" />
      </div>
    );
  }

  if (q.isError) {
    const notFound = q.error instanceof ApiError && q.error.status === 404;
    return (
      <div className="max-w-[720px] mx-auto px-4 pt-10 text-center">
        {notFound ? (
          <>
            <h1 className="display-tight text-3xl text-ink mb-3">Match not found</h1>
            <Link to="/radar" className="label-sc text-vermillion">← Back to radar</Link>
          </>
        ) : (
          <p className="font-body text-muted-foreground">Couldn't load this match.</p>
        )}
      </div>
    );
  }

  const m = q.data!;
  const f = m.finding;
  const title = f.title ?? f.url;
  const source = f.feedTitle ?? host(f.url);
  const when = f.publishedAt ?? f.discoveredAt;

  async function onSaveToLibrary() {
    setSaving(true);
    try {
      await saveLink(f.url);
      toast.success("Saved to library");
    } catch (e) {
      if (e instanceof ApiError && (e.code === "already_saved" || e.status === 409)) {
        toast.info("Already in library");
      } else {
        toast.error("Could not save");
      }
    } finally {
      setSaving(false);
    }
  }

  return (
    <article className="max-w-[720px] mx-auto px-4 pt-8 pb-20">
      <Link
        to={`/radar/${m.topicId}`}
        className="label-sc text-muted-foreground hover:text-vermillion inline-block mb-10"
      >
        ← Back to {m.topicName}
      </Link>

      <header className="mb-10">
        <div className="flex items-center gap-3 mb-6 flex-wrap">
          <Link
            to={`/radar/${m.topicId}`}
            className="stamp text-ink hover:text-vermillion"
          >
            {m.topicName}
          </Link>
        </div>
        <h1 className="display-tight text-4xl md:text-5xl text-ink leading-[1.05] mb-6">
          {title}
        </h1>
        <div className="flex flex-wrap items-center gap-x-5 gap-y-2 font-body italic text-base text-muted-foreground">
          <span>{source}</span>
          <span>{relativeFromNow(when)}</span>
        </div>
      </header>

      {f.summary ? (
        <p className="prose-reader font-body text-lg text-ink leading-relaxed whitespace-pre-line">
          {f.summary}
        </p>
      ) : (
        <p className="font-body italic text-muted-foreground">
          No summary captured. Open the original to read.
        </p>
      )}

      <div className="mt-16 pt-10 border-t-2 border-ink flex flex-wrap items-center gap-3">
        <a
          href={f.url}
          target="_blank"
          rel="noopener noreferrer"
          className="inline-flex"
        >
          <Button>Open original ↗</Button>
        </a>
        <Button variant="outline" onClick={onSaveToLibrary} disabled={saving}>
          {saving ? "Saving…" : "Save to library"}
        </Button>
      </div>
    </article>
  );
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
npm test -- --run src/features/radar/components/MatchReader.test.tsx
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/radar/components/MatchReader.tsx \
        web/src/features/radar/components/MatchReader.test.tsx
git commit -m "feat(radar/web): add MatchReader with auto-mark-seen"
```

---

# Phase 4 — Routes & integration

## Task 20: Route `/radar` — Radar list

**Files:**
- Create: `web/src/routes/radar._index.tsx`

- [ ] **Step 1: Create file**

```tsx
import { ApiError } from "@/shared/api/errors";
import { PageHeader } from "@/shared/layout/PageHeader";
import { Button } from "@/shared/ui/button";
import { useTopicsQuery, useRadarStatusQuery } from "@/features/radar/use-radar";
import { useNewTopicStore } from "@/features/radar/use-new-topic-store";
import { fmtSweep } from "@/features/radar/time";
import { TopicGrid } from "@/features/radar/components/TopicGrid";
import { EmptyTopicList } from "@/features/radar/components/EmptyTopicList";
import { SkeletonCard } from "@/features/radar/components/SkeletonCard";

function LoadingGrid() {
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
      <SkeletonCard />
      <SkeletonCard />
      <SkeletonCard />
      <SkeletonCard />
    </div>
  );
}

function RadarDisabled() {
  return (
    <div className="text-center py-20">
      <p className="display-tight text-3xl text-ink mb-3">Radar is disabled</p>
      <p className="font-body italic text-muted-foreground">
        This Linktheca instance was started with Radar turned off.
      </p>
    </div>
  );
}

export default function RadarListRoute() {
  const topics = useTopicsQuery();
  const status = useRadarStatusQuery();
  const openNewTopic = useNewTopicStore((s) => s.open);

  if (
    topics.error instanceof ApiError &&
    topics.error.code === "radar_disabled"
  ) {
    return <RadarDisabled />;
  }

  return (
    <div>
      <PageHeader
        title="Radar"
        subtitle={fmtSweep(status.data?.lastSweepAt ?? null)}
      />
      <div className="px-4 lg:px-8 pb-10">
        <div className="hidden md:flex justify-end mb-6">
          <Button onClick={openNewTopic}>+ New topic</Button>
        </div>
        <div className="md:hidden mb-8">
          <Button className="w-full" onClick={openNewTopic}>+ New topic</Button>
        </div>

        {topics.isLoading && <LoadingGrid />}

        {topics.isSuccess && topics.data.length === 0 && (
          <EmptyTopicList onAdd={openNewTopic} />
        )}

        {topics.isSuccess && topics.data.length > 0 && (
          <>
            <ActiveSection topics={topics.data.filter((t) => t.isActive)} />
            <PausedSection topics={topics.data.filter((t) => !t.isActive)} />
          </>
        )}
      </div>
    </div>
  );
}

function ActiveSection({ topics }: { topics: ReturnType<typeof useTopicsQuery>["data"] extends infer T ? T extends Array<infer U> ? U[] : never : never }) {
  if (topics.length === 0) return null;
  return (
    <>
      <div className="flex items-center gap-4 mb-8">
        <div className="label-sc-lg text-ink">On the radar</div>
        <div className="flex-1 rule-dotted" />
        <div className="label-sc text-muted-foreground">{topics.length} topics</div>
      </div>
      <div className="mb-16">
        <TopicGrid topics={topics} />
      </div>
    </>
  );
}

function PausedSection({ topics }: { topics: ReturnType<typeof useTopicsQuery>["data"] extends infer T ? T extends Array<infer U> ? U[] : never : never }) {
  if (topics.length === 0) return null;
  return (
    <>
      <div className="flex items-center gap-4 mb-8">
        <div className="label-sc-lg text-muted-foreground">Paused</div>
        <div className="flex-1 rule-dotted" />
        <div className="label-sc text-muted-foreground">{topics.length} topics</div>
      </div>
      <TopicGrid topics={topics} dim />
    </>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/routes/radar._index.tsx
git commit -m "feat(radar/web): add /radar route"
```

---

## Task 21: Route `/radar/$topicId` — Topic view

**Files:**
- Create: `web/src/routes/radar.$topicId.tsx`

- [ ] **Step 1: Create file**

```tsx
import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { ApiError } from "@/shared/api/errors";
import { Button } from "@/shared/ui/button";
import { useTopicQuery, useMatchesQuery } from "@/features/radar/use-radar";
import { useUpdateTopic, useDeleteTopic } from "@/features/radar/use-mutations";
import { TopicHeader } from "@/features/radar/components/TopicHeader";
import { StatsLine } from "@/features/radar/components/StatsLine";
import { MatchGrid } from "@/features/radar/components/MatchGrid";
import { EmptyTopicMatches } from "@/features/radar/components/EmptyTopicMatches";
import { EditTopicDialog } from "@/features/radar/components/EditTopicDialog";
import { DeleteTopicConfirm } from "@/features/radar/components/DeleteTopicConfirm";

export default function TopicRoute() {
  const { topicId } = useParams();
  const id = Number(topicId);
  const topic = useTopicQuery(id);
  const matches = useMatchesQuery({ topicId: id });
  const update = useUpdateTopic();
  const del = useDeleteTopic();
  const navigate = useNavigate();
  const [editOpen, setEditOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);

  if (Number.isNaN(id) || id <= 0) {
    return <div className="p-8 font-body text-muted-foreground">Invalid topic id.</div>;
  }

  if (topic.isLoading) {
    return (
      <div className="px-4 lg:px-8 pt-10" aria-label="Loading topic">
        <div className="skeleton h-12 w-3/4 mb-6" />
        <div className="skeleton h-4 w-1/2 mb-10" />
      </div>
    );
  }

  if (topic.isError) {
    const notFound = topic.error instanceof ApiError && topic.error.status === 404;
    return (
      <div className="px-4 lg:px-8 pt-10">
        {notFound ? (
          <div className="text-center py-20">
            <h1 className="display-tight text-3xl text-ink mb-3">Topic not found</h1>
            <Link to="/radar" className="label-sc text-vermillion">← Back to radar</Link>
          </div>
        ) : (
          <p className="font-body text-muted-foreground">Couldn't load this topic.</p>
        )}
      </div>
    );
  }

  const t = topic.data!;

  function onTogglePause() {
    update.mutate(
      { id: t.id, input: { isActive: !t.isActive } },
      {
        onError: () => toast.error("Could not update — please try again"),
      },
    );
  }

  function onDeleteConfirmed() {
    del.mutate(t.id, {
      onSuccess: () => {
        toast.success("Topic deleted");
        navigate("/radar");
      },
      onError: () => toast.error("Could not delete — please try again"),
    });
  }

  return (
    <div className="px-4 lg:px-8 pt-8 pb-20">
      <Link
        to="/radar"
        className="label-sc text-muted-foreground hover:text-vermillion inline-block mb-10"
      >
        ← Back to radar
      </Link>

      <TopicHeader
        topic={t}
        onEdit={() => setEditOpen(true)}
        onTogglePause={onTogglePause}
        onDelete={() => setDeleteOpen(true)}
        togglePending={update.isPending}
      />
      <StatsLine topic={t} />

      <div className="rule-thick my-8" />

      <div className="flex items-center gap-4 mb-8">
        <div className="label-sc-lg text-ink">Found entries</div>
        <div className="flex-1 rule-dotted" />
        <div className="label-sc text-muted-foreground">
          {matches.items.length} shown
        </div>
      </div>

      {matches.isLoading ? (
        <p className="font-body italic text-muted-foreground">Loading…</p>
      ) : matches.items.length === 0 ? (
        <EmptyTopicMatches />
      ) : (
        <>
          <MatchGrid matches={matches.items} />
          {matches.hasMore && (
            <div className="flex justify-center mt-10">
              <Button
                variant="outline"
                onClick={() => matches.fetchNextPage()}
                disabled={matches.isFetchingNextPage}
              >
                {matches.isFetchingNextPage ? "Loading…" : "Load more"}
              </Button>
            </div>
          )}
        </>
      )}

      <EditTopicDialog topic={t} open={editOpen} onOpenChange={setEditOpen} />
      <DeleteTopicConfirm
        open={deleteOpen}
        topicName={t.name}
        pending={del.isPending}
        onOpenChange={setDeleteOpen}
        onConfirm={onDeleteConfirmed}
      />
    </div>
  );
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/routes/radar.$topicId.tsx
git commit -m "feat(radar/web): add /radar/$topicId route with Edit/Pause/Delete"
```

---

## Task 22: Route `/radar/matches/$matchId`

**Files:**
- Create: `web/src/routes/radar.matches.$matchId.tsx`

- [ ] **Step 1: Create file**

```tsx
import { useParams } from "react-router";
import { MatchReader } from "@/features/radar/components/MatchReader";

export default function MatchRoute() {
  const { matchId } = useParams();
  const id = Number(matchId);
  if (Number.isNaN(id) || id <= 0) {
    return <div className="p-8 font-body text-muted-foreground">Invalid match id.</div>;
  }
  return <MatchReader matchId={id} />;
}
```

- [ ] **Step 2: Commit**

```bash
git add web/src/routes/radar.matches.$matchId.tsx
git commit -m "feat(radar/web): add /radar/matches/$matchId route"
```

---

## Task 23: Register routes, enable Sidebar nav, mount dialog

**Files:**
- Modify: `web/src/App.tsx`
- Modify: `web/src/shared/layout/Sidebar.tsx`
- Modify: `web/src/routes/__app.tsx`

- [ ] **Step 1: Update `App.tsx` to register radar routes**

Find the existing import block and add:

```tsx
import RadarListRoute from "./routes/radar._index";
import TopicRoute from "./routes/radar.$topicId";
import MatchRoute from "./routes/radar.matches.$matchId";
```

In the `AppLayout` children array, add three entries after the library routes:

```tsx
              { path: "radar", element: <RadarListRoute /> },
              { path: "radar/:topicId", element: <TopicRoute /> },
              { path: "radar/matches/:matchId", element: <MatchRoute /> },
```

- [ ] **Step 2: Update `Sidebar.tsx`** — remove the `disabled: true` on the Radar entry. Change:

```ts
  { to: "/radar", label: "Radar", number: "02", disabled: true },
```

to:

```ts
  { to: "/radar", label: "Radar", number: "02" },
```

Also update the type if needed (the `disabled` field becomes optional / missing — strip the renderer branch if it dead-ends; the existing `if (item.disabled)` simply won't trigger, no source change strictly needed beyond the entry).

- [ ] **Step 3: Mount `NewTopicDialog` in `__app.tsx`**

```tsx
import { Outlet } from "react-router";
import { AppShell } from "@/shared/layout/AppShell";
import { AddLinkDialog } from "@/features/library/components/AddLinkDialog";
import { NewTopicDialog } from "@/features/radar/components/NewTopicDialog";

export default function AppLayout() {
  return (
    <AppShell>
      <Outlet />
      <AddLinkDialog />
      <NewTopicDialog />
    </AppShell>
  );
}
```

- [ ] **Step 4: Build + typecheck**

```bash
cd /home/ismd/coding/linktheca/web
npm run build
```

Expected: PASS without typecheck errors.

- [ ] **Step 5: Run full web test suite**

```bash
npm test -- --run
```

Expected: ALL PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/App.tsx web/src/shared/layout/Sidebar.tsx web/src/routes/__app.tsx
git commit -m "feat(radar/web): wire radar routes, enable sidebar nav, mount dialog"
```

---

# Phase 5 — Wrap-up

## Task 24: Full test suites + manual smoke

**Files:**
- None modified

- [ ] **Step 1: Run full Go test suite**

```bash
cd /home/ismd/coding/linktheca
go test ./... -count=1
```

Expected: ALL PASS.

- [ ] **Step 2: Run full web test suite**

```bash
cd /home/ismd/coding/linktheca/web
npm test -- --run
```

Expected: ALL PASS.

- [ ] **Step 3: Lint web**

```bash
cd /home/ismd/coding/linktheca/web
npm run lint
```

Expected: clean (or only pre-existing warnings).

- [ ] **Step 4: Manual smoke in browser**

Boot dev environment (Go server + web dev server + radar enabled). Then walk through:

1. Open `/radar` → header shows "Last sweep · …" or "Awaiting first sweep". Topics list renders (empty state if no topics).
2. Click "+ New topic" → dialog opens. Submit with empty fields → see "Name is required". Type a real topic → submit → toast "Saved" → topic appears in list.
3. Click a topic card → `/radar/$topicId` opens. Header, stats line, "Found entries" section render. If matches exist, MatchGrid renders.
4. Click a match card → `/radar/matches/$matchId` opens. Reader displays title/source/summary. Open dev tools → confirm exactly one `PATCH /radar/matches/{id}` request fires. Refresh the same URL → no new PATCH, page loads directly.
5. Back to Topic view → newCount decremented; "new" stamp gone from that card.
6. Click "Save to library" in reader → toast "Saved". Open `/library` → article present.
7. Click "Edit" on Topic header → dialog opens with current values. Change description → save → header reflects new description.
8. Click "Pause" → Topic moves visually to "Paused" section on Radar list (opacity-60, "Paused" label). Click "Resume" → returns to "On the radar".
9. Click "Delete" → confirm dialog → confirm → redirect to `/radar`, toast "Topic deleted", topic gone.
10. Stop the embedder (or set `LINKTHECA_EMBEDDER_URL` to something invalid), try to create a topic → toast "Embedder is currently unavailable…", dialog stays open with data preserved.
11. Restart server with `LINKTHECA_RADAR_ENABLED=false`. Open `/radar` → "Radar is disabled in this instance." Sidebar still shows Radar entry.

- [ ] **Step 5: Mark plan complete**

If all manual checks pass, the feature is ready for review. Push the branch and open a PR referencing the spec `docs/superpowers/specs/2026-05-18-radar-frontend-design.md`.

---

## Self-review

**Spec coverage:**

| Spec section | Covered by |
|---|---|
| Backend addition `GET /radar/matches/{id}` | Tasks 1-5 |
| Sidebar enable | Task 23 |
| `features/radar/types.ts` | Task 6 |
| Zod schemas + mappers | Task 7 |
| API functions | Task 8 |
| Query hooks | Task 9 |
| Mutation hooks (incl. 503 propagation, optimistic) | Task 10 |
| Time helpers | Task 11 |
| TopicCard | Task 12 |
| TopicGrid / SkeletonCard / EmptyTopicList | Task 13 |
| MatchCard / MatchGrid / EmptyTopicMatches | Task 14 |
| StatsLine / TopicHeader | Task 15 |
| NewTopicDialog (validation, 503 toast, dialog stays open) | Task 16 |
| EditTopicDialog (partial PATCH, 503 toast) | Task 17 |
| DeleteTopicConfirm | Task 18 |
| MatchReader (auto-mark-seen, empty-summary fallback) | Task 19 |
| Radar list route | Task 20 |
| Topic view route (incl. Pause/Delete + Edit dialog wiring) | Task 21 |
| Match reader route | Task 22 |
| App.tsx + Sidebar + __app.tsx integration | Task 23 |
| Radar disabled state | Task 20 (RadarDisabled branch on `radar_disabled` error) |
| Manual smoke checklist | Task 24 |

All spec sections mapped to tasks. No gaps.

**No placeholders:** every step has concrete code, file paths, and commands. Test code uses real schemas, real bodies, real assertions.

**Type consistency:** `TopicWithStats`, `MatchView`, `MatchState`, `MatchFilters`, `radarKeys` used identically across tasks. Mutation hooks `useCreateTopic`/`useUpdateTopic`/`useDeleteTopic`/`useMarkMatchSeen` referenced by name and used consistently. API functions match the names in mutation tests. The mock `mapTopic` helper in `use-mutations.test.tsx` is consistent with `mapTopicWithStats` in `schemas.ts` (kept local to avoid test-internal coupling, but produces same shape).