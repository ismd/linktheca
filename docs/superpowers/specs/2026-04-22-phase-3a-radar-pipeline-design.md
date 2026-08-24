# Phase 3a: Radar Pipeline Backend — design

**Date:** 2026-04-22
**Status:** approved, ready for writing-plans
**Predecessors:** phase 1 (auth, done), phase 2 (library backend, done)

## Context

After phase 2, Linktheca has a working read-it-later backend: the auth flow plus
Library CRUD. Phase 3a adds the **Radar module's backend** — news monitoring for
subscribed topics through local embeddings.

Phase 3a's scope is deliberately cut down to "the pipeline is alive end to end"
without the full HTTP CRUD. That isolates the riskiest technical piece (TEI +
pgvector + the River job queue) into its own iteration. The full HTTP API
(List/Update/Delete for topics and feeds, a user-facing `/radar/feed`, a reader
view for findings) moves to phase 3b.

The main architectural decisions settled during brainstorming:
1. **TEI instead of Ollama** — the HuggingFace Text Embeddings Inference server
   with the bge-m3 model (1024 dim). Ollama was rejected: for an embeddings-only
   job, TEI is faster and simpler to run. Future LLM features will go through a
   cloud API rather than self-hosting, behind their own opt-in flags.
2. **The CLI always works over HTTP** — no direct database access from the CLI.
   In phase 3a the CLI gets the minimum POST endpoints for bootstrapping (topics,
   feeds, subscriptions); the rest arrive in phase 3b.
3. **Two binaries:** the existing `cmd/linktheca` is renamed to
   `cmd/linktheca-server`; the new `cmd/linktheca` is a cobra-based CLI tool for
   remote production use.
4. **Radar is optional** — `LINKTHECA_RADAR_ENABLED` (default true) controls the
   backend side of the feature. When false, the service runs as a plain
   read-it-later with no TEI.

## 1. Goal and scope

**The goal of phase 3a:** the Radar backend pipeline runs end to end on real RSS
data. An admin registers a feed through the CLI, and a user creates a topic and a
subscription through the CLI. From there River workers periodically pull the RSS,
embed the findings through TEI, and match them against topics by cosine
similarity. The results sit in `radar_topic_matches` and are reachable through
direct SQL (an HTTP read endpoint is phase 3b).

**In scope:**
- Five migrations: `radar_topics`, `radar_feeds`, `radar_feed_subscriptions`,
  `radar_findings`, `radar_topic_matches` (numbers 006–010; the pgvector
  extension already exists in migration 001)
- The `internal/core/embeddings/` package — an HTTP client for TEI plus a
  `FakeEmbedder` for tests
- The `internal/radar/` package with `types.go`, `store.go`, `service.go`, and a
  minimal `http.go` (three POST handlers)
- The `internal/radar/crawler/` (a gofeed wrapper) and `internal/radar/jobs/`
  (the River workers: Scheduler, CrawlFeed, EmbedFinding, MatchFinding)
  subpackages
- A new `cmd/linktheca/` binary (the CLI) with the `auth *` and `radar *`
  subcommands
- Renaming `cmd/linktheca/` → `cmd/linktheca-server/`
- Env config: `LINKTHECA_TEI_URL`, `LINKTHECA_TEI_TIMEOUT`,
  `LINKTHECA_EMBEDDING_DIM`, `LINKTHECA_RADAR_ENABLED`,
  `LINKTHECA_RADAR_SCHEDULER_INTERVAL`, `LINKTHECA_RADAR_MAX_WORKERS`,
  `LINKTHECA_URL`
- `compose.dev.yaml` gains a `tei` service
- A smoke test behind `-tags=smoke` against a real TEI container

**Out of scope (phase 3b or later):**
- The `GET /radar/topics`, `GET /radar/feeds`, `GET /radar/feed`,
  `GET /radar/findings/:id/read` endpoints, and Update/Delete on
  topics/feeds/subscriptions
- CSV/OPML import in the CLI
- Fetching full article content on demand, and the reader view
- Notifications of any kind
- Feed autodiscovery
- The admin UI, the production docker-compose, the production deployment story

## 2. File layout

```
linktheca/
├── cmd/
│   ├── linktheca-server/         # renamed from cmd/linktheca/
│   │   └── main.go
│   └── linktheca/                # new: the CLI
│       ├── main.go               # registers the cobra root
│       └── internal/
│           └── cli/
│               ├── root.go
│               ├── session/
│               │   ├── session.go          # ~/.config/linktheca/session.json
│               │   └── session_test.go
│               ├── apiclient/
│               │   ├── client.go           # HTTP with auto-refresh
│               │   ├── client_test.go
│               │   ├── auth.go             # auth endpoints
│               │   └── radar.go            # radar endpoints
│               ├── output/
│               │   ├── format.go           # table/json printers
│               │   └── format_test.go
│               └── cmd/
│                   ├── auth_register.go
│                   ├── auth_login.go
│                   ├── auth_logout.go
│                   ├── auth_whoami.go
│                   ├── radar_topic_add.go
│                   ├── radar_feed_add.go
│                   └── radar_subscribe.go
│
├── migrations/
│   ├── 006_radar_topics.sql
│   ├── 007_radar_feeds.sql
│   ├── 008_radar_feed_subscriptions.sql
│   ├── 009_radar_findings.sql
│   └── 010_radar_topic_matches.sql
│
├── internal/
│   ├── core/
│   │   └── embeddings/
│   │       ├── client.go                   # the Client interface + TEIClient
│   │       ├── client_test.go              # unit
│   │       ├── fake.go                     # FakeEmbedder
│   │       ├── fake_test.go
│   │       └── client_smoke_test.go        # //go:build smoke
│   ├── radar/
│   │   ├── types.go                        # Topic, Feed, Subscription, Finding, Match, DTOs
│   │   ├── store.go                        # SQL for the five tables
│   │   ├── store_test.go                   # integration with testdb + pgvector
│   │   ├── service.go                      # CreateTopic, AddFeed, Subscribe
│   │   ├── service_test.go                 # unit with a mock store + FakeEmbedder
│   │   ├── http.go                         # three POST handlers
│   │   ├── http_test.go                    # unit (validation)
│   │   ├── integration_test.go             # HTTP → service → store end to end
│   │   ├── crawler/
│   │   │   ├── crawler.go                  # the Fetcher interface + gofeed
│   │   │   └── crawler_test.go             # synthetic RSS, etag
│   │   └── jobs/
│   │       ├── jobs.go                     # River setup, args types
│   │       ├── crawl_feed.go
│   │       ├── embed_finding.go
│   │       ├── match_finding.go
│   │       ├── scheduler.go
│   │       ├── jobs_test.go                # integration against a real database
│   │       └── smoke_test.go               # //go:build smoke
│   └── server/
│       └── server.go                       # MODIFIED: the TEI client, the River client, radar wiring
│
├── compose.dev.yaml                         # MODIFIED: + the tei service
├── Makefile                                 # MODIFIED: smoke-radar, server-build, cli-build
└── embeds.go                                # MODIFIED: embed the new migrations
```

**Key layout decisions:**

- **CLI-specific packages live under `cmd/linktheca/internal/cli/`** (not in the
  top-level `internal/`). Backend code must not transitively pull in
  cobra/viper. Go convention: CLI-tool-specific code sits next to its cmd.
- **`jobs/` is a separate subpackage of `internal/radar/`** so `service.go` does
  not drag in a dependency on River. The service is pure business logic; jobs are
  orchestration.
- **`http.go` exists in phase 3a**, but holds only three POST handlers
  (~100–150 lines). The full CRUD is phase 3b.
- **Migrations are numbered from 006**, continuing the sequence. The pgvector
  extension migration already ran in `001_init.sql` and does not need repeating.

## 3. Migrations 006–010

Conventions: `BIGINT GENERATED ALWAYS AS IDENTITY` (users.id is `INT`),
`user_id INT REFERENCES users(id) ON DELETE CASCADE`, named indexes.

### 006_radar_topics.sql

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

`embedding` is nullable: the row is created in two steps (INSERT → TEI call →
UPDATE) so no transaction is held open across an HTTP request.

### 007_radar_feeds.sql

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

Feeds are global (no `user_id`) — one RSS source is parsed once for all its
subscribers.

### 008_radar_feed_subscriptions.sql

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

### 009_radar_findings.sql

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

`content_id` is nullable: when something is discovered in a feed we do not parse
the full content, only the metadata. A full `article_contents` row appears only
when a user opens the finding (phase 3b).

The HNSW index serves kNN search in the other direction (phase 3b: "give me the
findings similar to this topic"). MatchJob (phase 3a: "give me every topic close
to this one finding") does not use it, but it is in place ahead of time.

### 010_radar_topic_matches.sql

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

### River migrations

As a separate step at `linktheca-server` startup we call
`rivermigrate.New(pool, nil).Migrate(ctx, rivermigrate.DirectionUp, nil)`, which
creates the `river_job`, `river_leader`, `river_queue`, and `river_migration`
tables. River's versioning does not intersect with goose's.

Startup order: goose → the River migrator → starting the River client → the HTTP
server.

## 4. TEI integration

### Docker Compose — `compose.dev.yaml`

A `tei` service is added:

```yaml
services:
  postgres:
    # the existing service, unchanged

  tei:
    image: ghcr.io/huggingface/text-embeddings-inference:cpu-1.9
    command: --model-id BAAI/bge-m3 --port 8080
    ports:
      - "8081:8080"                # host port 8081 to avoid colliding with the backend
    volumes:
      - tei-data:/data             # the model cache (~2.3 GB)
    healthcheck:
      test: ["CMD", "curl", "-fs", "http://localhost:8080/health"]
      interval: 10s
      timeout: 5s
      retries: 10
      start_period: 120s           # the first start pulls the model from HF
    restart: unless-stopped

volumes:
  postgres-data:
  tei-data:
```

In dev the backend runs outside compose (as in phases 1 and 2):
`make dev-server` with `LINKTHECA_TEI_URL=http://localhost:8081`.

The production compose is a future phase, not phase 3a.

### The `internal/core/embeddings/` package

**The interface and the TEI implementation:**

```go
package embeddings

type Client interface {
    Embed(ctx context.Context, text string) ([]float32, error)
}

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

// POST {baseURL}/embed {"inputs":"text"} → [[f32, ...]]
// Returns the first array.
func (c *TEIClient) Embed(ctx context.Context, text string) ([]float32, error) { /* ... */ }
```

**FakeEmbedder for tests:**

```go
type FakeEmbedder struct {
    Dim int
}

// Deterministic: SHA-256(text) is stretched to Dim elements and L2-normalized.
// The same text yields the same vector. Different texts are far apart by cosine.
func (f *FakeEmbedder) Embed(ctx context.Context, text string) ([]float32, error) { /* ... */ }
```

**The smoke test:**

```go
//go:build smoke

// Brings up a TEI testcontainer with bge-m3, calls Embed, checks len == 1024
// and that different texts produce different vectors.
```

### Integration with pgvector

We use `github.com/pgvector/pgvector-go`:

```go
import "github.com/pgvector/pgvector-go"

vec, _ := teiClient.Embed(ctx, description)
_, err := pool.Exec(ctx,
    `UPDATE radar_topics SET embedding = $1, updated_at = now() WHERE id = $2`,
    pgvector.NewVector(vec), topicID)
```

`pgx` serializes a `pgvector.Vector` into a `vector(1024)` column.

### Validation at startup

`EMBEDDING_DIM = 1024` appears in the config. At server startup (when
`RADAR_ENABLED=true`) we call `embedder.Embed(ctx, "ping")` and check
`len(vec) == EMBEDDING_DIM`. On a mismatch we log a warning but do not fail fast
(TEI may be temporarily unavailable; the server starts, and the `/radar/*`
endpoints will later answer 503 on create-topic).

### The dimensionality constraint

`vector(1024)` is baked into migrations 006 and 009. Changing the model would
require:
1. A new migration altering the column dimensionality.
2. Recomputing **every** `radar_topics.embedding` and
   `radar_findings.embedding`.
3. Rebuilding the HNSW index.

That is a long-term architectural decision, not in phase 3a's scope.

## 5. The pipeline: River plus jobs

Four kinds of job:

```
SchedulerJob (periodic, every 5 minutes)
    └─► CrawlFeedJob (one per feed)
            └─► EmbedFindingJob (one per new finding)
                    └─► MatchFindingJob (one per finding that has an embedding)
```

All the workers run in the same process as the HTTP server (one binary).

**Wiring:** the River client is created in `server.go` (the DI layer, which owns
the pool). The set of workers and periodic jobs is assembled in the
`internal/radar/jobs` package (through something like
`jobs.RegisterWorkers(workers *river.Workers, service *radar.Service, embedder embeddings.Client)`),
and `server.go` passes the resulting `*river.Workers` into `river.Config`.

### SchedulerJob

River supports periodic jobs natively (`river.PeriodicJob`). Registered at
startup in `server.go`:

```go
riverClient, _ := river.NewClient(riverpgxv5.New(pool), &river.Config{
    Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: cfg.RadarMaxWorkers}},
    Workers: workers,
    PeriodicJobs: []*river.PeriodicJob{
        river.NewPeriodicJob(
            river.PeriodicInterval(cfg.RadarSchedulerInterval),
            func() (river.JobArgs, *river.InsertOpts) {
                return jobs.ScheduleCrawlsArgs{}, nil
            },
            &river.PeriodicJobOpts{RunOnStart: true},
        ),
    },
})
```

Inside the `ScheduleCrawlsWorker`:

```sql
SELECT id FROM radar_feeds
WHERE is_active
  AND (last_fetched_at IS NULL
       OR last_fetched_at + fetch_interval_seconds * interval '1 second' < now())
LIMIT 100;
```

For each id, `client.Insert(ctx, CrawlFeedArgs{FeedID: id}, nil)`.

### CrawlFeedJob

Argument: `FeedID int64`. The flow:

1. `SELECT * FROM radar_feeds WHERE id = $1` — fetches the url, etag, and
   last_modified.
2. HTTP GET with `If-None-Match` and `If-Modified-Since`. On a 304 →
   `UPDATE last_fetched_at = now(), last_error = NULL` and exit.
3. Parse through `mmcdole/gofeed.Parser{}.Parse(reader)`.
4. For each item:
   ```sql
   INSERT INTO radar_findings (feed_id, external_id, url, title, summary, published_at)
   VALUES ($1, $2, $3, $4, $5, $6)
   ON CONFLICT (feed_id, external_id) DO NOTHING
   RETURNING id;
   ```
   For each id returned, `client.Insert(ctx, EmbedFindingArgs{FindingID: id}, nil)`.
5. `UPDATE radar_feeds SET last_fetched_at = now(), etag = $1, last_modified = $2, last_error = NULL`.

Network and parse errors:
`UPDATE radar_feeds SET last_error = $1, last_fetched_at = now()`, then return an
`error` → River retries with exponential backoff (25 attempts over ~24 hours by
default).

This step does **NOT parse the full article content** — only the metadata from
the RSS. A full `article_contents` row is created in phase 3b when the finding is
opened.

### EmbedFindingJob

Argument: `FindingID int64`. The flow:

1. `SELECT embedding, title, summary FROM radar_findings WHERE id = $1`.
2. If `embedding IS NOT NULL`, exit (idempotency on repeats).
3. `text := strings.TrimSpace(title + "\n" + summary)`. If it is empty, finish
   without an error.
4. `vec, err := embedder.Embed(ctx, text)`. A TEI error → return the error →
   River retries.
5. `UPDATE radar_findings SET embedding = $1 WHERE id = $2` with
   `pgvector.NewVector(vec)`.
6. `client.Insert(ctx, MatchFindingArgs{FindingID: id}, nil)`.

We embed **title + summary, not the full text**. It is cheaper, and it is enough
for a short news item.

### MatchFindingJob

Argument: `FindingID int64`. One SQL statement:

```sql
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
  AND 1 - (rt.embedding <=> f.embedding) >= rt.match_threshold
ON CONFLICT (topic_id, finding_id) DO NOTHING;
```

`<=>` is pgvector's cosine distance. Similarity = `1 - distance`. Matching runs
against **every subscribed topic of every subscribed user** in a single query.

### Idempotency and recovery

- **Restarting the backend mid-flight:** River persists jobs in Postgres, and
  they are picked up again after the restart.
- **Re-running a job:** `ON CONFLICT DO NOTHING` on the INSERTs, and the
  `embedding IS NOT NULL` check before embedding.
- **Rate limiting for TEI:** `RadarMaxWorkers = 5` by default — at most five
  concurrent embedding requests.

### Interfaces for testing

```go
// internal/radar/crawler/crawler.go
type Fetcher interface {
    Fetch(ctx context.Context, url, etag, lastModified string) (*FetchResult, error)
}
type FetchResult struct {
    StatusCode   int
    Body         []byte
    Etag         string
    LastModified string
    NotModified  bool
}
```

In the `CrawlFeedJob` tests: a fake `Fetcher` returns prepared RSS XML, a
`FakeEmbedder`, and a real database through testdb.

### Accepted trade-offs

- No deduplication of similar findings across feeds (one news item from five RSS
  feeds appears five times).
- No rate limiting on TEI beyond `MaxWorkers`.
- SchedulerJob takes 100 feeds per cycle — enough for self-hosting.
- No smart backoff for broken feeds (it retries indefinitely; the operator fixes
  it).

## 6. HTTP endpoints

### Routes

```
POST /radar/topics           RequireUser         creates a topic and computes its embedding
POST /radar/feeds            RequireAdmin        registers a global feed
POST /radar/subscriptions    RequireUser         subscribes the authed user
```

Every other Radar endpoint (List/Update/Delete, `GET /radar/feed`,
`GET /radar/findings/:id/read`) is phase 3b.

### `POST /radar/topics`

Request:
```json
{
  "name": "AI and startups",
  "description": "Fundraising, product launches, and foundation-model benchmarks.",
  "match_threshold": 0.75
}
```

Validation:
- `name` — 1..200 characters, required.
- `description` — 10..2000 characters, required.
- `match_threshold` — optional, [0.0, 1.0], default 0.75.

The handler flow:
1. Validate through `go-playground/validator`.
2. `radarService.CreateTopic(ctx, userID, dto)`.
3. Service: INSERT → `embedder.Embed(ctx, description)` synchronously → UPDATE
   the embedding.
4. Return `201 Created` plus the topic object as JSON (without the raw embedding
   — only `has_embedding: true`).

Why the embedding is computed synchronously in the handler: the user expects a
ready-to-match topic. A TEI call of ~200–800ms is acceptable. If TEI is
unavailable the handler returns `503`, the topic sits in the database with
`embedding = NULL`, and the next MatchJob skips it
(`rt.embedding IS NOT NULL`).

Error cases:
- Validation → `400 Bad Request`.
- A TEI timeout or 5xx → `503 Service Unavailable`.

### `POST /radar/feeds`

Request:
```json
{
  "url": "https://news.ycombinator.com/rss",
  "kind": "rss",
  "fetch_interval_seconds": 3600
}
```

Validation:
- `url` — required, http(s), ≤ 2000 characters.
- `kind` — optional, `rss|atom`, default `rss`.
- `fetch_interval_seconds` — optional, 300..86400, default 3600.

The handler flow:
1. Validate.
2. INSERT the feed, with no synchronous fetch.
3. `201 Created` plus the feed object.

The first crawl happens within 5 minutes (the SchedulerJob tick), or immediately
under `RunOnStart: true`.

Error cases:
- A duplicate URL → `409 Conflict`.
- Validation → `400`.

### `POST /radar/subscriptions`

Request:
```json
{ "feed_id": 42 }
```

The handler flow:
1. Validate.
2. `INSERT ... ON CONFLICT DO NOTHING`.
3. `201 Created` plus the subscription object. Idempotent.

Error cases:
- A `feed_id` that does not exist → an FK violation → `404 Not Found`.

### Routing in server.go

```go
if cfg.RadarEnabled {
    r.Route("/radar", func(r chi.Router) {
        r.Use(auth.RequireUser)
        r.Post("/topics", radarHTTP.CreateTopic)
        r.Post("/subscriptions", radarHTTP.Subscribe)
        r.Group(func(r chi.Router) {
            r.Use(auth.RequireAdmin)
            r.Post("/feeds", radarHTTP.AddFeed)
        })
    })
} else {
    r.Route("/radar", func(r chi.Router) {
        r.HandleFunc("/*", radarHTTP.DisabledHandler)  // 501 with {"error":"radar_disabled"}
    })
}
```

### OpenAPI

Phase 3a does not write an OpenAPI spec. The CLI talks to HTTP directly. When a
frontend appears, that is its own phase with an `openapi.yaml` and an
`openapi-typescript` pipeline.

## 7. The `linktheca` CLI

### Framework and structure

- `github.com/spf13/cobra` for subcommand routing.
- The path: `cmd/linktheca/main.go` plus `cmd/linktheca/internal/cli/...`.
- The existing `cmd/linktheca/` is renamed to `cmd/linktheca-server/` **before**
  any CLI code is added (in one commit at the start of phase 3a).

### The session

```
~/.config/linktheca/session.json       # XDG_CONFIG_HOME/linktheca, permissions 0600
{
  "server_url": "http://localhost:8080",
  "access_token": "eyJ...",
  "refresh_token": "base64string",
  "user_id": 42,
  "is_admin": true,
  "expires_at": "2026-04-22T15:00:00Z"
}
```

The path is overridable through `LINKTHECA_CONFIG_DIR` (for tests).

### Global flags

```
--server URL          # overrides server_url from session.json; env LINKTHECA_URL
--config PATH         # an explicit path to session.json
--output FORMAT       # table | json, default table
```

### Phase 3a subcommands

**Auth:**
```
linktheca auth register --email=... --password=... --display-name=...
linktheca auth login --email=... --password=...
linktheca auth logout
linktheca auth whoami
```

`register` and `login` save the tokens into session.json. `whoami` calls
`GET /auth/me` with auto-refresh.

**Radar:**
```
linktheca radar topic add --name="AI" --description="..." [--threshold=0.75]
linktheca radar feed add --url="..." [--kind=rss] [--interval=3600]    # admin only
linktheca radar subscribe --feed-id=N
```

Exit code 0 on 2xx, 1 on 4xx/5xx.

### Auto-refreshing the access token

In the CLI's HTTP client:

```go
func (c *APIClient) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
    resp, err := c.httpClient.Do(c.withBearer(req))
    if err != nil { return nil, err }
    if resp.StatusCode == 401 {
        if err := c.refresh(ctx); err != nil {
            return nil, fmt.Errorf("session expired, run `linktheca auth login`: %w", err)
        }
        resp, err = c.httpClient.Do(c.withBearer(req))
    }
    return resp, err
}
```

### Handling `RADAR_ENABLED=false` on the server

On receiving a 501 with `{"error":"radar_disabled"}`, `linktheca radar *` prints:

```
Error: Radar is disabled on this server.
Set LINKTHECA_RADAR_ENABLED=true on the backend to enable.
```

### Out of phase 3a's scope

- The `linktheca library *` commands (save/list/...) — a separate phase.
- CSV/OPML bulk import — a separate phase.
- Shell completions (`linktheca completion bash`) — later.
- A TUI or interactive mode — not planned.

## 8. Testing

Four levels plus an optional smoke level.

### Level 1: Unit

| Component | File | Dependencies |
|---|---|---|
| `embeddings.TEIClient` parsing | `embeddings/client_test.go` | `httptest.Server` |
| `embeddings.FakeEmbedder` | `embeddings/fake_test.go` | stdlib |
| `radar.Service` | `radar/service_test.go` | a mock `radar.Store`, `FakeEmbedder` |
| `radar.http` validation | `radar/http_test.go` | `httptest`, a mock Service |
| CLI session | `cmd/linktheca/internal/cli/session/session_test.go` | a tmp dir |
| CLI output | `cmd/linktheca/internal/cli/output/format_test.go` | stdlib |
| CLI API client auto-refresh | `cmd/linktheca/internal/cli/apiclient/client_test.go` | `httptest.Server` |

The principle: `radar.Store` is an interface in `service.go`, implemented by the
real `store.go`. Mocks are written by hand, without gomock or mockery.

### Level 2: Store (a real Postgres)

Through `internal/testing/testdb`:

| What | File |
|---|---|
| Every `radar.Store` method | `radar/store_test.go` |
| A pgvector roundtrip | `radar/store_test.go::TestEmbeddingRoundtrip` |
| HNSW cosine queries | `radar/store_test.go::TestMatchQuery` |
| The crawler with synthetic RSS | `radar/crawler/crawler_test.go` |

Before phase 3a: verify that `testdb`'s schema-per-test works correctly with the
`vector` type (the extension is global and the schema sees it).

### Level 3: HTTP integration

| What | File |
|---|---|
| `POST /radar/topics`, `POST /radar/feeds`, `POST /radar/subscriptions` | `radar/integration_test.go` |
| The full cycle (Crawl → Embed → Match) through the real jobs | `radar/jobs/jobs_test.go` |

River in tests: we use either `river.NewTestClient` / synchronous execution of
the workers, or direct `worker.Work(ctx, job)` calls. The exact pattern is
settled during implementation.

### Level 4: Smoke (`-tags=smoke`, not in normal CI)

| What | File |
|---|---|
| TEI returns 1024 dimensions | `embeddings/client_smoke_test.go` |
| The pipeline against a real TEI and the HN RSS | `radar/jobs/smoke_test.go` |

Run with:
```
make smoke-radar
```

### Level 5: CLI end to end

`cmd/linktheca/integration_test.go`:
1. `go build -o /tmp/linktheca-test ./cmd/linktheca`.
2. A Postgres testcontainer plus `linktheca-server` in a goroutine with a
   `FakeEmbedder`.
3. CLI commands called in sequence through `exec.Command`.
4. Checking session.json, the SQL rows, and the exit codes.

### The `RADAR_ENABLED=false` test

`server/server_test.go::TestRadarDisabled`:
- The server starts with `RadarEnabled: false`.
- `POST /radar/topics` → 501 with `{"error":"radar_disabled"}`.
- River holds no radar workers.
- The TEI client in DI stayed nil.

### What we do not test

- Real RSS in normal CI (synthetic XML only).
- Precise whitespace in the CLI output (only the fields).
- Performance and benchmarks.
- Chaos tests.

### CI

- The existing `backend` job picks up the new packages automatically.
- No smoke job is added in phase 3a.

## 9. Config, compose, running it

### Phase 3a env variables

```
# TEI
LINKTHECA_TEI_URL=http://tei:8080              # in compose; http://localhost:8081 for local dev
LINKTHECA_TEI_TIMEOUT=30s
LINKTHECA_EMBEDDING_DIM=1024

# Radar
LINKTHECA_RADAR_ENABLED=true                   # default true
LINKTHECA_RADAR_SCHEDULER_INTERVAL=5m
LINKTHECA_RADAR_MAX_WORKERS=5

# CLI-only
LINKTHECA_URL=http://localhost:8080
LINKTHECA_CONFIG_DIR=~/.config/linktheca
```

Every existing phase 1/2 variable is kept.

### The `linktheca-server` startup sequence

```
1. Parse env → Config
2. Validate the config (JWT secret length, DSN format, RADAR_ENABLED parsing)
3. Connect to Postgres (pgxpool.New)
4. Goose migrations UP (embedded migrations 001-010)
5. River migrations UP
6. If RADAR_ENABLED:
     - Init the TEI client
     - An optional Embed("ping") self-check, with a warning logged on a mismatch
     - Register the radar workers (ScheduleCrawls, CrawlFeed, EmbedFinding, MatchFinding)
7. Initialize the River client and start it
8. The HTTP server listens on HTTP_ADDR
```

Shutdown: SIGTERM/SIGINT → HTTP Shutdown → river Stop → pool Close.

### Makefile additions

```make
dev-server:           ## run backend with hot reload
	LINKTHECA_TEI_URL=http://localhost:8081 \
	LINKTHECA_DB_DSN=postgres://linktheca:linktheca@localhost:5432/linktheca?sslmode=disable \
	LINKTHECA_JWT_SECRET=dev-only-secret-that-is-at-least-32-bytes-long \
	air

cli-build:            ## build linktheca CLI
	go build -o ./bin/linktheca ./cmd/linktheca

server-build:         ## build linktheca-server
	go build -o ./bin/linktheca-server ./cmd/linktheca-server

smoke-radar:          ## smoke tests with real TEI (slow)
	go test -tags=smoke -timeout=10m -count=1 ./internal/radar/... ./internal/core/embeddings/...
```

### The dev workflow after phase 3a

```bash
# Terminal 1: infrastructure
docker compose -f compose.dev.yaml up -d     # Postgres + TEI

# Terminal 2: the backend
make dev-server

# Terminal 3: the CLI
./bin/linktheca auth register --email=me@example.com --password=... --display-name="Me"
./bin/linktheca radar feed add --url=https://news.ycombinator.com/rss
./bin/linktheca radar topic add --name="AI" --description="..."
./bin/linktheca radar subscribe --feed-id=1

# Checking the pipeline
psql ... -c "SELECT count(*) FROM radar_findings"
psql ... -c "SELECT * FROM radar_topic_matches ORDER BY matched_at DESC LIMIT 10"
```

### Renaming `cmd/linktheca` → `cmd/linktheca-server`

In one commit at the start of phase 3a:
- `git mv cmd/linktheca cmd/linktheca-server`
- Update `Dockerfile`, `Makefile`, `.github/workflows/ci.yml`, the README, and
  any `go run ./cmd/linktheca` in the docs.
- The package name stays `package main` and the logic does not change.

## 10. Risks

| Risk | Mitigation |
|---|---|
| bge-m3 does not fit in memory (~2.5–3 GB RSS) | The README documents a 4 GB RAM minimum for running with Radar. The `bge-m3-small` (384 dim) alternative is on the table only if the pain is confirmed (changing the model means changing a migration) |
| The first TEI start is slow (downloading the model) | `start_period: 120s` in the healthcheck, and the `tei-data` volume caches it between reboots |
| River periodic jobs duplicate under HA | Phase 3a runs a single instance; HA is out of scope. River's leader election will provide the singleton later |
| An RSS feed serves broken XML | gofeed returns an error, `last_error` is saved, and River retries |
| A TEI timeout on a large text | `description` ≤ 2000 chars, and title+summary from RSS is usually under 1000 |
| pgvector/HNSW growth at 100k findings | HNSW scales. Monitoring the size is phase 3b |
| The CLI's session.json holds secrets on a shared machine | Permissions 0600, documented. OAuth-level security is out of scope |
| `RADAR_ENABLED=true` but TEI is unreachable | The `POST /radar/topics` handler returns 503. The topic is left without an embedding and MatchJob skips it. The user gets a clear message |

### Accepted trade-offs

- No deduplication of similar findings across feeds.
- No rate limiting on TEI beyond `MaxWorkers`.
- SchedulerJob is capped at 100 feeds per cycle.

## 11. Optional Radar

One env flag is the only documented contract:

```
LINKTHECA_RADAR_ENABLED=true    # default true
```

### Backend behaviour

**When `true`:**
- The TEI client is initialized and runs a self-check Embed.
- The Radar River workers are registered.
- The `POST /radar/*` routes are mounted behind `RequireUser` / `RequireAdmin`.
- SchedulerJob ticks.

**When `false`:**
- The TEI client is not created (`embeddings.Client` = nil in DI).
- The Radar workers are not registered with River.
- The `/radar/*` routes return `501 Not Implemented` with
  `{"error":"radar_disabled"}` through a catch-all handler on the prefix.
- Migrations 006–010 **always run** — the tables are created empty. That is cheap
  and it makes switching Radar on later painless, with no data loss.

### The CLI

A 501 with `radar_disabled` produces:

```
Error: Radar is disabled on this server.
Set LINKTHECA_RADAR_ENABLED=true on the backend to enable.
```

### Resource requirements (for the README)

| Configuration | Minimum RAM | Recommended RAM |
|---|---|---|
| `LINKTHECA_RADAR_ENABLED=true` (default) | 4 GB | 8 GB |
| `LINKTHECA_RADAR_ENABLED=false` | 512 MB | 1 GB |

### What we do not document

Managing the TEI container through compose (scale, removal, profiles) — the
operator decides that. The documentation focuses on the backend flag as the
feature's only contract.

### Future LLM features

When they arrive (summarization, Q&A, auto-tagging) they get their own
independent flags (`LINKTHECA_SUMMARIZATION_ENABLED` and the like) with external
API keys. They are not tied to `RADAR_ENABLED` and they do not change the local
server's resource needs.

## Next steps

After this document is approved — move to the `superpowers:writing-plans` skill
to create a step-by-step implementation plan.
