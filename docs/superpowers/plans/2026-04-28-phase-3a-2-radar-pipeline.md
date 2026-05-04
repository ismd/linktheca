# Phase 3a-2: Radar Pipeline Backend (service, HTTP, crawler, jobs) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire the Radar backend pipeline end-to-end on top of the store layer from phase 3a (plan 2026-04-22). After this phase a server with `LINKTHECA_RADAR_ENABLED=true` exposes `POST /radar/topics`, `POST /radar/feeds`, `POST /radar/subscriptions`, runs a River-backed pipeline (Scheduler → CrawlFeed → EmbedFinding → MatchFinding) against TEI, and writes matches into `radar_topic_matches`. With `LINKTHECA_RADAR_ENABLED=false` the routes return 501 and no River workers run.

**Architecture:** `internal/radar/service.go` exposes `CreateTopic`, `AddFeed`, `Subscribe` and depends on `radar.StoreAPI` (interface declared in service for unit-mocking) plus `embeddings.Client`. `internal/radar/http.go` adds three POSTs and a `DisabledHandler`. `internal/radar/crawler/` wraps `mmcdole/gofeed` behind a `Fetcher` interface for testability. `internal/radar/jobs/` owns River setup: four `JobArgs`/worker pairs (`ScheduleCrawls`, `CrawlFeed`, `EmbedFinding`, `MatchFinding`) and a `Register` helper that takes service + embedder + store and returns a `*river.Workers` plus periodic-job spec. `internal/server/server.go` switches on `cfg.RadarEnabled`: when true, builds TEI client, River client, registers workers, and mounts `/radar/*`. `cmd/linktheca-server/main.go` runs goose + river migrations on startup and orders shutdown HTTP → River → pool.

**Tech Stack:** Go 1.26+, `go-chi/chi/v5`, `jackc/pgx/v5`, `pgvector/pgvector-go`, `mmcdole/gofeed`, `riverqueue/river` + `riverdriver/riverpgxv5` + `river/rivermigrate`, `stretchr/testify`. Existing `internal/core/embeddings` package supplies the `Client` interface and `FakeEmbedder` for tests. New infra: HuggingFace TEI (`ghcr.io/huggingface/text-embeddings-inference:cpu-1.9` with `BAAI/bge-m3`) added to `compose.dev.yaml`.

**Module path:** `github.com/ismd/linktheca`

**Working directory:** `/home/ismd/coding/linktheca`

**Spec reference:** `docs/superpowers/specs/2026-04-22-phase-3a-radar-pipeline-design.md`

---

## File structure created or modified by this phase

```
linktheca/
├── internal/
│   ├── radar/
│   │   ├── service.go                  # NEW
│   │   ├── service_test.go             # NEW (unit, mock store + FakeEmbedder)
│   │   ├── http.go                     # NEW (3 POSTs + DisabledHandler)
│   │   ├── http_test.go                # NEW (validation, mock service)
│   │   ├── integration_test.go         # NEW (HTTP→service→store via testdb + FakeEmbedder)
│   │   ├── crawler/
│   │   │   ├── crawler.go              # NEW (Fetcher iface, HTTPFetcher, Parser)
│   │   │   └── crawler_test.go         # NEW
│   │   └── jobs/
│   │       ├── jobs.go                 # NEW (args types, Register helper)
│   │       ├── crawl_feed.go           # NEW
│   │       ├── embed_finding.go        # NEW
│   │       ├── match_finding.go        # NEW
│   │       ├── scheduler.go            # NEW
│   │       ├── jobs_test.go            # NEW (integration via testdb)
│   │       └── smoke_test.go           # NEW (//go:build smoke)
│   └── server/
│       ├── server.go                   # MODIFIED (TEI + River + radar wiring)
│       └── server_test.go              # MODIFIED (TestRadarDisabled)
│
├── cmd/linktheca-server/main.go        # MODIFIED (River migrations + start/stop)
├── compose.dev.yaml                    # MODIFIED (+ tei service)
├── Makefile                            # MODIFIED (smoke-radar target)
└── go.mod / go.sum                     # MODIFIED (+ gofeed, river, rivermigrate)
```

**Out of scope (phase 3b or later):** any `GET /radar/*`, Update/Delete on topics/feeds/subscriptions, reader view for findings, full content extraction for findings, frontend, CLI binary (that's phase 3a-3).

---

## Conventions for every task

- **TDD everywhere.** Failing test → minimal implementation → green → commit.
- **Commit after each task.** Stage explicit files; never `git add .`.
- **Run from repo root** (`/home/ismd/coding/linktheca`).
- **Commit messages:** `<type>(<scope>): <subject>`, e.g. `feat(radar): add CreateTopic service method`.
- **Errors:** sentinel errors at top of `radar/types.go`. Wrap with `%w` for context. Reuse the existing `ErrNotFound`, `ErrDuplicate`, `ErrFeedNotFound`.
- **Context:** every store/service method takes `ctx context.Context` first.
- **Integer types:** `int64` for BIGINT PKs, `int` for `users.id` source columns scanned as `int64` is also fine (existing convention).
- **Mock stores in tests:** plain hand-written structs implementing `radar.StoreAPI` (see `internal/library/service_test.go` for the pattern).

### Quick reference — existing helpers and types

- `radar.NewStore(pool) *radar.Store` — already exists with all needed methods (`CreateTopic`, `UpdateTopicEmbedding`, `AddFeed`, `Subscribe`, `ListDueFeeds`, `GetFeedForFetch`, `MarkFeedFetched`, `MarkFeedError`, `UpsertFinding`, `GetFindingByExternalID`, `UpdateFindingEmbedding`, `MatchFindingToTopics`, `GetFindingForEmbed`, `FindingForEmbed`, `FeedFetchState`).
- `embeddings.Client` interface, `embeddings.NewTEIClient(url, timeout)`, `embeddings.FakeEmbedder{Dim:1024}`.
- `coreauth.UserID(ctx) int64`, `coreauth.IsAdmin(ctx) bool`, `coreauth.RequireUser(issuer)`, `coreauth.RequireAdmin`.
- `httpx.WriteJSON(w, status, body)`, `httpx.WriteError(w, status, code, msg)`.
- `testdb.New(t) *pgxpool.Pool` — schema-per-test Postgres with all migrations applied.

### River API note

River v0 evolves; before writing worker code in tasks 13+, run `go doc github.com/riverqueue/river` and `go doc github.com/riverqueue/river/riverdriver/riverpgxv5` to confirm signatures. The patterns shown in this plan (`river.WorkerDefaults`, `river.AddWorker`, `river.NewClient`, `client.Insert`, `river.NewPeriodicJob`) match v0.x at the time of writing. If the API has shifted, adjust the call sites and keep the structure (one args type per worker, one worker type per file).

---

## Part A: Service layer

### Task 1: Add gofeed and River dependencies

**Files:**
- Modify: `go.mod`, `go.sum`

- [x] **Step 1: Add the deps**

```bash
go get github.com/mmcdole/gofeed
go get github.com/riverqueue/river
go get github.com/riverqueue/river/riverdriver/riverpgxv5
go get github.com/riverqueue/river/rivermigrate
```

- [x] **Step 2: Tidy and build**

```bash
go mod tidy
go build ./...
```

Expected: success.

- [x] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore(deps): add gofeed and river for radar pipeline"
```

---

### Task 2: Service skeleton + CreateTopic with embedding

**Files:**
- Create: `internal/radar/service.go`
- Create: `internal/radar/service_test.go`

- [x] **Step 1: Write the failing test**

Create `internal/radar/service_test.go`:

```go
package radar_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/ismd/linktheca/internal/radar"
	"github.com/pgvector/pgvector-go"
	"github.com/stretchr/testify/require"
)

// --- mock store ---

type mockStore struct {
	topics        map[int64]*radar.Topic
	topicEmb      map[int64]pgvector.Vector
	feeds         map[int64]*radar.Feed
	feedsByURL    map[string]*radar.Feed
	subs          map[string]*radar.Subscription
	nextTopicID   int64
	nextFeedID    int64
	createTopicErr error
	addFeedErr     error
	subscribeErr   error
	updateEmbErr   error
}

func newMockStore() *mockStore {
	return &mockStore{
		topics:     make(map[int64]*radar.Topic),
		topicEmb:   make(map[int64]pgvector.Vector),
		feeds:      make(map[int64]*radar.Feed),
		feedsByURL: make(map[string]*radar.Feed),
		subs:       make(map[string]*radar.Subscription),
	}
}

func (m *mockStore) CreateTopic(_ context.Context, p radar.CreateTopicParams) (*radar.Topic, error) {
	if m.createTopicErr != nil {
		return nil, m.createTopicErr
	}
	m.nextTopicID++
	t := &radar.Topic{
		ID: m.nextTopicID, UserID: p.UserID, Name: p.Name,
		Description: p.Description, MatchThreshold: p.MatchThreshold,
		IsActive: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	m.topics[t.ID] = t
	return t, nil
}

func (m *mockStore) UpdateTopicEmbedding(_ context.Context, topicID int64, vec pgvector.Vector) error {
	if m.updateEmbErr != nil {
		return m.updateEmbErr
	}
	t, ok := m.topics[topicID]
	if !ok {
		return radar.ErrNotFound
	}
	m.topicEmb[topicID] = vec
	t.HasEmbedding = true
	return nil
}

func (m *mockStore) AddFeed(_ context.Context, p radar.AddFeedParams) (*radar.Feed, error) {
	if m.addFeedErr != nil {
		return nil, m.addFeedErr
	}
	if _, ok := m.feedsByURL[p.URL]; ok {
		return nil, radar.ErrDuplicate
	}
	m.nextFeedID++
	f := &radar.Feed{
		ID: m.nextFeedID, URL: p.URL, Kind: p.Kind,
		FetchIntervalSeconds: p.FetchIntervalSeconds, IsActive: true,
		CreatedAt: time.Now(),
	}
	m.feeds[f.ID] = f
	m.feedsByURL[p.URL] = f
	return f, nil
}

func (m *mockStore) Subscribe(_ context.Context, userID, feedID int64) (*radar.Subscription, error) {
	if m.subscribeErr != nil {
		return nil, m.subscribeErr
	}
	if _, ok := m.feeds[feedID]; !ok {
		return nil, radar.ErrFeedNotFound
	}
	key := keyOf(userID, feedID)
	if existing, ok := m.subs[key]; ok {
		return existing, nil
	}
	sub := &radar.Subscription{UserID: userID, FeedID: feedID, CreatedAt: time.Now()}
	m.subs[key] = sub
	return sub, nil
}

func keyOf(u, f int64) string { return string(rune(u)) + ":" + string(rune(f)) }

// Compile-time check.
var _ radar.StoreAPI = (*mockStore)(nil)

// --- tests ---

func TestService_CreateTopic_Success(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	thr := float32(0.8)
	topic, err := svc.CreateTopic(context.Background(), 7, radar.CreateTopicRequest{
		Name: "AI", Description: "machine learning research", MatchThreshold: &thr,
	})
	require.NoError(t, err)
	require.Equal(t, int64(7), topic.UserID)
	require.Equal(t, "AI", topic.Name)
	require.Equal(t, float32(0.8), topic.MatchThreshold)
	require.True(t, topic.HasEmbedding, "embedding must be set after CreateTopic")
}

func TestService_CreateTopic_DefaultThreshold(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	topic, err := svc.CreateTopic(context.Background(), 1, radar.CreateTopicRequest{
		Name: "x", Description: "ten chars long",
	})
	require.NoError(t, err)
	require.Equal(t, float32(0.75), topic.MatchThreshold)
}

func TestService_CreateTopic_Validation(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	// empty name
	_, err := svc.CreateTopic(context.Background(), 1, radar.CreateTopicRequest{
		Name: "", Description: "ten chars long",
	})
	require.ErrorIs(t, err, radar.ErrInvalidInput)

	// short description
	_, err = svc.CreateTopic(context.Background(), 1, radar.CreateTopicRequest{
		Name: "x", Description: "short",
	})
	require.ErrorIs(t, err, radar.ErrInvalidInput)

	// out-of-range threshold
	bad := float32(1.5)
	_, err = svc.CreateTopic(context.Background(), 1, radar.CreateTopicRequest{
		Name: "x", Description: "ten chars long", MatchThreshold: &bad,
	})
	require.ErrorIs(t, err, radar.ErrInvalidInput)
}

func TestService_CreateTopic_EmbedderError(t *testing.T) {
	store := newMockStore()
	emb := &errEmbedder{err: errors.New("boom")}
	svc := radar.NewService(store, emb)

	_, err := svc.CreateTopic(context.Background(), 1, radar.CreateTopicRequest{
		Name: "x", Description: "ten chars long",
	})
	require.ErrorIs(t, err, radar.ErrEmbedderUnavailable)
}

type errEmbedder struct{ err error }

func (e *errEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, e.err
}
```

- [x] **Step 2: Run — expect compile failure**

```bash
go test ./internal/radar/... -run TestService_
```

Expected: build error (`NewService`, `ErrInvalidInput`, `ErrEmbedderUnavailable`, `StoreAPI` undefined).

- [x] **Step 3: Add new sentinels to types.go**

Edit `internal/radar/types.go` — extend the var block:

```go
var (
	ErrNotFound            = errors.New("not found")
	ErrDuplicate           = errors.New("duplicate")
	ErrFeedNotFound        = errors.New("feed not found")
	ErrInvalidInput        = errors.New("invalid input")
	ErrEmbedderUnavailable = errors.New("embedder unavailable")
)
```

- [x] **Step 4: Create service.go**

Create `internal/radar/service.go`:

```go
package radar

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/pgvector/pgvector-go"
)

const defaultMatchThreshold = 0.75

type StoreAPI interface {
	CreateTopic(ctx context.Context, p CreateTopicParams) (*Topic, error)
	UpdateTopicEmbedding(ctx context.Context, topicID int64, vec pgvector.Vector) error
	AddFeed(ctx context.Context, p AddFeedParams) (*Feed, error)
	Subscribe(ctx context.Context, userID, feedID int64) (*Subscription, error)
}

type Service struct {
	store    StoreAPI
	embedder embeddings.Client
}

func NewService(store StoreAPI, embedder embeddings.Client) *Service {
	return &Service{store: store, embedder: embedder}
}

// CreateTopic validates the request, persists the topic, and synchronously
// computes its embedding. If the embedder is unavailable the topic stays in
// the database without an embedding and is silently skipped by MatchFindingJob.
func (s *Service) CreateTopic(ctx context.Context, userID int64, req CreateTopicRequest) (*Topic, error) {
	name := strings.TrimSpace(req.Name)
	desc := strings.TrimSpace(req.Description)
	if name == "" || len(name) > 200 {
		return nil, fmt.Errorf("%w: name must be 1..200 chars", ErrInvalidInput)
	}
	if len(desc) < 10 || len(desc) > 2000 {
		return nil, fmt.Errorf("%w: description must be 10..2000 chars", ErrInvalidInput)
	}
	threshold := float32(defaultMatchThreshold)
	if req.MatchThreshold != nil {
		threshold = *req.MatchThreshold
		if threshold < 0 || threshold > 1 {
			return nil, fmt.Errorf("%w: match_threshold must be in [0,1]", ErrInvalidInput)
		}
	}

	topic, err := s.store.CreateTopic(ctx, CreateTopicParams{
		UserID: userID, Name: name, Description: desc, MatchThreshold: threshold,
	})
	if err != nil {
		return nil, fmt.Errorf("create topic: %w", err)
	}

	vec, err := s.embedder.Embed(ctx, desc)
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
	return topic, nil
}
```

- [x] **Step 5: Run tests — expect pass**

```bash
go test ./internal/radar/... -run TestService_CreateTopic -v
```

Expected: 4 PASS.

- [x] **Step 6: Commit**

```bash
git add internal/radar/
git commit -m "feat(radar): add Service.CreateTopic with synchronous embedding"
```

---

### Task 3: Service.AddFeed

**Files:**
- Modify: `internal/radar/service.go`
- Modify: `internal/radar/service_test.go`

- [x] **Step 1: Write failing tests**

Append to `internal/radar/service_test.go`:

```go
func TestService_AddFeed_Defaults(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	feed, err := svc.AddFeed(context.Background(), radar.AddFeedRequest{
		URL: "https://example.com/feed.xml",
	})
	require.NoError(t, err)
	require.Equal(t, "rss", feed.Kind)
	require.Equal(t, 3600, feed.FetchIntervalSeconds)
}

func TestService_AddFeed_Validation(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	_, err := svc.AddFeed(context.Background(), radar.AddFeedRequest{URL: ""})
	require.ErrorIs(t, err, radar.ErrInvalidInput)

	_, err = svc.AddFeed(context.Background(), radar.AddFeedRequest{URL: "not-a-url"})
	require.ErrorIs(t, err, radar.ErrInvalidInput)

	bad := "weird"
	_, err = svc.AddFeed(context.Background(), radar.AddFeedRequest{
		URL: "https://x.example/f", Kind: &bad,
	})
	require.ErrorIs(t, err, radar.ErrInvalidInput)

	tooFast := 60
	_, err = svc.AddFeed(context.Background(), radar.AddFeedRequest{
		URL: "https://x.example/f", FetchIntervalSeconds: &tooFast,
	})
	require.ErrorIs(t, err, radar.ErrInvalidInput)
}

func TestService_AddFeed_Duplicate(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	_, err := svc.AddFeed(context.Background(), radar.AddFeedRequest{
		URL: "https://x.example/dup",
	})
	require.NoError(t, err)
	_, err = svc.AddFeed(context.Background(), radar.AddFeedRequest{
		URL: "https://x.example/dup",
	})
	require.ErrorIs(t, err, radar.ErrDuplicate)
}
```

- [x] **Step 2: Run — expect compile failure**

- [x] **Step 3: Implement AddFeed**

Append to `internal/radar/service.go`:

```go
const (
	defaultFetchIntervalSeconds = 3600
	minFetchIntervalSeconds     = 300
	maxFetchIntervalSeconds     = 86400
)

func (s *Service) AddFeed(ctx context.Context, req AddFeedRequest) (*Feed, error) {
	url := strings.TrimSpace(req.URL)
	if url == "" || len(url) > 2000 {
		return nil, fmt.Errorf("%w: url must be 1..2000 chars", ErrInvalidInput)
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return nil, fmt.Errorf("%w: url must be http(s)", ErrInvalidInput)
	}
	kind := "rss"
	if req.Kind != nil {
		kind = *req.Kind
		if kind != "rss" && kind != "atom" {
			return nil, fmt.Errorf("%w: kind must be rss|atom", ErrInvalidInput)
		}
	}
	interval := defaultFetchIntervalSeconds
	if req.FetchIntervalSeconds != nil {
		interval = *req.FetchIntervalSeconds
		if interval < minFetchIntervalSeconds || interval > maxFetchIntervalSeconds {
			return nil, fmt.Errorf("%w: fetch_interval_seconds must be %d..%d",
				ErrInvalidInput, minFetchIntervalSeconds, maxFetchIntervalSeconds)
		}
	}

	feed, err := s.store.AddFeed(ctx, AddFeedParams{
		URL: url, Kind: kind, FetchIntervalSeconds: interval,
	})
	if err != nil {
		return nil, err
	}
	return feed, nil
}
```

- [x] **Step 4: Run tests — expect pass**

```bash
go test ./internal/radar/... -run TestService_ -v
```

- [x] **Step 5: Commit**

```bash
git add internal/radar/
git commit -m "feat(radar): add Service.AddFeed with validation"
```

---

### Task 4: Service.Subscribe

**Files:**
- Modify: `internal/radar/service.go`
- Modify: `internal/radar/service_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/radar/service_test.go`:

```go
func TestService_Subscribe_Success(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	feed, err := svc.AddFeed(context.Background(), radar.AddFeedRequest{
		URL: "https://x.example/sub",
	})
	require.NoError(t, err)

	sub, err := svc.Subscribe(context.Background(), 42, radar.SubscribeRequest{FeedID: feed.ID})
	require.NoError(t, err)
	require.Equal(t, int64(42), sub.UserID)
	require.Equal(t, feed.ID, sub.FeedID)
}

func TestService_Subscribe_Idempotent(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	feed, _ := svc.AddFeed(context.Background(), radar.AddFeedRequest{URL: "https://x.example/i"})

	sub1, _ := svc.Subscribe(context.Background(), 1, radar.SubscribeRequest{FeedID: feed.ID})
	sub2, err := svc.Subscribe(context.Background(), 1, radar.SubscribeRequest{FeedID: feed.ID})
	require.NoError(t, err)
	require.Equal(t, sub1.CreatedAt, sub2.CreatedAt)
}

func TestService_Subscribe_FeedMissing(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	_, err := svc.Subscribe(context.Background(), 1, radar.SubscribeRequest{FeedID: 999})
	require.ErrorIs(t, err, radar.ErrFeedNotFound)
}

func TestService_Subscribe_Validation(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	_, err := svc.Subscribe(context.Background(), 1, radar.SubscribeRequest{FeedID: 0})
	require.ErrorIs(t, err, radar.ErrInvalidInput)
}
```

- [ ] **Step 2: Run — expect failure**

- [ ] **Step 3: Implement Subscribe**

Append to `internal/radar/service.go`:

```go
func (s *Service) Subscribe(ctx context.Context, userID int64, req SubscribeRequest) (*Subscription, error) {
	if req.FeedID <= 0 {
		return nil, fmt.Errorf("%w: feed_id must be positive", ErrInvalidInput)
	}
	return s.store.Subscribe(ctx, userID, req.FeedID)
}
```

- [ ] **Step 4: Run — expect pass**

```bash
go test ./internal/radar/... -run TestService_ -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/radar/
git commit -m "feat(radar): add Service.Subscribe"
```

---

## Part B: HTTP handlers

### Task 5: HTTP — CreateTopic + DisabledHandler

**Files:**
- Create: `internal/radar/http.go`
- Create: `internal/radar/http_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/radar/http_test.go`:

```go
package radar_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/ismd/linktheca/internal/radar"
	"github.com/stretchr/testify/require"
)

// userOnlyContext attaches a user_id to ctx without parsing a JWT.
func userOnlyContext(ctx context.Context, userID int64, isAdmin bool) context.Context {
	return coreauthWithUser(ctx, userID, isAdmin)
}

func TestHTTP_CreateTopic_201(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	body, _ := json.Marshal(radar.CreateTopicRequest{
		Name: "AI", Description: "machine learning research and products",
	})
	req := httptest.NewRequest(http.MethodPost, "/radar/topics", bytes.NewReader(body))
	req = req.WithContext(userOnlyContext(req.Context(), 7, false))
	rec := httptest.NewRecorder()

	h.CreateTopicHandler()(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var got radar.Topic
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
	require.Equal(t, "AI", got.Name)
	require.True(t, got.HasEmbedding)
}

func TestHTTP_CreateTopic_400_BadJSON(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodPost, "/radar/topics",
		strings.NewReader(`{"name":}`))
	req = req.WithContext(userOnlyContext(req.Context(), 1, false))
	rec := httptest.NewRecorder()
	h.CreateTopicHandler()(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHTTP_CreateTopic_400_Validation(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	body, _ := json.Marshal(radar.CreateTopicRequest{Name: "", Description: "short"})
	req := httptest.NewRequest(http.MethodPost, "/radar/topics", bytes.NewReader(body))
	req = req.WithContext(userOnlyContext(req.Context(), 1, false))
	rec := httptest.NewRecorder()
	h.CreateTopicHandler()(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHTTP_CreateTopic_503_EmbedderDown(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &errEmbedder{err: errors.New("connection refused")})
	h := radar.NewHTTP(svc)

	body, _ := json.Marshal(radar.CreateTopicRequest{
		Name: "x", Description: "ten chars long enough",
	})
	req := httptest.NewRequest(http.MethodPost, "/radar/topics", bytes.NewReader(body))
	req = req.WithContext(userOnlyContext(req.Context(), 1, false))
	rec := httptest.NewRecorder()
	h.CreateTopicHandler()(rec, req)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestHTTP_DisabledHandler_501(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/radar/topics", nil)
	radar.DisabledHandler(rec, req)
	require.Equal(t, http.StatusNotImplemented, rec.Code)
	require.Contains(t, rec.Body.String(), "radar_disabled")
}
```

Add a tiny helper file `internal/radar/coreauth_helper_test.go` to bridge test code with the unexported `WithUser` (it's exported from `internal/core/auth`, so the test can call it directly — this helper is a thin alias for clarity):

```go
package radar_test

import (
	"context"

	coreauth "github.com/ismd/linktheca/internal/core/auth"
)

func coreauthWithUser(ctx context.Context, userID int64, isAdmin bool) context.Context {
	return coreauth.WithUser(ctx, userID, isAdmin)
}
```

- [ ] **Step 2: Run — expect compile failure**

```bash
go test ./internal/radar/... -run TestHTTP_
```

Expected: build error (`NewHTTP`, `CreateTopicHandler`, `DisabledHandler` undefined).

- [ ] **Step 3: Implement http.go**

Create `internal/radar/http.go`:

```go
package radar

import (
	"encoding/json"
	"errors"
	"net/http"

	coreauth "github.com/ismd/linktheca/internal/core/auth"
	"github.com/ismd/linktheca/internal/core/httpx"
)

type HTTP struct {
	svc *Service
}

func NewHTTP(svc *Service) *HTTP { return &HTTP{svc: svc} }

func (h *HTTP) CreateTopicHandler() http.HandlerFunc { return h.createTopic }

func (h *HTTP) createTopic(w http.ResponseWriter, r *http.Request) {
	var req CreateTopicRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}
	userID := coreauth.UserID(r.Context())
	topic, err := h.svc.CreateTopic(r.Context(), userID, req)
	if err != nil {
		writeRadarError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, topic)
}

// DisabledHandler is mounted on /radar/* when LINKTHECA_RADAR_ENABLED=false.
// Returns 501 with a stable error code so the CLI can produce a useful message.
func DisabledHandler(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteError(w, http.StatusNotImplemented, "radar_disabled",
		"radar feature is disabled on this server")
}

func writeRadarError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", err.Error())
	case errors.Is(err, ErrEmbedderUnavailable):
		httpx.WriteError(w, http.StatusServiceUnavailable, "embedder_unavailable",
			"embedding service is unavailable, try again later")
	case errors.Is(err, ErrDuplicate):
		httpx.WriteError(w, http.StatusConflict, "duplicate", "resource already exists")
	case errors.Is(err, ErrFeedNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "feed not found")
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "")
	}
}
```

- [ ] **Step 4: Run tests — expect pass**

```bash
go test ./internal/radar/... -run TestHTTP_ -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/radar/
git commit -m "feat(radar): add CreateTopic HTTP handler and DisabledHandler"
```

---

### Task 6: HTTP — AddFeed + Subscribe handlers

**Files:**
- Modify: `internal/radar/http.go`
- Modify: `internal/radar/http_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/radar/http_test.go`:

```go
func TestHTTP_AddFeed_201(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	body, _ := json.Marshal(radar.AddFeedRequest{URL: "https://news.ycombinator.com/rss"})
	req := httptest.NewRequest(http.MethodPost, "/radar/feeds", bytes.NewReader(body))
	req = req.WithContext(userOnlyContext(req.Context(), 1, true))
	rec := httptest.NewRecorder()
	h.AddFeedHandler()(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var got radar.Feed
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
	require.Equal(t, "rss", got.Kind)
	require.True(t, got.IsActive)
}

func TestHTTP_AddFeed_409_Duplicate(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	body, _ := json.Marshal(radar.AddFeedRequest{URL: "https://dup.example/f"})
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/radar/feeds", bytes.NewReader(body))
		req = req.WithContext(userOnlyContext(req.Context(), 1, true))
		rec := httptest.NewRecorder()
		h.AddFeedHandler()(rec, req)
		if i == 0 {
			require.Equal(t, http.StatusCreated, rec.Code)
		} else {
			require.Equal(t, http.StatusConflict, rec.Code)
		}
	}
}

func TestHTTP_Subscribe_201(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	feed, _ := svc.AddFeed(context.Background(), radar.AddFeedRequest{URL: "https://s.example/f"})

	body, _ := json.Marshal(radar.SubscribeRequest{FeedID: feed.ID})
	req := httptest.NewRequest(http.MethodPost, "/radar/subscriptions", bytes.NewReader(body))
	req = req.WithContext(userOnlyContext(req.Context(), 99, false))
	rec := httptest.NewRecorder()
	h.SubscribeHandler()(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var sub radar.Subscription
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&sub))
	require.Equal(t, int64(99), sub.UserID)
}

func TestHTTP_Subscribe_404_FeedMissing(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	body, _ := json.Marshal(radar.SubscribeRequest{FeedID: 12345})
	req := httptest.NewRequest(http.MethodPost, "/radar/subscriptions", bytes.NewReader(body))
	req = req.WithContext(userOnlyContext(req.Context(), 1, false))
	rec := httptest.NewRecorder()
	h.SubscribeHandler()(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}
```

- [ ] **Step 2: Run — expect failure**

- [ ] **Step 3: Implement handlers**

Append to `internal/radar/http.go`:

```go
func (h *HTTP) AddFeedHandler() http.HandlerFunc    { return h.addFeed }
func (h *HTTP) SubscribeHandler() http.HandlerFunc  { return h.subscribe }

func (h *HTTP) addFeed(w http.ResponseWriter, r *http.Request) {
	var req AddFeedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}
	feed, err := h.svc.AddFeed(r.Context(), req)
	if err != nil {
		writeRadarError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, feed)
}

func (h *HTTP) subscribe(w http.ResponseWriter, r *http.Request) {
	var req SubscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}
	userID := coreauth.UserID(r.Context())
	sub, err := h.svc.Subscribe(r.Context(), userID, req)
	if err != nil {
		writeRadarError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, sub)
}
```

- [ ] **Step 4: Run — expect pass**

```bash
go test ./internal/radar/... -run TestHTTP_ -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/radar/
git commit -m "feat(radar): add AddFeed and Subscribe HTTP handlers"
```

---

### Task 7: HTTP integration test (real DB, real service)

**Files:**
- Create: `internal/radar/integration_test.go`

- [ ] **Step 1: Write the test**

Create `internal/radar/integration_test.go`:

```go
package radar_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	coreauth "github.com/ismd/linktheca/internal/core/auth"
	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/ismd/linktheca/internal/radar"
	"github.com/ismd/linktheca/internal/testing/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func seedRadarUser(t *testing.T, pool *pgxpool.Pool, isAdmin bool) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email, password_hash, display_name, is_admin)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		"u+"+t.Name()+"@example.com", "x", "Tester", isAdmin).Scan(&id)
	require.NoError(t, err)
	return id
}

func TestIntegrationRadarFlow(t *testing.T) {
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
		r.Post("/subscriptions", h.SubscribeHandler())
		r.Group(func(r chi.Router) {
			r.Use(coreauth.RequireAdmin)
			r.Post("/feeds", h.AddFeedHandler())
		})
	})

	// 1. Add a feed (admin).
	feedBody, _ := json.Marshal(radar.AddFeedRequest{URL: "https://hn.example/rss"})
	req := httptest.NewRequest(http.MethodPost, "/radar/feeds", bytes.NewReader(feedBody))
	req.Header.Set("Authorization", auth)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var feed radar.Feed
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&feed))

	// 2. Create a topic (any user).
	topicBody, _ := json.Marshal(radar.CreateTopicRequest{
		Name: "ML", Description: "machine learning and embeddings",
	})
	req = httptest.NewRequest(http.MethodPost, "/radar/topics", bytes.NewReader(topicBody))
	req.Header.Set("Authorization", auth)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var topic radar.Topic
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&topic))
	require.True(t, topic.HasEmbedding)

	// 3. Subscribe.
	subBody, _ := json.Marshal(radar.SubscribeRequest{FeedID: feed.ID})
	req = httptest.NewRequest(http.MethodPost, "/radar/subscriptions", bytes.NewReader(subBody))
	req.Header.Set("Authorization", auth)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	// 4. Verify rows in DB.
	var nFeed, nTopic, nSub int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM radar_feeds WHERE id=$1`, feed.ID).Scan(&nFeed))
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM radar_topics WHERE id=$1 AND embedding IS NOT NULL`,
		topic.ID).Scan(&nTopic))
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM radar_feed_subscriptions WHERE user_id=$1 AND feed_id=$2`,
		userID, feed.ID).Scan(&nSub))
	require.Equal(t, 1, nFeed)
	require.Equal(t, 1, nTopic)
	require.Equal(t, 1, nSub)
}

func TestIntegrationAddFeedRequiresAdmin(t *testing.T) {
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
		r.Group(func(r chi.Router) {
			r.Use(coreauth.RequireAdmin)
			r.Post("/feeds", h.AddFeedHandler())
		})
	})

	body, _ := json.Marshal(radar.AddFeedRequest{URL: "https://x.example/f"})
	req := httptest.NewRequest(http.MethodPost, "/radar/feeds", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)
}
```

- [ ] **Step 2: Run**

```bash
make dev-db
go test ./internal/radar/... -run TestIntegration -v
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/radar/integration_test.go
git commit -m "test(radar): add HTTP integration tests for radar flow"
```

---

## Part C: Crawler

### Task 8: Fetcher interface + HTTPFetcher

**Files:**
- Create: `internal/radar/crawler/crawler.go`
- Create: `internal/radar/crawler/crawler_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/radar/crawler/crawler_test.go`:

```go
package crawler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ismd/linktheca/internal/radar/crawler"
	"github.com/stretchr/testify/require"
)

func TestHTTPFetcher_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"abc"`)
		w.Header().Set("Last-Modified", "Wed, 22 Apr 2026 12:00:00 GMT")
		_, _ = w.Write([]byte("<rss/>"))
	}))
	defer srv.Close()

	f := crawler.NewHTTPFetcher(2 * time.Second)
	got, err := f.Fetch(context.Background(), srv.URL, "", "")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, got.StatusCode)
	require.Equal(t, `"abc"`, got.Etag)
	require.Equal(t, "Wed, 22 Apr 2026 12:00:00 GMT", got.LastModified)
	require.Contains(t, string(got.Body), "rss")
	require.False(t, got.NotModified)
}

func TestHTTPFetcher_304(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, `"abc"`, r.Header.Get("If-None-Match"))
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()

	f := crawler.NewHTTPFetcher(2 * time.Second)
	got, err := f.Fetch(context.Background(), srv.URL, `"abc"`, "")
	require.NoError(t, err)
	require.True(t, got.NotModified)
}

func TestHTTPFetcher_5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	f := crawler.NewHTTPFetcher(2 * time.Second)
	_, err := f.Fetch(context.Background(), srv.URL, "", "")
	require.Error(t, err)
}
```

- [ ] **Step 2: Run — expect failure**

```bash
go test ./internal/radar/crawler/...
```

- [ ] **Step 3: Implement HTTPFetcher**

Create `internal/radar/crawler/crawler.go`:

```go
// Package crawler turns RSS/Atom feed URLs into structured items. Fetcher
// abstracts the HTTP layer so jobs can be tested without real network IO.
package crawler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

type FetchResult struct {
	StatusCode   int
	Body         []byte
	Etag         string
	LastModified string
	NotModified  bool
}

type Fetcher interface {
	Fetch(ctx context.Context, url, etag, lastModified string) (*FetchResult, error)
}

type HTTPFetcher struct {
	client *http.Client
}

func NewHTTPFetcher(timeout time.Duration) *HTTPFetcher {
	return &HTTPFetcher{client: &http.Client{Timeout: timeout}}
}

func (f *HTTPFetcher) Fetch(ctx context.Context, url, etag, lastModified string) (*FetchResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "linktheca/0.1")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return &FetchResult{
			StatusCode:   resp.StatusCode,
			Etag:         resp.Header.Get("ETag"),
			LastModified: resp.Header.Get("Last-Modified"),
			NotModified:  true,
		}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 10 MiB cap
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	return &FetchResult{
		StatusCode:   resp.StatusCode,
		Body:         body,
		Etag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}, nil
}
```

- [ ] **Step 4: Run — expect pass**

```bash
go test ./internal/radar/crawler/... -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/radar/crawler/
git commit -m "feat(crawler): add Fetcher interface and HTTPFetcher"
```

---

### Task 9: Parser (gofeed wrapper)

**Files:**
- Modify: `internal/radar/crawler/crawler.go`
- Modify: `internal/radar/crawler/crawler_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/radar/crawler/crawler_test.go`:

```go
import "github.com/ismd/linktheca/internal/radar"

func TestParse_RSS(t *testing.T) {
	rss := []byte(`<?xml version="1.0"?>
<rss version="2.0"><channel><title>HN</title>
<item>
  <title>OpenAI ships</title>
  <link>https://news.example/post/1</link>
  <description>About models</description>
  <guid>hn:1</guid>
  <pubDate>Wed, 22 Apr 2026 12:00:00 GMT</pubDate>
</item>
</channel></rss>`)

	got, err := crawler.Parse(rss)
	require.NoError(t, err)
	require.Len(t, got, 1)

	upserts := crawler.ToUpserts(42, got)
	require.Equal(t, int64(42), upserts[0].FeedID)
	require.Equal(t, "https://news.example/post/1", upserts[0].URL)
	require.NotNil(t, upserts[0].ExternalID)
	require.Equal(t, "hn:1", *upserts[0].ExternalID)
	require.NotNil(t, upserts[0].Title)
	require.Equal(t, "OpenAI ships", *upserts[0].Title)
}

func TestParse_GarbageReturnsError(t *testing.T) {
	_, err := crawler.Parse([]byte("not xml"))
	require.Error(t, err)
}

// Compile-time check that ToUpserts returns []radar.FindingUpsert.
var _ []radar.FindingUpsert = crawler.ToUpserts(0, nil)
```

Also: add the import for `radar` to the existing import block at the top of `crawler_test.go`.

- [ ] **Step 2: Run — expect failure**

- [ ] **Step 3: Implement Parse and ToUpserts**

Append to `internal/radar/crawler/crawler.go`:

```go
import (
	"bytes"
	"strings"

	"github.com/ismd/linktheca/internal/radar"
	"github.com/mmcdole/gofeed"
)

// Parse turns raw feed XML into gofeed items. Caller passes the bytes from
// FetchResult.Body. Errors propagate from gofeed.
func Parse(body []byte) ([]*gofeed.Item, error) {
	parser := gofeed.NewParser()
	feed, err := parser.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("parse feed: %w", err)
	}
	return feed.Items, nil
}

// ToUpserts converts gofeed items into FindingUpsert structs ready for the store.
// External IDs come from item.GUID; if empty, the URL is used (most aggregators
// guarantee one or the other). Items without a URL are skipped.
func ToUpserts(feedID int64, items []*gofeed.Item) []radar.FindingUpsert {
	out := make([]radar.FindingUpsert, 0, len(items))
	for _, it := range items {
		if it == nil || strings.TrimSpace(it.Link) == "" {
			continue
		}
		up := radar.FindingUpsert{FeedID: feedID, URL: strings.TrimSpace(it.Link)}
		ext := strings.TrimSpace(it.GUID)
		if ext == "" {
			ext = up.URL
		}
		up.ExternalID = &ext
		if t := strings.TrimSpace(it.Title); t != "" {
			up.Title = &t
		}
		if d := strings.TrimSpace(it.Description); d != "" {
			up.Summary = &d
		}
		if it.PublishedParsed != nil {
			pp := *it.PublishedParsed
			up.PublishedAt = &pp
		}
		out = append(out, up)
	}
	return out
}
```

Reorganize the `import` block in `crawler.go` so `bytes`, `context`, `fmt`, `io`, `net/http`, `strings`, `time`, and the two third-party packages are all in one parenthesized block at the top.

- [ ] **Step 4: Run — expect pass**

```bash
go test ./internal/radar/crawler/... -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/radar/crawler/
git commit -m "feat(crawler): add gofeed-based Parse and ToUpserts"
```

---

## Part D: River jobs

### Task 10: Job args, Inserter, Deps

Before writing this task, run:

```bash
go doc github.com/riverqueue/river | head -100
go doc github.com/riverqueue/river WorkerDefaults
```

Use these to confirm exact field names and method signatures. The patterns in tasks 10–14 match River v0.x at the time of writing.

**Files:**
- Create: `internal/radar/jobs/jobs.go`

- [ ] **Step 1: Create args + Inserter + Deps types only**

This task defines the shared types that subsequent tasks will reference. The `Build` function and `NewClient` helper are added in Task 14, after all four workers exist.

Create `internal/radar/jobs/jobs.go`:

```go
// Package jobs hosts the four River workers that drive the Radar pipeline:
// ScheduleCrawls (periodic) → CrawlFeed → EmbedFinding → MatchFinding.
package jobs

import (
	"context"

	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/ismd/linktheca/internal/radar"
	"github.com/ismd/linktheca/internal/radar/crawler"
	"github.com/riverqueue/river"
)

// --- Args (one type per worker; River uses Kind() for routing) ---

type ScheduleCrawlsArgs struct{}

func (ScheduleCrawlsArgs) Kind() string { return "radar.schedule_crawls" }

type CrawlFeedArgs struct {
	FeedID int64 `json:"feed_id"`
}

func (CrawlFeedArgs) Kind() string { return "radar.crawl_feed" }

type EmbedFindingArgs struct {
	FindingID int64 `json:"finding_id"`
}

func (EmbedFindingArgs) Kind() string { return "radar.embed_finding" }

type MatchFindingArgs struct {
	FindingID int64 `json:"finding_id"`
}

func (MatchFindingArgs) Kind() string { return "radar.match_finding" }

// Inserter is the slice of river.Client that workers actually need.
// Defining an interface lets jobs_test.go run workers without a real client.
type Inserter interface {
	Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*river.JobInsertResult, error)
}

// Deps groups everything passed into Build so callers don't drift.
type Deps struct {
	Store    *radar.Store
	Embedder embeddings.Client
	Fetcher  crawler.Fetcher
}
```

- [ ] **Step 2: Compile**

```bash
go build ./internal/radar/jobs/
```

Expected: success — this file is self-contained.

- [ ] **Step 3: Commit**

```bash
git add internal/radar/jobs/jobs.go
git commit -m "feat(radar/jobs): add job args, Inserter, and Deps"
```

---

### Task 11: CrawlFeedWorker

**Files:**
- Create: `internal/radar/jobs/crawl_feed.go`

- [ ] **Step 1: Write the worker**

Create `internal/radar/jobs/crawl_feed.go`:

```go
package jobs

import (
	"context"
	"fmt"

	"github.com/ismd/linktheca/internal/radar"
	"github.com/ismd/linktheca/internal/radar/crawler"
	"github.com/riverqueue/river"
)

type CrawlFeedWorker struct {
	river.WorkerDefaults[CrawlFeedArgs]
	store    *radar.Store
	fetcher  crawler.Fetcher
	inserter Inserter // set after River client is built; see SetInserter
}

func NewCrawlFeedWorker(store *radar.Store, fetcher crawler.Fetcher) *CrawlFeedWorker {
	return &CrawlFeedWorker{store: store, fetcher: fetcher}
}

func (w *CrawlFeedWorker) SetInserter(i Inserter) { w.inserter = i }

func (w *CrawlFeedWorker) Work(ctx context.Context, job *river.Job[CrawlFeedArgs]) error {
	feedID := job.Args.FeedID

	state, err := w.store.GetFeedForFetch(ctx, feedID)
	if err != nil {
		return fmt.Errorf("get feed %d: %w", feedID, err)
	}

	etag, lastMod := "", ""
	if state.Etag != nil {
		etag = *state.Etag
	}
	if state.LastModified != nil {
		lastMod = *state.LastModified
	}

	res, err := w.fetcher.Fetch(ctx, state.URL, etag, lastMod)
	if err != nil {
		_ = w.store.MarkFeedError(ctx, feedID, err.Error())
		return fmt.Errorf("fetch feed %d: %w", feedID, err)
	}
	if res.NotModified {
		return w.store.MarkFeedFetched(ctx, feedID, ptrOrNil(res.Etag), ptrOrNil(res.LastModified))
	}

	items, err := crawler.Parse(res.Body)
	if err != nil {
		_ = w.store.MarkFeedError(ctx, feedID, err.Error())
		return fmt.Errorf("parse feed %d: %w", feedID, err)
	}

	for _, up := range crawler.ToUpserts(feedID, items) {
		f, created, err := w.store.UpsertFinding(ctx, up)
		if err != nil {
			return fmt.Errorf("upsert finding for feed %d: %w", feedID, err)
		}
		if !created {
			continue
		}
		if w.inserter != nil {
			if _, err := w.inserter.Insert(ctx, EmbedFindingArgs{FindingID: f.ID}, nil); err != nil {
				return fmt.Errorf("enqueue embed for finding %d: %w", f.ID, err)
			}
		}
	}

	return w.store.MarkFeedFetched(ctx, feedID, ptrOrNil(res.Etag), ptrOrNil(res.LastModified))
}

func ptrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
```

- [ ] **Step 2: Compile check**

```bash
go build ./internal/radar/jobs/
```

Still expect failure for the other three workers. Don't commit yet.

---

### Task 12: EmbedFindingWorker

**Files:**
- Create: `internal/radar/jobs/embed_finding.go`

- [ ] **Step 1: Write the worker**

Create `internal/radar/jobs/embed_finding.go`:

```go
package jobs

import (
	"context"
	"fmt"
	"strings"

	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/ismd/linktheca/internal/radar"
	"github.com/pgvector/pgvector-go"
	"github.com/riverqueue/river"
)

type EmbedFindingWorker struct {
	river.WorkerDefaults[EmbedFindingArgs]
	store    *radar.Store
	embedder embeddings.Client
	inserter Inserter
}

func NewEmbedFindingWorker(store *radar.Store, embedder embeddings.Client) *EmbedFindingWorker {
	return &EmbedFindingWorker{store: store, embedder: embedder}
}

func (w *EmbedFindingWorker) SetInserter(i Inserter) { w.inserter = i }

func (w *EmbedFindingWorker) Work(ctx context.Context, job *river.Job[EmbedFindingArgs]) error {
	id := job.Args.FindingID

	f, err := w.store.GetFindingForEmbed(ctx, id)
	if err != nil {
		return fmt.Errorf("get finding %d: %w", id, err)
	}
	if f.HasEmbedding {
		return nil
	}

	text := embedText(f)
	if text == "" {
		return nil
	}

	vec, err := w.embedder.Embed(ctx, text)
	if err != nil {
		return fmt.Errorf("embed finding %d: %w", id, err)
	}

	if err := w.store.UpdateFindingEmbedding(ctx, id, pgvector.NewVector(vec)); err != nil {
		return fmt.Errorf("save embedding %d: %w", id, err)
	}

	if w.inserter != nil {
		if _, err := w.inserter.Insert(ctx, MatchFindingArgs{FindingID: id}, nil); err != nil {
			return fmt.Errorf("enqueue match for finding %d: %w", id, err)
		}
	}
	return nil
}

func embedText(f *radar.FindingForEmbed) string {
	parts := []string{}
	if f.Title != nil {
		parts = append(parts, strings.TrimSpace(*f.Title))
	}
	if f.Summary != nil {
		parts = append(parts, strings.TrimSpace(*f.Summary))
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}
```

- [ ] **Step 2: Compile check** — still expect failures for remaining workers.

---

### Task 13: MatchFindingWorker

**Files:**
- Create: `internal/radar/jobs/match_finding.go`

- [ ] **Step 1: Write the worker**

Create `internal/radar/jobs/match_finding.go`:

```go
package jobs

import (
	"context"
	"fmt"

	"github.com/ismd/linktheca/internal/radar"
	"github.com/riverqueue/river"
)

type MatchFindingWorker struct {
	river.WorkerDefaults[MatchFindingArgs]
	store *radar.Store
}

func NewMatchFindingWorker(store *radar.Store) *MatchFindingWorker {
	return &MatchFindingWorker{store: store}
}

func (w *MatchFindingWorker) Work(ctx context.Context, job *river.Job[MatchFindingArgs]) error {
	if _, err := w.store.MatchFindingToTopics(ctx, job.Args.FindingID); err != nil {
		return fmt.Errorf("match finding %d: %w", job.Args.FindingID, err)
	}
	return nil
}

// Compile-time check that radar.Store satisfies what MatchFindingWorker needs.
var _ matchStore = (*radar.Store)(nil)

type matchStore interface {
	MatchFindingToTopics(ctx context.Context, findingID int64) (int64, error)
}
```

- [ ] **Step 2: Compile** — still missing ScheduleCrawlsWorker.

---

### Task 14: ScheduleCrawlsWorker + periodic registration

**Files:**
- Create: `internal/radar/jobs/scheduler.go`
- Modify: `internal/radar/jobs/jobs.go`

- [ ] **Step 1: Write the worker**

Create `internal/radar/jobs/scheduler.go`:

```go
package jobs

import (
	"context"
	"fmt"

	"github.com/ismd/linktheca/internal/radar"
	"github.com/riverqueue/river"
)

const scheduleCrawlsBatchSize = 100

type ScheduleCrawlsWorker struct {
	river.WorkerDefaults[ScheduleCrawlsArgs]
	store    *radar.Store
	inserter Inserter
}

func NewScheduleCrawlsWorker(store *radar.Store) *ScheduleCrawlsWorker {
	return &ScheduleCrawlsWorker{store: store}
}

func (w *ScheduleCrawlsWorker) SetInserter(i Inserter) { w.inserter = i }

func (w *ScheduleCrawlsWorker) Work(ctx context.Context, _ *river.Job[ScheduleCrawlsArgs]) error {
	ids, err := w.store.ListDueFeeds(ctx, scheduleCrawlsBatchSize)
	if err != nil {
		return fmt.Errorf("list due feeds: %w", err)
	}
	if w.inserter == nil {
		return nil
	}
	for _, id := range ids {
		if _, err := w.inserter.Insert(ctx, CrawlFeedArgs{FeedID: id}, nil); err != nil {
			return fmt.Errorf("enqueue crawl_feed for feed %d: %w", id, err)
		}
	}
	return nil
}
```

- [ ] **Step 2: Add Build, Bundle, and NewClient to jobs.go**

Edit `internal/radar/jobs/jobs.go`. Append (the existing imports include `context` and the radar/embeddings/crawler packages — extend the import block to add `time`, `pgxpool`, `riverpgxv5`):

```go
// Bundle is what Build returns: ready-to-mount workers, the periodic-job
// spec, and a setter that the caller invokes after constructing the River
// client so workers can enqueue downstream jobs.
type Bundle struct {
	Workers      *river.Workers
	Periodic     []*river.PeriodicJob
	WireInserter func(Inserter)
}

func Build(d Deps, schedulerInterval time.Duration) Bundle {
	scheduler := NewScheduleCrawlsWorker(d.Store)
	crawl := NewCrawlFeedWorker(d.Store, d.Fetcher)
	embed := NewEmbedFindingWorker(d.Store, d.Embedder)
	match := NewMatchFindingWorker(d.Store)

	workers := river.NewWorkers()
	river.AddWorker(workers, scheduler)
	river.AddWorker(workers, crawl)
	river.AddWorker(workers, embed)
	river.AddWorker(workers, match)

	periodic := []*river.PeriodicJob{
		river.NewPeriodicJob(
			river.PeriodicInterval(schedulerInterval),
			func() (river.JobArgs, *river.InsertOpts) {
				return ScheduleCrawlsArgs{}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		),
	}

	return Bundle{
		Workers:  workers,
		Periodic: periodic,
		WireInserter: func(i Inserter) {
			scheduler.SetInserter(i)
			crawl.SetInserter(i)
			embed.SetInserter(i)
		},
	}
}

// NewClient is a thin convenience over river.NewClient for callers that just
// want sensible defaults.
func NewClient(pool *pgxpool.Pool, workers *river.Workers, periodic []*river.PeriodicJob, maxWorkers int) (*river.Client[pgx.Tx], error) {
	return river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:       map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: maxWorkers}},
		Workers:      workers,
		PeriodicJobs: periodic,
	})
}
```

Make sure the consolidated import block reads:

```go
import (
	"context"
	"time"

	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/ismd/linktheca/internal/radar"
	"github.com/ismd/linktheca/internal/radar/crawler"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)
```

Note on the `*river.Client[pgx.Tx]` type: River's generic parameter is the transaction type the driver exposes. `riverpgxv5` uses `pgx.Tx`. Verify with `go doc github.com/riverqueue/river NewClient` and adjust the parameterization if River v0.x has changed it.

- [ ] **Step 3: Build everything**

```bash
go build ./internal/radar/...
```

Expected: success.

- [ ] **Step 4: Commit (atomic commit for all four workers + Build/NewClient)**

```bash
git add internal/radar/jobs/
git commit -m "feat(radar): add River workers and Build helper for radar pipeline"
```

---

### Task 15: Jobs integration test (full pipeline against testdb)

**Files:**
- Create: `internal/radar/jobs/jobs_test.go`

The test drives the four workers without a running River client, by calling `worker.Work(ctx, job)` directly and using a fake `Inserter` that dispatches enqueued args to the matching worker inline. That keeps the test independent of River internals while still exercising the full data flow.

- [ ] **Step 1: Write the test**

Create `internal/radar/jobs/jobs_test.go`:

```go
package jobs_test

import (
	"context"
	"testing"

	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/ismd/linktheca/internal/radar"
	"github.com/ismd/linktheca/internal/radar/crawler"
	"github.com/ismd/linktheca/internal/radar/jobs"
	"github.com/ismd/linktheca/internal/testing/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"
)

// fakeInserter records args and dispatches each one synchronously to the
// matching worker — so a single scheduler.Work call cascades through
// crawl → embed → match in-process.
type fakeInserter struct {
	dispatch map[string]func(ctx context.Context, args river.JobArgs)
	calls    []river.JobArgs
}

func newFakeInserter() *fakeInserter {
	return &fakeInserter{dispatch: map[string]func(context.Context, river.JobArgs){}}
}

func (f *fakeInserter) Insert(ctx context.Context, args river.JobArgs, _ *river.InsertOpts) (*river.JobInsertResult, error) {
	f.calls = append(f.calls, args)
	if fn, ok := f.dispatch[args.Kind()]; ok {
		fn(ctx, args)
	}
	return &river.JobInsertResult{}, nil
}

type fakeFetcher struct{ body []byte }

func (f *fakeFetcher) Fetch(_ context.Context, _, _, _ string) (*crawler.FetchResult, error) {
	return &crawler.FetchResult{StatusCode: 200, Body: f.body, Etag: `"v1"`}, nil
}

func seedUser(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email, password_hash, display_name)
		 VALUES ($1, $2, $3) RETURNING id`,
		"u+"+t.Name()+"@example.com", "x", "Tester").Scan(&id)
	require.NoError(t, err)
	return id
}

func TestJobs_FullPipeline(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := radar.NewStore(pool)
	emb := &embeddings.FakeEmbedder{Dim: 1024}

	userID := seedUser(t, pool)
	feed, err := store.AddFeed(context.Background(), radar.AddFeedParams{
		URL: "https://x.example/feed.xml", Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)
	_, err = store.Subscribe(context.Background(), userID, feed.ID)
	require.NoError(t, err)

	topic, err := store.CreateTopic(context.Background(), radar.CreateTopicParams{
		UserID: userID, Name: "ML", Description: "machine learning research", MatchThreshold: 0.0,
	})
	require.NoError(t, err)
	tvec, _ := emb.Embed(context.Background(), "machine learning research")
	require.NoError(t, store.UpdateTopicEmbedding(context.Background(), topic.ID, pgvector.NewVector(tvec)))

	rss := []byte(`<?xml version="1.0"?>
<rss version="2.0"><channel>
<item><title>OpenAI ships</title><link>https://news.example/post/1</link>
<description>About models</description><guid>g1</guid></item>
</channel></rss>`)

	scheduler := jobs.NewScheduleCrawlsWorker(store)
	crawl := jobs.NewCrawlFeedWorker(store, &fakeFetcher{body: rss})
	embed := jobs.NewEmbedFindingWorker(store, emb)
	match := jobs.NewMatchFindingWorker(store)

	inserter := newFakeInserter()
	scheduler.SetInserter(inserter)
	crawl.SetInserter(inserter)
	embed.SetInserter(inserter)

	inserter.dispatch[jobs.CrawlFeedArgs{}.Kind()] = func(ctx context.Context, a river.JobArgs) {
		require.NoError(t, crawl.Work(ctx, &river.Job[jobs.CrawlFeedArgs]{Args: a.(jobs.CrawlFeedArgs)}))
	}
	inserter.dispatch[jobs.EmbedFindingArgs{}.Kind()] = func(ctx context.Context, a river.JobArgs) {
		require.NoError(t, embed.Work(ctx, &river.Job[jobs.EmbedFindingArgs]{Args: a.(jobs.EmbedFindingArgs)}))
	}
	inserter.dispatch[jobs.MatchFindingArgs{}.Kind()] = func(ctx context.Context, a river.JobArgs) {
		require.NoError(t, match.Work(ctx, &river.Job[jobs.MatchFindingArgs]{Args: a.(jobs.MatchFindingArgs)}))
	}

	// Cascade.
	require.NoError(t, scheduler.Work(context.Background(), &river.Job[jobs.ScheduleCrawlsArgs]{}))

	var n int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM radar_topic_matches WHERE topic_id=$1`, topic.ID).Scan(&n))
	require.Equal(t, 1, n, "expected exactly one match")

	// Idempotent: second cascade does not duplicate.
	require.NoError(t, scheduler.Work(context.Background(), &river.Job[jobs.ScheduleCrawlsArgs]{}))
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM radar_topic_matches WHERE topic_id=$1`, topic.ID).Scan(&n))
	require.Equal(t, 1, n, "second cascade must be idempotent")
}
```

- [ ] **Step 2: Run**

```bash
make dev-db
go test ./internal/radar/jobs/... -v -run TestJobs_FullPipeline
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/radar/jobs/
git commit -m "test(radar/jobs): add full-pipeline integration test"
```

---

## Part E: Server wiring

### Task 16: Server.New plumbs TEI, River, and /radar routes

**Files:**
- Modify: `internal/server/server.go`

- [ ] **Step 1: Update Deps**

Edit `internal/server/server.go`. Replace the `Deps` struct and add new fields plus a `Radar` substruct that callers can leave nil:

```go
type Deps struct {
	Config *config.Config
	Logger *slog.Logger
	DB     *pgxpool.Pool
	Radar  *RadarDeps // nil when RADAR_ENABLED=false
}

// RadarDeps is wired by the caller (cmd/linktheca-server) when Radar is enabled.
// It owns the River client lifecycle.
type RadarDeps struct {
	Embedder embeddings.Client
	River    *river.Client[pgx.Tx]
	Workers  *river.Workers
}
```

Add imports:
```go
"github.com/ismd/linktheca/internal/core/embeddings"
"github.com/ismd/linktheca/internal/radar"
"github.com/jackc/pgx/v5"
"github.com/riverqueue/river"
```

- [ ] **Step 2: Mount /radar routes inside `New`**

Inside `server.New`, after the existing `r.Route("/library", ...)` block, add:

```go
	if cfg.RadarEnabled && deps.Radar != nil {
		radarStore := radar.NewStore(deps.DB)
		radarSvc := radar.NewService(radarStore, deps.Radar.Embedder)
		radarHTTP := radar.NewHTTP(radarSvc)

		r.Route("/radar", func(r chi.Router) {
			r.Use(coreauth.RequireUser(issuer))
			r.Post("/topics", radarHTTP.CreateTopicHandler())
			r.Post("/subscriptions", radarHTTP.SubscribeHandler())
			r.Group(func(r chi.Router) {
				r.Use(coreauth.RequireAdmin)
				r.Post("/feeds", radarHTTP.AddFeedHandler())
			})
		})
	} else {
		r.HandleFunc("/radar", radar.DisabledHandler)
		r.HandleFunc("/radar/*", radar.DisabledHandler)
	}
```

- [ ] **Step 3: Build**

```bash
go build ./...
```

Expected: success.

- [ ] **Step 4: Commit**

```bash
git add internal/server/server.go
git commit -m "feat(server): mount /radar routes when RADAR_ENABLED"
```

---

### Task 17: TestRadarDisabled

**Files:**
- Modify: `internal/server/server_test.go`

- [ ] **Step 1: Open existing server_test.go and read it**

```bash
go doc -src github.com/ismd/linktheca/internal/server | head -40
```

Read `internal/server/server_test.go` — note how the existing tests build `Deps` and call `server.New`.

- [ ] **Step 2: Add the test**

Append to `internal/server/server_test.go`:

```go
func TestRadarDisabled_Returns501OnAnyRoute(t *testing.T) {
	cfg := &config.Config{
		HTTPAddr:     ":0",
		JWTSecret:    "test-secret-at-least-32-bytes-long-for-hmac",
		JWTAccessTTL: 15 * time.Minute,
		RadarEnabled: false,
	}
	deps := server.Deps{
		Config: cfg,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:     nil, // not used by /radar disabled handler
		Radar:  nil,
	}
	srv := server.New(deps)
	defer srv.Close()

	for _, path := range []string{"/radar/topics", "/radar/feeds", "/radar/subscriptions", "/radar/anything"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
		rec := httptest.NewRecorder()
		srv.Handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusNotImplemented, rec.Code, "path %s", path)
		require.Contains(t, rec.Body.String(), "radar_disabled")
	}
}
```

Add necessary imports (`io`, `net/http`, `net/http/httptest`, `strings`, `time`, `log/slog`, `testing`, `config`, `server`, `require`).

- [ ] **Step 3: Run**

```bash
go test ./internal/server/... -run TestRadarDisabled -v
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/server/server_test.go
git commit -m "test(server): assert /radar returns 501 when disabled"
```

---

### Task 18: Wire River + TEI in main.go (startup + shutdown)

**Files:**
- Modify: `cmd/linktheca-server/main.go`

- [ ] **Step 1: Update run() to construct radar deps when enabled**

Edit `cmd/linktheca-server/main.go`. Replace `run()` with:

```go
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := logging.New(os.Stdout, cfg.LogFormat, cfg.LogLevel)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx, cfg.DBDSN)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool, linktheca.MigrationsFS, "migrations"); err != nil {
		return err
	}
	logger.Info("goose migrations applied")

	var radarDeps *server.RadarDeps
	var riverClient *river.Client[pgx.Tx]
	if cfg.RadarEnabled {
		// River migrations.
		mig := rivermigrate.New(riverpgxv5.New(pool), nil)
		if _, err := mig.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
			return fmt.Errorf("river migrate: %w", err)
		}
		logger.Info("river migrations applied")

		teiClient := embeddings.NewTEIClient(cfg.TEIURL, cfg.TEITimeout)
		// Best-effort self-check; warn but don't fail-fast.
		if vec, err := teiClient.Embed(ctx, "ping"); err != nil {
			logger.Warn("TEI self-check failed", "err", err)
		} else if len(vec) != cfg.EmbeddingDim {
			logger.Warn("TEI embedding dim mismatch",
				"want", cfg.EmbeddingDim, "got", len(vec))
		}

		store := radar.NewStore(pool)
		bundle := jobs.Build(jobs.Deps{
			Store:    store,
			Embedder: teiClient,
			Fetcher:  crawler.NewHTTPFetcher(30 * time.Second),
		}, cfg.RadarSchedulerInterval)

		client, err := jobs.NewClient(pool, bundle.Workers, bundle.Periodic, cfg.RadarMaxWorkers)
		if err != nil {
			return fmt.Errorf("river new client: %w", err)
		}
		bundle.WireInserter(client)
		if err := client.Start(ctx); err != nil {
			return fmt.Errorf("river start: %w", err)
		}
		riverClient = client

		radarDeps = &server.RadarDeps{
			Embedder: teiClient,
			River:    client,
			Workers:  bundle.Workers,
		}
	}

	srv := server.New(server.Deps{
		Config: cfg,
		Logger: logger,
		DB:     pool,
		Radar:  radarDeps,
	})

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server starting", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	logger.Info("http server stopped")

	if riverClient != nil {
		if err := riverClient.Stop(shutdownCtx); err != nil {
			logger.Warn("river stop", "err", err)
		}
	}
	return nil
}
```

Add imports:
```go
"fmt"
"github.com/ismd/linktheca/internal/core/embeddings"
"github.com/ismd/linktheca/internal/radar"
"github.com/ismd/linktheca/internal/radar/crawler"
"github.com/ismd/linktheca/internal/radar/jobs"
"github.com/jackc/pgx/v5"
"github.com/riverqueue/river"
"github.com/riverqueue/river/riverdriver/riverpgxv5"
"github.com/riverqueue/river/rivermigrate"
```

- [ ] **Step 2: Build**

```bash
go build ./cmd/linktheca-server
```

Expected: success.

- [ ] **Step 3: Manual sanity check (Postgres up, TEI not)**

```bash
make dev-db
sleep 3
LINKTHECA_DB_DSN="postgres://linktheca:linktheca@localhost:5432/linktheca?sslmode=disable" \
LINKTHECA_JWT_SECRET="dev-only-secret-that-is-at-least-32-bytes-long" \
LINKTHECA_RADAR_ENABLED=false \
go run ./cmd/linktheca-server &
SERVER_PID=$!
sleep 3
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/healthz
kill $SERVER_PID
```

Expected: `200`.

- [ ] **Step 4: Commit**

```bash
git add cmd/linktheca-server/main.go
git commit -m "feat(server): wire River + TEI on startup with graceful shutdown"
```

---

## Part F: Compose, Makefile, smoke test

### Task 19: Add tei service to compose.dev.yaml

**Files:**
- Modify: `compose.dev.yaml`

- [ ] **Step 1: Edit compose**

Replace the file with:

```yaml
services:
  postgres:
    image: pgvector/pgvector:0.8.2-pg18-trixie
    restart: unless-stopped
    environment:
      POSTGRES_USER: linktheca
      POSTGRES_PASSWORD: linktheca
      POSTGRES_DB: linktheca
    ports:
      - "5432:5432"
    volumes:
      - linktheca_pg_data:/var/lib/postgresql
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U linktheca -d linktheca"]
      interval: 5s
      timeout: 5s
      retries: 5

  tei:
    image: ghcr.io/huggingface/text-embeddings-inference:cpu-1.9
    command: ["--model-id", "BAAI/bge-m3", "--port", "8080"]
    ports:
      - "8081:8080"
    volumes:
      - linktheca_tei_data:/data
    healthcheck:
      test: ["CMD", "curl", "-fs", "http://localhost:8080/health"]
      interval: 10s
      timeout: 5s
      retries: 10
      start_period: 120s
    restart: unless-stopped

volumes:
  linktheca_pg_data:
  linktheca_tei_data:
```

- [ ] **Step 2: Verify (manual)**

```bash
docker compose -f compose.dev.yaml config
```

Expected: parses without error.

- [ ] **Step 3: Commit**

```bash
git add compose.dev.yaml
git commit -m "chore(compose): add TEI service for radar pipeline"
```

---

### Task 20: Makefile smoke-radar target

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Add target**

Replace lines 36–37 (`test-integration`) by appending the smoke target after them:

```make
smoke-radar:
	go test -tags=smoke -timeout=10m -count=1 ./internal/radar/... ./internal/core/embeddings/...
```

Update help text (lines 3–15) — add the line for `smoke-radar`:

```make
	@echo "  smoke-radar     - smoke tests with real TEI (slow)"
```

- [ ] **Step 2: Sanity**

```bash
make help
```

Expected: smoke-radar is listed.

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "chore(make): add smoke-radar target"
```

---

### Task 21: Pipeline smoke test (real TEI + real RSS)

**Files:**
- Create: `internal/radar/jobs/smoke_test.go`

- [ ] **Step 1: Write the smoke test**

Create `internal/radar/jobs/smoke_test.go`:

```go
//go:build smoke

package jobs_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/ismd/linktheca/internal/radar"
	"github.com/ismd/linktheca/internal/radar/crawler"
	"github.com/ismd/linktheca/internal/radar/jobs"
	"github.com/ismd/linktheca/internal/testing/testdb"
	"github.com/pgvector/pgvector-go"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestSmoke_PipelineWithRealTEI runs a TEI container, serves a synthetic RSS
// feed, and walks scheduler → crawl → embed → match using the real embedder.
// Verifies that bge-m3 produces 1024-dim vectors and that a topic similar to
// the finding gets matched.
func TestSmoke_PipelineWithRealTEI(t *testing.T) {
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "ghcr.io/huggingface/text-embeddings-inference:cpu-1.9",
		Cmd:          []string{"--model-id", "BAAI/bge-m3", "--port", "8080"},
		ExposedPorts: []string{"8080/tcp"},
		WaitingFor:   wait.ForHTTP("/health").WithPort("8080/tcp").WithStartupTimeout(10 * time.Minute),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req, Started: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "8080/tcp")
	require.NoError(t, err)

	teiClient := embeddings.NewTEIClient("http://"+host+":"+port.Port(), 60*time.Second)

	// Synthetic RSS server.
	rss := `<?xml version="1.0"?>
<rss version="2.0"><channel>
<item><title>OpenAI ships GPT</title>
<link>https://news.example/post/1</link>
<description>Recent advances in transformer architectures and inference.</description>
<guid>g1</guid></item></channel></rss>`
	rssSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(rss))
	}))
	defer rssSrv.Close()

	pool := testdb.New(t)
	store := radar.NewStore(pool)

	userID := seedUser(t, pool)
	feed, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: rssSrv.URL, Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)
	_, err = store.Subscribe(ctx, userID, feed.ID)
	require.NoError(t, err)

	topic, err := store.CreateTopic(ctx, radar.CreateTopicParams{
		UserID: userID, Name: "AI", Description: "transformers and large language models", MatchThreshold: 0.0,
	})
	require.NoError(t, err)
	tvec, err := teiClient.Embed(ctx, "transformers and large language models")
	require.NoError(t, err)
	require.Len(t, tvec, 1024)
	require.NoError(t, store.UpdateTopicEmbedding(ctx, topic.ID, pgvector.NewVector(tvec)))

	scheduler := jobs.NewScheduleCrawlsWorker(store)
	crawl := jobs.NewCrawlFeedWorker(store, crawler.NewHTTPFetcher(30*time.Second))
	embed := jobs.NewEmbedFindingWorker(store, teiClient)
	match := jobs.NewMatchFindingWorker(store)

	inserter := newFakeInserter()
	scheduler.SetInserter(inserter)
	crawl.SetInserter(inserter)
	embed.SetInserter(inserter)

	inserter.dispatch[jobs.CrawlFeedArgs{}.Kind()] = func(ctx context.Context, a river.JobArgs) {
		require.NoError(t, crawl.Work(ctx, &river.Job[jobs.CrawlFeedArgs]{Args: a.(jobs.CrawlFeedArgs)}))
	}
	inserter.dispatch[jobs.EmbedFindingArgs{}.Kind()] = func(ctx context.Context, a river.JobArgs) {
		require.NoError(t, embed.Work(ctx, &river.Job[jobs.EmbedFindingArgs]{Args: a.(jobs.EmbedFindingArgs)}))
	}
	inserter.dispatch[jobs.MatchFindingArgs{}.Kind()] = func(ctx context.Context, a river.JobArgs) {
		require.NoError(t, match.Work(ctx, &river.Job[jobs.MatchFindingArgs]{Args: a.(jobs.MatchFindingArgs)}))
	}

	require.NoError(t, scheduler.Work(ctx, &river.Job[jobs.ScheduleCrawlsArgs]{}))

	var n int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM radar_topic_matches WHERE topic_id=$1`, topic.ID).Scan(&n))
	require.GreaterOrEqual(t, n, 1, "expected at least one match")
}
```

- [ ] **Step 2: Build-tag check**

```bash
go vet -tags=smoke ./internal/radar/jobs/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/radar/jobs/smoke_test.go
git commit -m "test(radar/jobs): add real-TEI pipeline smoke test"
```

---

## Part G: Manual end-to-end verification

### Task 22: Manual smoke run

This is a manual verification step — no commit, but mark the plan complete after.

- [ ] **Step 1: Bring up Postgres + TEI**

```bash
docker compose -f compose.dev.yaml up -d
docker compose -f compose.dev.yaml logs -f tei | head -40 &
```

Wait for TEI healthcheck to go green (~2 minutes first time as it pulls the model).

- [ ] **Step 2: Start the server**

```bash
LINKTHECA_DB_DSN="postgres://linktheca:linktheca@localhost:5432/linktheca?sslmode=disable" \
LINKTHECA_JWT_SECRET="dev-only-secret-that-is-at-least-32-bytes-long" \
LINKTHECA_TEI_URL="http://localhost:8081" \
LINKTHECA_RADAR_ENABLED=true \
go run ./cmd/linktheca-server &
SERVER_PID=$!
sleep 3
```

Expected logs: "goose migrations applied", "river migrations applied", "http server starting".

- [ ] **Step 3: Register an admin user via curl**

```bash
ADMIN_TOKEN=$(curl -s -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@example.com","password":"correcthorse","display_name":"Admin"}' \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["tokens"]["access_token"])')
echo "$ADMIN_TOKEN"
```

(First user is auto-promoted to admin per existing auth logic.)

- [ ] **Step 4: Add a feed, topic, subscription**

```bash
FEED_ID=$(curl -s -X POST http://localhost:8080/radar/feeds \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://news.ycombinator.com/rss"}' \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')

curl -s -X POST http://localhost:8080/radar/topics \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"AI","description":"machine learning research and large language models"}'

curl -s -X POST http://localhost:8080/radar/subscriptions \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"feed_id\": $FEED_ID}"
```

- [ ] **Step 5: Wait ≥30s for the periodic crawl to fire and inspect**

```bash
sleep 30
docker compose -f compose.dev.yaml exec -T postgres \
  psql -U linktheca -d linktheca -c "SELECT count(*) FROM radar_findings"
docker compose -f compose.dev.yaml exec -T postgres \
  psql -U linktheca -d linktheca -c "SELECT topic_id, count(*) FROM radar_topic_matches GROUP BY topic_id"
```

Expected: at least a few findings, possibly some matches (depends on HN headlines).

- [ ] **Step 6: Stop server**

```bash
kill $SERVER_PID
```

- [ ] **Step 7: Mark this plan as complete in the docs**

(No file change required beyond reviewing checkboxes; completion is signaled by the user moving on to phase 3a-3.)

---

## Self-review checklist (engineer runs this before declaring done)

- [ ] Every `radar.Service` method covered by both unit tests (`service_test.go`) and an HTTP integration test (`integration_test.go`).
- [ ] `DisabledHandler` returns 501 with `"radar_disabled"` and is mounted on `/radar` and `/radar/*` when `RadarEnabled=false`.
- [ ] `server.Deps.Radar` is `nil`-safe — `server.New` does not nil-deref when Radar is off.
- [ ] River workers use the constructors `NewScheduleCrawlsWorker`, `NewCrawlFeedWorker`, `NewEmbedFindingWorker`, `NewMatchFindingWorker`. `Build` wires them and `WireInserter` injects the inserter after the client exists.
- [ ] `cmd/linktheca-server/main.go` orders shutdown HTTP → River → pool, and runs goose then River migrations on startup.
- [ ] `compose.dev.yaml` exposes TEI on `8081:8080` with the `linktheca_tei_data` volume.
- [ ] Smoke test sits behind `//go:build smoke` and runs only via `make smoke-radar`.
