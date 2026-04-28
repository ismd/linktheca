# Phase 3a: Radar Pipeline Backend — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the backend of the Radar module end-to-end: users register RSS feeds and topics, a background pipeline pulls feeds, embeds findings via TEI, and matches them against topic embeddings with pgvector. A CLI binary (`linktheca`) wraps three new HTTP endpoints for bootstrap.

**Architecture:** Five new migrations (006–010) add `radar_topics`, `radar_feeds`, `radar_feed_subscriptions`, `radar_findings`, `radar_topic_matches`. A new `internal/core/embeddings/` package provides a `Client` interface with a `TEIClient` and a deterministic `FakeEmbedder` for tests. The `internal/radar/` module follows the project's `store → service → http` pattern. A `crawler/` sub-package wraps `mmcdole/gofeed`; a `jobs/` sub-package owns the River-based pipeline (Scheduler → CrawlFeed → EmbedFinding → MatchFinding). The HTTP surface in phase 3a is three POST endpoints behind `RequireUser` / `RequireAdmin`. The existing `cmd/linktheca` is renamed to `cmd/linktheca-server`; a new `cmd/linktheca` is a cobra-based CLI that talks to the server via HTTP only.

**Tech Stack:** Go 1.26+, `go-chi/chi/v5`, `jackc/pgx/v5`, `pgvector/pgvector-go`, `mmcdole/gofeed`, `riverqueue/river` + `riverdriver/riverpgxv5`, `spf13/cobra`, `stretchr/testify`, `testcontainers-go`. New infrastructure: HuggingFace TEI (docker, `cpu-1.9`, `BAAI/bge-m3`, 1024 dim).

**Module path:** `github.com/ismd/linktheca`

**Working directory:** `/home/ismd/coding/linktheca`

---

## File structure created or modified by this phase

```
linktheca/
├── cmd/
│   ├── linktheca-server/              # renamed from cmd/linktheca/
│   │   └── main.go                    # MOVED, content unchanged
│   └── linktheca/                     # NEW: CLI
│       ├── main.go
│       ├── integration_test.go
│       └── internal/cli/
│           ├── root.go
│           ├── session/
│           │   ├── session.go
│           │   └── session_test.go
│           ├── apiclient/
│           │   ├── client.go
│           │   ├── client_test.go
│           │   ├── auth.go
│           │   └── radar.go
│           ├── output/
│           │   ├── format.go
│           │   └── format_test.go
│           └── cmd/
│               ├── auth_register.go
│               ├── auth_login.go
│               ├── auth_logout.go
│               ├── auth_whoami.go
│               ├── radar_topic_add.go
│               ├── radar_feed_add.go
│               └── radar_subscribe.go
│
├── migrations/
│   ├── 006_radar_topics.sql           # NEW
│   ├── 007_radar_feeds.sql            # NEW
│   ├── 008_radar_feed_subscriptions.sql  # NEW
│   ├── 009_radar_findings.sql         # NEW
│   └── 010_radar_topic_matches.sql    # NEW
│
├── internal/
│   ├── core/
│   │   ├── config/config.go           # MODIFIED: new fields
│   │   └── embeddings/                # NEW package
│   │       ├── client.go
│   │       ├── client_test.go
│   │       ├── fake.go
│   │       ├── fake_test.go
│   │       └── client_smoke_test.go
│   ├── radar/                          # NEW package
│   │   ├── types.go
│   │   ├── store.go
│   │   ├── store_test.go
│   │   ├── service.go
│   │   ├── service_test.go
│   │   ├── http.go
│   │   ├── http_test.go
│   │   ├── integration_test.go
│   │   ├── crawler/
│   │   │   ├── crawler.go
│   │   │   └── crawler_test.go
│   │   └── jobs/
│   │       ├── jobs.go
│   │       ├── crawl_feed.go
│   │       ├── embed_finding.go
│   │       ├── match_finding.go
│   │       ├── scheduler.go
│   │       ├── jobs_test.go
│   │       └── smoke_test.go
│   ├── server/
│   │   └── server.go                  # MODIFIED: TEI + River wiring, conditional /radar
│   └── testing/
│       └── testdb/testdb.go           # MODIFIED: ensure pgvector works with schema-per-test
│
├── compose.dev.yaml                   # MODIFIED: + tei service
├── Makefile                           # MODIFIED: cli-build, server-build, smoke-radar
├── Dockerfile                         # MODIFIED (if present): new binary name
├── .github/workflows/ci.yml           # MODIFIED: new binary path
├── README.md                          # MODIFIED: Radar on/off section, RAM requirements
└── embeds.go                          # UNCHANGED: `migrations/*.sql` glob picks up new files
```

**Not in this phase:** `GET /radar/topics`, `GET /radar/feeds`, `GET /radar/feed`, reader view, Update/Delete endpoints, CSV/OPML import, feed auto-discovery, notifications, production docker-compose, admin UI, frontend.

---

## Conventions for every task

- **TDD everywhere.** Every non-trivial function gets a failing test first, then minimal implementation, then verification.
- **Commit after each task.** Small, focused commits make review easy and rollback cheap.
- **Run from the repo root** (`/home/ismd/coding/linktheca`) unless otherwise noted.
- **Do not use `git add .`** — stage files explicitly.
- **Commit messages** follow `<type>(<scope>): <subject>` (e.g., `feat(radar): add radar_topics migration`).
- **Go version:** Go 1.26+. Check with `go version`.
- **Go integer style:** use `int64` for BIGINT PKs (`user_id` is `INT` → still scanned as `int64` via pgx); never use `uint`. Matches library/auth modules.
- **Errors:** sentinel errors at the package top (like `library.ErrNotFound`). Wrap with `%w` for context.
- **Context:** every store/service method takes `ctx context.Context` as first arg.

### Quick reference — existing helpers

- `coreauth.UserID(ctx) int64` — gets the authenticated user id out of the request context.
- `coreauth.RequireUser(issuer)` — middleware that rejects un-authed requests.
- `httpx.WriteJSON(w, status, body)`, `httpx.WriteError(w, status, code, msg)` — stdlib-only helpers.
- `testdb.New(t) *pgxpool.Pool` — spins up Postgres, applies migrations in a fresh schema, auto-cleans.

---

## Part A: Preparation

### Task 1: Rename `cmd/linktheca` → `cmd/linktheca-server`

**Files:**
- Move: `cmd/linktheca/main.go` → `cmd/linktheca-server/main.go`
- Modify: `Makefile`

- [x] **Step 1: Move the directory via git**

```bash
git mv cmd/linktheca cmd/linktheca-server
```

- [x] **Step 2: Verify contents unchanged and build**

```bash
go build ./cmd/linktheca-server
```

Expected: success, no output.

- [x] **Step 3: Update `Makefile`**

Replace the `run:`, `build:` targets and extend the help text. Open `Makefile` and apply:

Replace:
```make
run:
	go run ./cmd/linktheca

...

build:
	mkdir -p bin
	go build -o bin/linktheca ./cmd/linktheca
```

With:
```make
run:
	go run ./cmd/linktheca-server

...

build: server-build cli-build

server-build:
	mkdir -p bin
	go build -o bin/linktheca-server ./cmd/linktheca-server

cli-build:
	mkdir -p bin
	go build -o bin/linktheca ./cmd/linktheca
```

Also update `.PHONY` to add `server-build cli-build smoke-radar` (smoke-radar added in a later task, adding now is fine).

- [x] **Step 4: Search for any other references**

```bash
grep -rn "cmd/linktheca\b" --include="*.go" --include="*.yml" --include="*.yaml" --include="Dockerfile*" --include="*.md" .
```

If any references remain (e.g., `Dockerfile`, `.github/workflows/ci.yml`, `README.md`), update them to `cmd/linktheca-server`.

- [x] **Step 5: Ensure `go build ./...` still works**

```bash
go build ./...
```

Expected: success. The `cmd/linktheca/` directory no longer exists; later tasks re-create it as the CLI.

- [x] **Step 6: Commit**

```bash
git add cmd/ Makefile
# add any other files that were modified in step 4:
# git add Dockerfile .github/workflows/ci.yml README.md
git commit -m "refactor(cmd): rename linktheca to linktheca-server"
```

---

### Task 2: Add new config fields

**Files:**
- Modify: `internal/core/config/config.go`

- [x] **Step 1: Write a failing test for the new fields**

Create `internal/core/config/config_test.go` if it doesn't exist, or add to it. (This test validates that the new env vars are parsed — it's a tiny guard, not a full config-coverage pass.)

```go
package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoad_RadarDefaults(t *testing.T) {
	t.Setenv("LINKTHECA_DB_DSN", "postgres://x:y@localhost:5432/z?sslmode=disable")
	t.Setenv("LINKTHECA_JWT_SECRET", "dev-only-secret-that-is-at-least-32-bytes-long")

	cfg, err := Load()
	require.NoError(t, err)

	require.Equal(t, "http://localhost:8081", cfg.TEIURL)
	require.Equal(t, 30*time.Second, cfg.TEITimeout)
	require.Equal(t, 1024, cfg.EmbeddingDim)
	require.True(t, cfg.RadarEnabled)
	require.Equal(t, 5*time.Minute, cfg.RadarSchedulerInterval)
	require.Equal(t, 5, cfg.RadarMaxWorkers)
}
```

- [x] **Step 2: Run test — expect failure**

```bash
go test ./internal/core/config/...
```

Expected: compile error (fields don't exist yet).

- [x] **Step 3: Add fields to `Config`**

Edit `internal/core/config/config.go`. Inside the `Config` struct (after existing fields), add:

```go
	TEIURL       string        `env:"LINKTHECA_TEI_URL" envDefault:"http://localhost:8081"`
	TEITimeout   time.Duration `env:"LINKTHECA_TEI_TIMEOUT" envDefault:"30s"`
	EmbeddingDim int           `env:"LINKTHECA_EMBEDDING_DIM" envDefault:"1024"`

	RadarEnabled           bool          `env:"LINKTHECA_RADAR_ENABLED" envDefault:"true"`
	RadarSchedulerInterval time.Duration `env:"LINKTHECA_RADAR_SCHEDULER_INTERVAL" envDefault:"5m"`
	RadarMaxWorkers        int           `env:"LINKTHECA_RADAR_MAX_WORKERS" envDefault:"5"`
```

- [x] **Step 4: Run tests — expect pass**

```bash
go test ./internal/core/config/...
```

Expected: PASS.

- [x] **Step 5: Commit**

```bash
git add internal/core/config/
git commit -m "feat(config): add TEI and Radar env fields"
```

---

### Task 3: Add new Go module dependencies

**Files:**
- Modify: `go.mod`, `go.sum`

- [x] **Step 1: Add dependencies**

```bash
go get github.com/pgvector/pgvector-go
go get github.com/mmcdole/gofeed
go get github.com/riverqueue/river
go get github.com/riverqueue/river/riverdriver/riverpgxv5
go get github.com/riverqueue/river/rivermigrate
go get github.com/spf13/cobra
```

- [x] **Step 2: Tidy**

```bash
go mod tidy
```

- [x] **Step 3: Verify build**

```bash
go build ./...
```

Expected: success (no code uses them yet; they are tracked as direct deps once first imported in later tasks — that's fine).

- [x] **Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "chore(deps): add pgvector, gofeed, river, cobra"
```

---

### Task 4: Verify testdb works with pgvector

Observation: `testdb.New` uses `search_path=<schema>` without `,public`. Extensions live in a fixed schema (typically `public`) — the `vector` type is resolved there. In practice Postgres's unqualified-type resolution searches `pg_catalog` and the type's schema is fine as long as we use `vector(1024)` without a schema qualifier. We still add a belt-and-suspenders sanity test to catch silent breakage.

**Files:**
- Create: `internal/testing/testdb/testdb_vector_test.go`

- [x] **Step 1: Write a sanity test**

Create `internal/testing/testdb/testdb_vector_test.go`:

```go
package testdb_test

import (
	"context"
	"testing"

	"github.com/ismd/linktheca/internal/testing/testdb"
	"github.com/pgvector/pgvector-go"
	"github.com/stretchr/testify/require"
)

// TestVectorTypeAvailable asserts that the pgvector `vector` type is usable
// inside the per-test schema created by testdb.New.
func TestVectorTypeAvailable(t *testing.T) {
	pool := testdb.New(t)
	ctx := context.Background()

	_, err := pool.Exec(ctx, `CREATE TEMP TABLE t (v vector(3))`)
	require.NoError(t, err)

	vec := pgvector.NewVector([]float32{1, 2, 3})
	_, err = pool.Exec(ctx, `INSERT INTO t (v) VALUES ($1)`, vec)
	require.NoError(t, err)

	var got pgvector.Vector
	err = pool.QueryRow(ctx, `SELECT v FROM t LIMIT 1`).Scan(&got)
	require.NoError(t, err)
	require.Equal(t, []float32{1, 2, 3}, got.Slice())
}
```

- [x] **Step 2: Run the test**

```bash
go test ./internal/testing/testdb/... -run TestVectorTypeAvailable -v
```

Expected: PASS. If it fails with `type "vector" does not exist`, fix by editing `testdb.go`: change `scopedDSN := sharedDSN + "&search_path=" + schema` to `scopedDSN := sharedDSN + "&search_path=" + schema + ",public"`. Re-run — expected PASS.

- [x] **Step 3: Commit**

```bash
git add internal/testing/testdb/
git commit -m "test(testdb): assert pgvector type works under schema-per-test"
```

---

## Part B: Migrations 006–010

### Task 5: Migration 006 — radar_topics

**Files:**
- Create: `migrations/006_radar_topics.sql`

- [x] **Step 1: Create the migration file**

```sql
-- +goose Up
CREATE TABLE radar_topics (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id         INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL,
    embedding       vector(1024),
    match_threshold REAL NOT NULL DEFAULT 0.75,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX radar_topics_user_active_idx ON radar_topics (user_id) WHERE is_active;

-- +goose Down
DROP TABLE radar_topics;
```

- [x] **Step 2: Verify it applies**

```bash
make dev-db
sleep 3
LINKTHECA_DB_DSN="postgres://linktheca:linktheca@localhost:5432/linktheca?sslmode=disable" \
LINKTHECA_JWT_SECRET="dev-only-secret-that-is-at-least-32-bytes-long" \
go run ./cmd/linktheca-server &
SERVER_PID=$!
sleep 2
kill $SERVER_PID 2>/dev/null
```

Then inspect:
```bash
docker compose -f compose.dev.yaml exec -T postgres \
  psql -U linktheca -d linktheca -c "\d radar_topics"
```

Expected: table definition printed.

- [x] **Step 3: Commit**

```bash
git add migrations/006_radar_topics.sql
git commit -m "feat(radar): add radar_topics migration"
```

---

### Task 6: Migration 007 — radar_feeds

**Files:**
- Create: `migrations/007_radar_feeds.sql`

- [x] **Step 1: Create the migration file**

```sql
-- +goose Up
CREATE TABLE radar_feeds (
    id                     BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    url                    TEXT NOT NULL UNIQUE,
    kind                   TEXT NOT NULL DEFAULT 'rss'
                           CHECK (kind IN ('rss', 'atom')),
    title                  TEXT,
    last_fetched_at        TIMESTAMPTZ,
    last_error             TEXT,
    etag                   TEXT,
    last_modified          TEXT,
    fetch_interval_seconds INT NOT NULL DEFAULT 3600,
    is_active              BOOLEAN NOT NULL DEFAULT TRUE,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX radar_feeds_active_fetched_idx ON radar_feeds (is_active, last_fetched_at);

-- +goose Down
DROP TABLE radar_feeds;
```

- [x] **Step 2: Verify apply (same procedure as task 5)**

- [x] **Step 3: Commit**

```bash
git add migrations/007_radar_feeds.sql
git commit -m "feat(radar): add radar_feeds migration"
```

---

### Task 7: Migration 008 — radar_feed_subscriptions

**Files:**
- Create: `migrations/008_radar_feed_subscriptions.sql`

- [x] **Step 1: Create the migration file**

```sql
-- +goose Up
CREATE TABLE radar_feed_subscriptions (
    user_id    INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    feed_id    BIGINT NOT NULL REFERENCES radar_feeds(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, feed_id)
);

-- +goose Down
DROP TABLE radar_feed_subscriptions;
```

- [x] **Step 2: Verify apply**

- [x] **Step 3: Commit**

```bash
git add migrations/008_radar_feed_subscriptions.sql
git commit -m "feat(radar): add radar_feed_subscriptions migration"
```

---

### Task 8: Migration 009 — radar_findings

**Files:**
- Create: `migrations/009_radar_findings.sql`

- [x] **Step 1: Create the migration file**

```sql
-- +goose Up
CREATE TABLE radar_findings (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    feed_id       BIGINT NOT NULL REFERENCES radar_feeds(id) ON DELETE CASCADE,
    content_id    BIGINT REFERENCES article_contents(id),
    external_id   TEXT,
    url           TEXT NOT NULL,
    title         TEXT,
    summary       TEXT,
    embedding     vector(1024),
    published_at  TIMESTAMPTZ,
    discovered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (feed_id, external_id)
);
CREATE INDEX radar_findings_discovered_idx ON radar_findings (discovered_at DESC);
CREATE INDEX radar_findings_embedding_hnsw ON radar_findings USING hnsw (embedding vector_cosine_ops);

-- +goose Down
DROP TABLE radar_findings;
```

- [x] **Step 2: Verify apply**

- [x] **Step 3: Commit**

```bash
git add migrations/009_radar_findings.sql
git commit -m "feat(radar): add radar_findings migration with HNSW index"
```

---

### Task 9: Migration 010 — radar_topic_matches

**Files:**
- Create: `migrations/010_radar_topic_matches.sql`

- [x] **Step 1: Create the migration file**

```sql
-- +goose Up
CREATE TABLE radar_topic_matches (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    topic_id   BIGINT NOT NULL REFERENCES radar_topics(id) ON DELETE CASCADE,
    finding_id BIGINT NOT NULL REFERENCES radar_findings(id) ON DELETE CASCADE,
    similarity REAL NOT NULL,
    state      TEXT NOT NULL DEFAULT 'new'
               CHECK (state IN ('new', 'seen')),
    matched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (topic_id, finding_id)
);
CREATE INDEX radar_topic_matches_topic_state_idx ON radar_topic_matches (topic_id, state, matched_at DESC);

-- +goose Down
DROP TABLE radar_topic_matches;
```

- [x] **Step 2: Verify apply**

- [x] **Step 3: Commit**

```bash
git add migrations/010_radar_topic_matches.sql
git commit -m "feat(radar): add radar_topic_matches migration"
```

---

## Part C: Embeddings package

### Task 10: Embeddings `Client` interface + `FakeEmbedder`

**Files:**
- Create: `internal/core/embeddings/client.go` (interface + types only in this task)
- Create: `internal/core/embeddings/fake.go`
- Create: `internal/core/embeddings/fake_test.go`

- [x] **Step 1: Write the failing test**

Create `internal/core/embeddings/fake_test.go`:

```go
package embeddings

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFakeEmbedder_Deterministic(t *testing.T) {
	e := &FakeEmbedder{Dim: 1024}
	ctx := context.Background()

	v1, err := e.Embed(ctx, "hello world")
	require.NoError(t, err)
	require.Len(t, v1, 1024)

	v2, err := e.Embed(ctx, "hello world")
	require.NoError(t, err)
	require.Equal(t, v1, v2)
}

func TestFakeEmbedder_Differs(t *testing.T) {
	e := &FakeEmbedder{Dim: 1024}
	ctx := context.Background()

	a, _ := e.Embed(ctx, "alpha")
	b, _ := e.Embed(ctx, "bravo")
	require.NotEqual(t, a, b)
}

func TestFakeEmbedder_Normalized(t *testing.T) {
	e := &FakeEmbedder{Dim: 1024}
	v, _ := e.Embed(context.Background(), "anything")

	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	require.InDelta(t, 1.0, math.Sqrt(sum), 1e-5)
}
```

- [x] **Step 2: Run test — expect failure**

```bash
go test ./internal/core/embeddings/...
```

Expected: compile error (package doesn't exist).

- [x] **Step 3: Create the interface**

Create `internal/core/embeddings/client.go`:

```go
// Package embeddings provides a text-to-vector client interface with a TEI
// (HuggingFace Text Embeddings Inference) implementation and a deterministic
// fake for tests.
package embeddings

import "context"

type Client interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}
```

- [x] **Step 4: Implement `FakeEmbedder`**

Create `internal/core/embeddings/fake.go`:

```go
package embeddings

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"math"
)

// FakeEmbedder is a deterministic stand-in for TEI used in unit and
// integration tests. Embed returns an L2-normalized vector of length Dim
// derived from SHA-256(text). Same text → same vector; different text →
// different vector with low expected cosine similarity.
type FakeEmbedder struct {
	Dim int
}

func (f *FakeEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	dim := f.Dim
	if dim <= 0 {
		dim = 1024
	}

	out := make([]float32, dim)
	seed := sha256.Sum256([]byte(text))

	for i := 0; i < dim; i++ {
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], uint64(i))
		h := sha256.New()
		h.Write(seed[:])
		h.Write(buf[:])
		sum := h.Sum(nil)
		// Turn 4 bytes into a float in roughly [-1, 1].
		u := binary.BigEndian.Uint32(sum[:4])
		f32 := (float32(u)/float32(math.MaxUint32))*2 - 1
		out[i] = f32
	}

	// L2-normalize.
	var norm float64
	for _, x := range out {
		norm += float64(x) * float64(x)
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for i := range out {
			out[i] = float32(float64(out[i]) / norm)
		}
	}
	return out, nil
}
```

- [x] **Step 5: Run tests — expect pass**

```bash
go test ./internal/core/embeddings/... -v
```

Expected: all three tests PASS.

- [x] **Step 6: Commit**

```bash
git add internal/core/embeddings/
git commit -m "feat(embeddings): add Client interface and FakeEmbedder"
```

---

### Task 11: `TEIClient` implementation

**Files:**
- Modify: `internal/core/embeddings/client.go`
- Create: `internal/core/embeddings/client_test.go`

- [x] **Step 1: Write the failing test**

Create `internal/core/embeddings/client_test.go`:

```go
package embeddings

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTEIClient_Embed_Success(t *testing.T) {
	var got struct {
		Inputs string `json:"inputs"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/embed", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&got))
		require.Equal(t, "hello", got.Inputs)
		_, _ = w.Write([]byte(`[[0.1, 0.2, 0.3]]`))
	}))
	defer srv.Close()

	c := NewTEIClient(srv.URL, 2*time.Second)
	v, err := c.Embed(context.Background(), "hello")
	require.NoError(t, err)
	require.Equal(t, []float32{0.1, 0.2, 0.3}, v)
}

func TestTEIClient_Embed_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewTEIClient(srv.URL, 2*time.Second)
	_, err := c.Embed(context.Background(), "hello")
	require.Error(t, err)
}

func TestTEIClient_Embed_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c := NewTEIClient(srv.URL, 2*time.Second)
	_, err := c.Embed(context.Background(), "hello")
	require.Error(t, err)
}
```

- [x] **Step 2: Run test — expect failure**

```bash
go test ./internal/core/embeddings/... -run TEIClient
```

Expected: compile error (`NewTEIClient` / `TEIClient` not defined).

- [x] **Step 3: Implement `TEIClient`**

Append to `internal/core/embeddings/client.go`:

```go
import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type TEIClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewTEIClient(baseURL string, timeout time.Duration) *TEIClient {
	return &TEIClient{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (c *TEIClient) Embed(ctx context.Context, text string) ([]float32, error) {
	body, err := json.Marshal(map[string]string{"inputs": text})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("tei status %d: %s", resp.StatusCode, string(snippet))
	}

	var out [][]float32
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("tei returned empty embedding list")
	}
	return out[0], nil
}
```

Reorganize the import block so it's a single `import (...)` at the top. The package comment stays at the top of the file; only the `import "context"` line should be replaced by the consolidated block.

- [x] **Step 4: Run tests — expect pass**

```bash
go test ./internal/core/embeddings/... -v
```

Expected: all tests PASS.

- [x] **Step 5: Commit**

```bash
git add internal/core/embeddings/
git commit -m "feat(embeddings): add TEIClient"
```

---

### Task 12: Smoke-test skeleton for TEI

**Files:**
- Create: `internal/core/embeddings/client_smoke_test.go`

- [x] **Step 1: Write the smoke test**

Create `internal/core/embeddings/client_smoke_test.go`. The `//go:build smoke` tag keeps it out of normal `go test ./...`.

```go
//go:build smoke

package embeddings_test

import (
	"context"
	"testing"
	"time"

	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestTEI_RealEmbedding(t *testing.T) {
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "ghcr.io/huggingface/text-embeddings-inference:cpu-1.9",
		Cmd:          []string{"--model-id", "BAAI/bge-m3", "--port", "8080"},
		ExposedPorts: []string{"8080/tcp"},
		WaitingFor:   wait.ForHTTP("/health").WithPort("8080/tcp").WithStartupTimeout(10 * time.Minute),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "8080/tcp")
	require.NoError(t, err)

	client := embeddings.NewTEIClient("http://"+host+":"+port.Port(), 60*time.Second)

	v, err := client.Embed(ctx, "Linktheca is a read-it-later service")
	require.NoError(t, err)
	require.Len(t, v, 1024, "bge-m3 must produce 1024-dim vectors")

	other, err := client.Embed(ctx, "totally unrelated random sentence about penguins")
	require.NoError(t, err)
	require.NotEqual(t, v, other, "different inputs must produce different vectors")
}
```

- [x] **Step 2: Verify build-tag compiles**

```bash
go vet -tags=smoke ./internal/core/embeddings/...
```

Expected: no errors.

- [x] **Step 3: Commit**

```bash
git add internal/core/embeddings/client_smoke_test.go
git commit -m "test(embeddings): add TEI smoke test under build tag"
```

---

## Part D: Radar types and store

### Task 13: Radar types

**Files:**
- Create: `internal/radar/types.go`

- [x] **Step 1: Create the types file**

```go
// Package radar implements the news-monitoring module: topics, feeds,
// subscriptions, crawled findings, and topic↔finding matches backed by
// pgvector similarity search.
package radar

import (
	"errors"
	"time"
)

var (
	ErrNotFound   = errors.New("not found")
	ErrDuplicate  = errors.New("duplicate")
	ErrFeedNotFound = errors.New("feed not found")
)

type Topic struct {
	ID             int64     `json:"id"`
	UserID         int64     `json:"user_id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	MatchThreshold float32   `json:"match_threshold"`
	IsActive       bool      `json:"is_active"`
	HasEmbedding   bool      `json:"has_embedding"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Feed struct {
	ID                   int64      `json:"id"`
	URL                  string     `json:"url"`
	Kind                 string     `json:"kind"`
	Title                *string    `json:"title,omitempty"`
	FetchIntervalSeconds int        `json:"fetch_interval_seconds"`
	IsActive             bool       `json:"is_active"`
	LastFetchedAt        *time.Time `json:"last_fetched_at,omitempty"`
	LastError            *string    `json:"last_error,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
}

type Subscription struct {
	UserID    int64     `json:"user_id"`
	FeedID    int64     `json:"feed_id"`
	CreatedAt time.Time `json:"created_at"`
}

type Finding struct {
	ID           int64      `json:"id"`
	FeedID       int64      `json:"feed_id"`
	ContentID    *int64     `json:"content_id,omitempty"`
	ExternalID   *string    `json:"external_id,omitempty"`
	URL          string     `json:"url"`
	Title        *string    `json:"title,omitempty"`
	Summary      *string    `json:"summary,omitempty"`
	PublishedAt  *time.Time `json:"published_at,omitempty"`
	DiscoveredAt time.Time  `json:"discovered_at"`
	HasEmbedding bool       `json:"has_embedding"`
}

type Match struct {
	ID         int64     `json:"id"`
	TopicID    int64     `json:"topic_id"`
	FindingID  int64     `json:"finding_id"`
	Similarity float32   `json:"similarity"`
	State      string    `json:"state"`
	MatchedAt  time.Time `json:"matched_at"`
}

// DTOs ---------------------------------------------------------------------

type CreateTopicRequest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	MatchThreshold *float32 `json:"match_threshold,omitempty"`
}

type AddFeedRequest struct {
	URL                  string  `json:"url"`
	Kind                 *string `json:"kind,omitempty"`
	FetchIntervalSeconds *int    `json:"fetch_interval_seconds,omitempty"`
}

type SubscribeRequest struct {
	FeedID int64 `json:"feed_id"`
}

// Internal params ---------------------------------------------------------

type CreateTopicParams struct {
	UserID         int64
	Name           string
	Description    string
	MatchThreshold float32
}

type AddFeedParams struct {
	URL                  string
	Kind                 string
	FetchIntervalSeconds int
}

type FindingUpsert struct {
	FeedID      int64
	ExternalID  *string
	URL         string
	Title       *string
	Summary     *string
	PublishedAt *time.Time
}
```

- [x] **Step 2: Compile check**

```bash
go build ./internal/radar/...
```

Expected: success.

- [x] **Step 3: Commit**

```bash
git add internal/radar/types.go
git commit -m "feat(radar): add module types and DTOs"
```

---

### Task 14: Radar store — topics

**Files:**
- Create: `internal/radar/store.go`
- Create: `internal/radar/store_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/radar/store_test.go`:

```go
package radar_test

import (
	"context"
	"testing"

	"github.com/ismd/linktheca/internal/radar"
	"github.com/ismd/linktheca/internal/testing/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	"github.com/stretchr/testify/require"
)

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

func TestStore_CreateTopic(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)

	topic, err := store.CreateTopic(ctx, radar.CreateTopicParams{
		UserID:         userID,
		Name:           "AI",
		Description:    "ML research and products",
		MatchThreshold: 0.8,
	})
	require.NoError(t, err)
	require.NotZero(t, topic.ID)
	require.Equal(t, userID, topic.UserID)
	require.Equal(t, "AI", topic.Name)
	require.Equal(t, float32(0.8), topic.MatchThreshold)
	require.True(t, topic.IsActive)
	require.False(t, topic.HasEmbedding)
}

func TestStore_UpdateTopicEmbedding(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)
	topic, err := store.CreateTopic(ctx, radar.CreateTopicParams{
		UserID: userID, Name: "x", Description: "y", MatchThreshold: 0.75,
	})
	require.NoError(t, err)

	vec := make([]float32, 1024)
	for i := range vec {
		vec[i] = 0.01
	}
	err = store.UpdateTopicEmbedding(ctx, topic.ID, pgvector.NewVector(vec))
	require.NoError(t, err)

	// Reload via raw SQL to assert embedding is non-null.
	var isNull bool
	err = pool.QueryRow(ctx,
		`SELECT embedding IS NULL FROM radar_topics WHERE id=$1`, topic.ID).Scan(&isNull)
	require.NoError(t, err)
	require.False(t, isNull)
}
```

- [ ] **Step 2: Run test — expect failure**

```bash
go test ./internal/radar/... -run TestStore_CreateTopic -v
```

Expected: compile error (`NewStore`, `CreateTopic` missing).

- [ ] **Step 3: Implement store (topics methods)**

Create `internal/radar/store.go`:

```go
package radar

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

func (s *Store) CreateTopic(ctx context.Context, p CreateTopicParams) (*Topic, error) {
	row := s.db.QueryRow(ctx, `
		INSERT INTO radar_topics (user_id, name, description, match_threshold)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, name, description, match_threshold, is_active,
		          embedding IS NOT NULL, created_at, updated_at
	`, p.UserID, p.Name, p.Description, p.MatchThreshold)

	var t Topic
	if err := row.Scan(&t.ID, &t.UserID, &t.Name, &t.Description,
		&t.MatchThreshold, &t.IsActive, &t.HasEmbedding, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, fmt.Errorf("create topic: %w", err)
	}
	return &t, nil
}

func (s *Store) UpdateTopicEmbedding(ctx context.Context, topicID int64, vec pgvector.Vector) error {
	cmd, err := s.db.Exec(ctx,
		`UPDATE radar_topics SET embedding=$1, updated_at=now() WHERE id=$2`,
		vec, topicID)
	if err != nil {
		return fmt.Errorf("update topic embedding: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// wrapPgError converts known Postgres errors into package-level sentinels.
func wrapPgError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique violation
			return ErrDuplicate
		case "23503": // foreign key violation
			return ErrFeedNotFound
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
```

- [ ] **Step 4: Run tests — expect pass**

```bash
go test ./internal/radar/... -run TestStore_ -v
```

Expected: both PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/radar/
git commit -m "feat(radar): add store with CreateTopic and UpdateTopicEmbedding"
```

---

### Task 15: Radar store — feeds and subscriptions

**Files:**
- Modify: `internal/radar/store.go`
- Modify: `internal/radar/store_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/radar/store_test.go`:

```go
func TestStore_AddFeed(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	feed, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: "https://example.com/feed.xml", Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)
	require.NotZero(t, feed.ID)
	require.Equal(t, "rss", feed.Kind)
	require.True(t, feed.IsActive)

	_, err = store.AddFeed(ctx, radar.AddFeedParams{
		URL: "https://example.com/feed.xml", Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.ErrorIs(t, err, radar.ErrDuplicate)
}

func TestStore_Subscribe(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)
	feed, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: "https://a.example/feed.xml", Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)

	sub, err := store.Subscribe(ctx, userID, feed.ID)
	require.NoError(t, err)
	require.Equal(t, userID, sub.UserID)
	require.Equal(t, feed.ID, sub.FeedID)

	// Idempotent.
	sub2, err := store.Subscribe(ctx, userID, feed.ID)
	require.NoError(t, err)
	require.Equal(t, sub.CreatedAt, sub2.CreatedAt)

	// Non-existent feed → ErrFeedNotFound.
	_, err = store.Subscribe(ctx, userID, 999999)
	require.ErrorIs(t, err, radar.ErrFeedNotFound)
}
```

- [ ] **Step 2: Run — expect failure**

- [ ] **Step 3: Implement `AddFeed` and `Subscribe`**

Append to `internal/radar/store.go`:

```go
func (s *Store) AddFeed(ctx context.Context, p AddFeedParams) (*Feed, error) {
	row := s.db.QueryRow(ctx, `
		INSERT INTO radar_feeds (url, kind, fetch_interval_seconds)
		VALUES ($1, $2, $3)
		RETURNING id, url, kind, title, fetch_interval_seconds, is_active,
		          last_fetched_at, last_error, created_at
	`, p.URL, p.Kind, p.FetchIntervalSeconds)

	var f Feed
	if err := row.Scan(&f.ID, &f.URL, &f.Kind, &f.Title,
		&f.FetchIntervalSeconds, &f.IsActive,
		&f.LastFetchedAt, &f.LastError, &f.CreatedAt); err != nil {
		return nil, wrapPgError(err)
	}
	return &f, nil
}

func (s *Store) Subscribe(ctx context.Context, userID, feedID int64) (*Subscription, error) {
	row := s.db.QueryRow(ctx, `
		INSERT INTO radar_feed_subscriptions (user_id, feed_id)
		VALUES ($1, $2)
		ON CONFLICT (user_id, feed_id) DO UPDATE SET created_at = radar_feed_subscriptions.created_at
		RETURNING user_id, feed_id, created_at
	`, userID, feedID)

	var sub Subscription
	if err := row.Scan(&sub.UserID, &sub.FeedID, &sub.CreatedAt); err != nil {
		return nil, wrapPgError(err)
	}
	return &sub, nil
}
```

- [ ] **Step 4: Run tests — expect pass**

```bash
go test ./internal/radar/... -run TestStore_ -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/radar/
git commit -m "feat(radar): add AddFeed and Subscribe to store"
```

---

### Task 16: Radar store — crawler-facing helpers

**Files:**
- Modify: `internal/radar/store.go`
- Modify: `internal/radar/store_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/radar/store_test.go`:

```go
func TestStore_ListDueFeeds(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	a, _ := store.AddFeed(ctx, radar.AddFeedParams{URL: "https://a.example/f", Kind: "rss", FetchIntervalSeconds: 3600})
	b, _ := store.AddFeed(ctx, radar.AddFeedParams{URL: "https://b.example/f", Kind: "rss", FetchIntervalSeconds: 3600})

	// b has been fetched recently; only a should be "due".
	_, err := pool.Exec(ctx, `UPDATE radar_feeds SET last_fetched_at = now() WHERE id = $1`, b.ID)
	require.NoError(t, err)

	due, err := store.ListDueFeeds(ctx, 100)
	require.NoError(t, err)
	ids := make(map[int64]bool)
	for _, id := range due {
		ids[id] = true
	}
	require.True(t, ids[a.ID])
	require.False(t, ids[b.ID])
}

func TestStore_UpsertFinding(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	feed, _ := store.AddFeed(ctx, radar.AddFeedParams{URL: "https://feed.example/f", Kind: "rss", FetchIntervalSeconds: 3600})

	ext := "guid-1"
	title := "hello"
	f1, created, err := store.UpsertFinding(ctx, radar.FindingUpsert{
		FeedID: feed.ID, ExternalID: &ext, URL: "https://post.example/1", Title: &title,
	})
	require.NoError(t, err)
	require.True(t, created)
	require.NotZero(t, f1.ID)

	_, created2, err := store.UpsertFinding(ctx, radar.FindingUpsert{
		FeedID: feed.ID, ExternalID: &ext, URL: "https://post.example/1", Title: &title,
	})
	require.NoError(t, err)
	require.False(t, created2, "second upsert with same (feed_id, external_id) must not create")
}
```

- [ ] **Step 2: Run — expect compile failure**

- [ ] **Step 3: Implement helpers**

Append to `internal/radar/store.go`:

```go
// ListDueFeeds returns IDs of active feeds whose next-fetch moment has passed.
func (s *Store) ListDueFeeds(ctx context.Context, limit int) ([]int64, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id FROM radar_feeds
		WHERE is_active
		  AND (last_fetched_at IS NULL
		       OR last_fetched_at + fetch_interval_seconds * interval '1 second' < now())
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list due feeds: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

type FeedFetchState struct {
	URL          string
	Etag         *string
	LastModified *string
}

func (s *Store) GetFeedForFetch(ctx context.Context, feedID int64) (*FeedFetchState, error) {
	row := s.db.QueryRow(ctx,
		`SELECT url, etag, last_modified FROM radar_feeds WHERE id=$1`, feedID)
	var st FeedFetchState
	if err := row.Scan(&st.URL, &st.Etag, &st.LastModified); err != nil {
		return nil, wrapPgError(err)
	}
	return &st, nil
}

func (s *Store) MarkFeedFetched(ctx context.Context, feedID int64, etag, lastModified *string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE radar_feeds
		SET last_fetched_at = now(), etag = $1, last_modified = $2, last_error = NULL
		WHERE id = $3
	`, etag, lastModified, feedID)
	return err
}

func (s *Store) MarkFeedError(ctx context.Context, feedID int64, errMsg string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE radar_feeds SET last_fetched_at = now(), last_error = $1 WHERE id = $2
	`, errMsg, feedID)
	return err
}

// UpsertFinding inserts a finding; returns (finding, created=true) if new, else (existing, false).
func (s *Store) UpsertFinding(ctx context.Context, p FindingUpsert) (*Finding, bool, error) {
	row := s.db.QueryRow(ctx, `
		INSERT INTO radar_findings (feed_id, external_id, url, title, summary, published_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (feed_id, external_id) DO NOTHING
		RETURNING id, feed_id, content_id, external_id, url, title, summary,
		          published_at, discovered_at, embedding IS NOT NULL
	`, p.FeedID, p.ExternalID, p.URL, p.Title, p.Summary, p.PublishedAt)

	var f Finding
	err := row.Scan(&f.ID, &f.FeedID, &f.ContentID, &f.ExternalID, &f.URL,
		&f.Title, &f.Summary, &f.PublishedAt, &f.DiscoveredAt, &f.HasEmbedding)
	if err == nil {
		return &f, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, fmt.Errorf("upsert finding: %w", err)
	}

	// Conflict path — fetch the existing row.
	existing, err := s.GetFindingByExternalID(ctx, p.FeedID, p.ExternalID)
	if err != nil {
		return nil, false, err
	}
	return existing, false, nil
}

func (s *Store) GetFindingByExternalID(ctx context.Context, feedID int64, externalID *string) (*Finding, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, feed_id, content_id, external_id, url, title, summary,
		       published_at, discovered_at, embedding IS NOT NULL
		FROM radar_findings
		WHERE feed_id = $1 AND external_id IS NOT DISTINCT FROM $2
	`, feedID, externalID)
	var f Finding
	if err := row.Scan(&f.ID, &f.FeedID, &f.ContentID, &f.ExternalID, &f.URL,
		&f.Title, &f.Summary, &f.PublishedAt, &f.DiscoveredAt, &f.HasEmbedding); err != nil {
		return nil, wrapPgError(err)
	}
	return &f, nil
}
```

- [ ] **Step 4: Run tests — expect pass**

```bash
go test ./internal/radar/... -run TestStore_ -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/radar/
git commit -m "feat(radar): add feed-fetch and finding-upsert store methods"
```

---

### Task 17: Radar store — embedding and matching

**Files:**
- Modify: `internal/radar/store.go`
- Modify: `internal/radar/store_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/radar/store_test.go`:

```go
func TestStore_UpdateFindingEmbedding_AndMatch(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)
	topic, err := store.CreateTopic(ctx, radar.CreateTopicParams{
		UserID: userID, Name: "ai", Description: "artificial intelligence", MatchThreshold: 0.5,
	})
	require.NoError(t, err)

	// Topic embedding.
	vec := make([]float32, 1024)
	vec[0] = 1.0
	err = store.UpdateTopicEmbedding(ctx, topic.ID, pgvector.NewVector(vec))
	require.NoError(t, err)

	feed, err := store.AddFeed(ctx, radar.AddFeedParams{URL: "https://f.example/x", Kind: "rss", FetchIntervalSeconds: 3600})
	require.NoError(t, err)
	_, err = store.Subscribe(ctx, userID, feed.ID)
	require.NoError(t, err)

	ext := "g1"
	f, _, err := store.UpsertFinding(ctx, radar.FindingUpsert{
		FeedID: feed.ID, ExternalID: &ext, URL: "https://p.example/1",
	})
	require.NoError(t, err)

	// Finding embedding identical → cosine distance 0 → similarity 1.
	err = store.UpdateFindingEmbedding(ctx, f.ID, pgvector.NewVector(vec))
	require.NoError(t, err)

	n, err := store.MatchFindingToTopics(ctx, f.ID)
	require.NoError(t, err)
	require.Equal(t, int64(1), n, "exactly one topic should match")

	var sim float32
	err = pool.QueryRow(ctx,
		`SELECT similarity FROM radar_topic_matches WHERE topic_id=$1 AND finding_id=$2`,
		topic.ID, f.ID).Scan(&sim)
	require.NoError(t, err)
	require.InDelta(t, 1.0, sim, 0.001)
}
```

- [ ] **Step 2: Run — expect compile failure**

- [ ] **Step 3: Implement**

Append to `internal/radar/store.go`:

```go
func (s *Store) UpdateFindingEmbedding(ctx context.Context, findingID int64, vec pgvector.Vector) error {
	cmd, err := s.db.Exec(ctx,
		`UPDATE radar_findings SET embedding = $1 WHERE id = $2`, vec, findingID)
	if err != nil {
		return fmt.Errorf("update finding embedding: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// MatchFindingToTopics inserts matches for all subscribed+active topics of
// the finding's feed where cosine similarity ≥ topic.match_threshold.
// Returns the number of matches inserted (existing rows are not counted).
func (s *Store) MatchFindingToTopics(ctx context.Context, findingID int64) (int64, error) {
	cmd, err := s.db.Exec(ctx, `
		INSERT INTO radar_topic_matches (topic_id, finding_id, similarity, state)
		SELECT rt.id,
		       $1,
		       1 - (rt.embedding <=> f.embedding) AS similarity,
		       'new'
		FROM radar_topics rt
		JOIN radar_feed_subscriptions rfs ON rfs.user_id = rt.user_id
		JOIN radar_findings f ON f.id = $1
		WHERE rfs.feed_id = f.feed_id
		  AND rt.is_active
		  AND rt.embedding IS NOT NULL
		  AND f.embedding IS NOT NULL
		  AND 1 - (rt.embedding <=> f.embedding) >= rt.match_threshold
		ON CONFLICT (topic_id, finding_id) DO NOTHING
	`, findingID)
	if err != nil {
		return 0, fmt.Errorf("match finding: %w", err)
	}
	return cmd.RowsAffected(), nil
}

type FindingForEmbed struct {
	ID           int64
	Title        *string
	Summary      *string
	HasEmbedding bool
}

func (s *Store) GetFindingForEmbed(ctx context.Context, findingID int64) (*FindingForEmbed, error) {
	row := s.db.QueryRow(ctx,
		`SELECT id, title, summary, embedding IS NOT NULL
		 FROM radar_findings WHERE id = $1`, findingID)
	var f FindingForEmbed
	if err := row.Scan(&f.ID, &f.Title, &f.Summary, &f.HasEmbedding); err != nil {
		return nil, wrapPgError(err)
	}
	return &f, nil
}
```

- [ ] **Step 4: Run tests — expect pass**

```bash
go test ./internal/radar/... -run TestStore_ -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/radar/
git commit -m "feat(radar): add finding embedding update and match query"
```

---
