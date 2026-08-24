# Linktheca: MVP architecture design

**Date:** 2026-04-10
**Status:** approved, ready for writing-plans

## Context

Linktheca is an open-source, self-hosted read-it-later service with topic-based
news monitoring built on semantic search over local embeddings.

The modules:
- **Library** (read-it-later) — links the user saves by hand.
- **Radar** (news monitoring) — relevant articles found automatically for
  subscribed topics.
- **Core** — shared services: auth, content parsing, embeddings, the database.

**The key UX decision (from CLAUDE.md):** Library and Radar do not mix. What the
user saves and what the crawler finds live in different sections.

## Top-level constraints and decisions

Taken during brainstorming:

| # | Question | Decision |
|---|---|---|
| 1 | User model | Multi-user self-hosted, with registration closed off through env |
| 2 | Scale of a single instance | Medium: ~100 users, hundreds of thousands of articles. Eventually a shared public instance (but we are not optimizing for that now) |
| 3 | MVP scope | Library in full plus a minimal Radar (RSS only, embeddings, no notifications) |
| 4 | Mobile | First-class from day one: a clean JSON API, Bearer tokens |
| 5 | Deployment | Docker Compose |
| 6 | Developer experience | New to Go — prefer idiomatic, simple solutions |

## Top-level architecture

A monolith with clean boundaries between modules. One Go module, one binary, and
the boundaries between `library`/`radar`/`core` enforced through `internal/`
packages and the direction of imports.

### Docker Compose shape

```
docker compose
├── web          nginx serving the built React static files
├── backend      the Go binary: the HTTP API plus workers (River) in one process
├── postgres     Postgres 18 + pgvector (data, vectors, the River queue, FTS)
└── ollama       the embedding server, the bge-m3 model
```

The mobile client lives in a **separate repository** (`linktheca-mobile`) and
does not touch this one.

## 1. Repository layout

```
linktheca/
├── CLAUDE.md
├── README.md
├── docker-compose.yml
├── docker-compose.dev.yml
├── docker-compose.prod.yml
├── Makefile
│
├── cmd/
│   └── linktheca/main.go           # the one backend binary
│
├── internal/
│   ├── core/
│   │   ├── config/                 # loading config from env
│   │   ├── db/                     # the pgx pool, migrations through embed
│   │   ├── httpx/                  # middleware, response helpers
│   │   ├── auth/                   # JWT, argon2id, middleware
│   │   ├── content/                # article parsing (readability)
│   │   ├── embeddings/             # the Ollama client
│   │   └── logging/                # slog
│   ├── library/                    # the read-it-later module
│   │   ├── types.go
│   │   ├── store.go                # SQL through pgx
│   │   ├── service.go              # business logic
│   │   └── http.go                 # HTTP handlers
│   ├── radar/                      # news monitoring
│   │   ├── types.go
│   │   ├── store.go
│   │   ├── service.go
│   │   ├── http.go
│   │   ├── crawler/                # the RSS/Atom fetcher (gofeed)
│   │   └── jobs/                   # River workers
│   └── server/
│       └── server.go               # DI, assembles the chi router
│
├── migrations/                     # SQL migrations (goose)
│
├── web/                            # the React SPA — a separate npm project
│   ├── package.json
│   ├── vite.config.ts
│   ├── tailwind.config.ts
│   └── src/
│       ├── main.tsx
│       ├── App.tsx
│       ├── routes/
│       ├── features/
│       │   ├── auth/
│       │   ├── library/
│       │   └── radar/
│       ├── shared/
│       │   ├── api/
│       │   ├── ui/                 # shadcn/ui components
│       │   ├── layout/
│       │   ├── hooks/
│       │   └── lib/
│       └── styles/
│
├── prototype/index.html            # the existing visual prototype, left alone
│
├── docs/superpowers/specs/         # spec documents
│
├── go.mod                          # one Go module for the whole backend
├── go.sum
└── .github/workflows/              # CI
```

**Key decisions:**

- **One `go.mod` for the whole backend.** Not a workspace, not multi-module. The
  boundaries between modules are enforced through `internal/` and the direction
  of imports.
- **The `library` and `radar` modules never import each other directly.** They
  communicate only through interfaces declared at the point of use (see
  "Communication between modules").
- **One binary (`cmd/linktheca`).** The API and the workers in one process. If
  they ever need splitting, we add a second `cmd/worker` later.
- **`web/` is a separate npm project in the same repo.** A monorepo, because API
  and UI change together, share a release cycle, and share one CI.
- **The frontend is built into an nginx image** during the Docker build; in
  compose it is a separate `web` container.

## 2. Backend: libraries and internals

### Libraries

| Concern | Library | Role |
|---|---|---|
| HTTP routing | `go-chi/chi/v5` | A thin layer over `net/http` |
| Postgres driver | `jackc/pgx/v5` | Pool, types, native pgvector support |
| Vectors in pgx | `pgvector/pgvector-go` | The `vector` type for pgx |
| SQL | raw SQL through pgx | No ORM, no codegen in the MVP. If the SQL gets verbose we introduce `sqlc` later as its own decision |
| Migrations | `pressly/goose` | `.sql` files, run from embed at startup |
| Job queue | `riverqueue/river` | Postgres-backed, for crawl/embed/match |
| Validation | `go-playground/validator/v10` | Struct tags |
| JWT | `golang-jwt/jwt/v5` | Access tokens |
| Password hashing | `golang.org/x/crypto/argon2` | argon2id with a PHC string |
| Content parsing | `go-shiori/go-readability` | Extracting article text |
| RSS/Atom | `mmcdole/gofeed` | The feed parser |
| Logs | `log/slog` (stdlib) | Structured logging |
| Config | `caarlos0/env` | Env vars → struct |
| Rate limiting | `go-chi/httprate` | An in-memory token bucket for /login and /register |
| CORS | `go-chi/cors` | CORS middleware |
| Tests: containers | `testcontainers-go` | A real Postgres for integration tests |
| Tests: assertions | `stretchr/testify` | `assert`, `require` |
| Auto-reload (dev) | `air-verse/air` | Rebuilds the binary on change |

**Deliberately not used:** GORM/ent (an ORM hides the SQL), Gin/Echo/Fiber (more
magic), viper (env is enough), Redis/asynq (River covers it through Postgres).

### Every module's pattern: `store → service → http`

- **`store`** knows only about the database. Its methods return domain types. No
  HTTP, no business logic.
- **`service`** is the business logic. It combines the store with external
  dependencies (`content.Extractor`, `embeddings.Client`). It knows nothing about
  HTTP. It takes the store through an interface (for tests).
- **`http`** holds thin handlers. They parse the request, call the service, and
  serialize the response. No logic.

### Communication between modules

The principle: **`library` and `radar` never import each other directly.**

The only point of contact in the MVP is "move a finding from Radar into
Library". The solution: `radar` declares the interface it needs, and `server.go`
passes in the implementation from `library` at assembly time:

```go
// in internal/radar
type LibrarySaver interface {
    Save(ctx context.Context, userID int64, url string) error
}
```

The standard Go pattern: interfaces are declared where they are used.

## 3. Data model (Postgres + pgvector)

The `pgvector` extension is created in the first migration:
`CREATE EXTENSION IF NOT EXISTS vector;`

### The core block

```sql
-- System users
CREATE TABLE users (
    id            BIGSERIAL PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,          -- argon2id PHC string
    display_name  TEXT NOT NULL,
    is_admin      BOOLEAN NOT NULL DEFAULT FALSE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Refresh tokens (access is a stateless JWT; refresh is server-side and revocable)
CREATE TABLE refresh_tokens (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,      -- a hash of the refresh token; the original is never stored
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    user_agent TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON refresh_tokens (user_id) WHERE revoked_at IS NULL;

-- A shared cache of parsed content: one URL is parsed at most once, and both
-- library and radar reference it without duplicating the text.
CREATE TABLE article_contents (
    id                   BIGSERIAL PRIMARY KEY,
    url                  TEXT NOT NULL UNIQUE,
    canonical_url        TEXT,
    title                TEXT,
    byline               TEXT,
    excerpt              TEXT,
    text                 TEXT,            -- plain text
    html                 TEXT,            -- cleaned HTML for the reader view
    lang                 TEXT,
    reading_time_seconds INT,
    fetched_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    fetch_error          TEXT
);
CREATE INDEX ON article_contents USING GIN (
    to_tsvector('simple', coalesce(title,'') || ' ' || coalesce(text,''))
);
```

**Note:** registration is controlled by the `LINKTHECA_REGISTRATION_ENABLED` env
variable; there is no separate settings table.

### The library block

```sql
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
CREATE INDEX ON library_items (user_id, saved_at DESC);
CREATE INDEX ON library_items (user_id, state);
```

**There are no tags in the MVP.** We add them later in their own migration.

### The radar block

```sql
-- The topics a user subscribes to
CREATE TABLE radar_topics (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL,           -- the text that gets embedded
    embedding       vector(1024),            -- the bge-m3 dimensionality
    match_threshold REAL NOT NULL DEFAULT 0.75,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX ON radar_topics (user_id) WHERE is_active;

-- Feeds are global: one source is parsed once for every subscribed user
CREATE TABLE radar_feeds (
    id                     BIGSERIAL PRIMARY KEY,
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
CREATE INDEX ON radar_feeds (is_active, last_fetched_at);

-- Subscriptions: even without a UI in the MVP, we create the table up front
CREATE TABLE radar_feed_subscriptions (
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    feed_id    BIGINT NOT NULL REFERENCES radar_feeds(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, feed_id)
);

-- Raw findings, before semantic filtering
CREATE TABLE radar_findings (
    id            BIGSERIAL PRIMARY KEY,
    feed_id       BIGINT NOT NULL REFERENCES radar_feeds(id) ON DELETE CASCADE,
    content_id    BIGINT REFERENCES article_contents(id),  -- NULL until it is opened
    external_id   TEXT,                                    -- the id/guid from the RSS
    url           TEXT NOT NULL,
    title         TEXT,
    summary       TEXT,
    embedding     vector(1024),                            -- bge-m3
    published_at  TIMESTAMPTZ,
    discovered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (feed_id, external_id)
);
CREATE INDEX ON radar_findings (discovered_at DESC);
CREATE INDEX ON radar_findings USING hnsw (embedding vector_cosine_ops);

-- Matching: this is what the user sees in the Radar UI
CREATE TABLE radar_topic_matches (
    id         BIGSERIAL PRIMARY KEY,
    topic_id   BIGINT NOT NULL REFERENCES radar_topics(id) ON DELETE CASCADE,
    finding_id BIGINT NOT NULL REFERENCES radar_findings(id) ON DELETE CASCADE,
    similarity REAL NOT NULL,
    state      TEXT NOT NULL DEFAULT 'new'
               CHECK (state IN ('new', 'seen')),
    matched_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (topic_id, finding_id)
);
CREATE INDEX ON radar_topic_matches (topic_id, state, matched_at DESC);
```

**Key properties of the schema:**

- **`article_contents` lives in core** and is shared between library and radar.
  One URL is parsed once.
- **`radar_findings` are global**, `radar_topic_matches` are per-user. One feed
  is parsed once for every subscribed user.
- **"Saved to Library"** is derived from a JOIN of `radar_findings` →
  `article_contents` ← `library_items`, with no separate state field.
- **vector(1024)** for the `bge-m3` model (justified in the "Embeddings"
  section).
- **An HNSW index** on `radar_findings.embedding` — fast kNN search with cosine
  distance.
- **Migrations run automatically when the backend starts** (embedded through
  `//go:embed migrations/*.sql`).

## 4. Auth flow

### The principle

One mechanism for web and mobile. All authorization goes through the
`Authorization: Bearer <access>` header. No cookies.

### The token model

| Token | Type | Lifetime | Where it lives |
|---|---|---|---|
| Access | JWT HS256 (stateless) | 15 minutes | Client: memory. Server: nowhere |
| Refresh | A random string (32 bytes base64) | 30 days | Client: secure storage. Server: a hash in `refresh_tokens` |

**Rationale:**
- Access is a JWT so every request can be validated without hitting the
  database.
- Refresh is an opaque token in the database so it can be revoked. A JWT cannot
  be revoked.
- A short access token minimizes the damage from a leak; refresh rotation detects
  reuse.

### Access JWT claims

```json
{
  "sub": "42",
  "iat": 1712750000,
  "exp": 1712750900,
  "is_admin": false
}
```

The claims are minimal — only what the middleware needs. Email, display_name and
the like come from `/auth/me`.

### Endpoints

```
POST /auth/register  { email, password, display_name }
POST /auth/login     { email, password }
POST /auth/refresh   { refresh_token }                   (rotation: the old one is revoked)
POST /auth/logout    + Bearer  { refresh_token }
GET  /auth/me        + Bearer
```

- `/register` returns 403 when `LINKTHECA_REGISTRATION_ENABLED=false`.
- `/register` and `/login` are rate-limited through `httprate` (10 attempts per
  IP per 10 minutes).
- `/refresh` always rotates: it issues a new refresh token and marks the old one
  `revoked_at`.

### Middleware

- `auth.RequireUser` — parses the Bearer token, validates the JWT, and puts
  `userID` and `is_admin` into the context.
- `auth.RequireAdmin` — always composed on top of `RequireUser`; 403 when
  `!is_admin`.

Route groups in `server.go`:
- `/auth/*` — public, with protected routes inside
- `/library/*`, `/radar/*` — everything behind `RequireUser`
- `/admin/*` — behind `RequireUser + RequireAdmin`

### The first user is the admin

At registration: if the database has no users yet, the new user is created with
`is_admin=true`. No seed scripts, no setup wizard.

### Security

| What | How |
|---|---|
| Password hashing | argon2id, parameters `time=2, memory=64MB, threads=2, keyLen=32, salt=16 bytes`. Stored in the standard PHC string format |
| Minimum password length | 10 characters |
| Rate limiting | `httprate` in memory, 10/10min on `/login` and `/register` |
| HTTPS | The operator's responsibility (Caddy/Traefik outside compose) |
| CORS | The `LINKTHECA_CORS_ORIGINS` env var, empty by default |

**Not in the MVP:** email verification, password reset, 2FA, OAuth, an audit
log, an admin UI for managing users.

## 5. The Radar pipeline

Four kinds of work, each its own River job:

```
Scheduler → CrawlFeedJob → EmbedJob → MatchJob
```

### Scheduler (a periodic River job)

Every 5 minutes:
```sql
SELECT * FROM radar_feeds
WHERE is_active
  AND (last_fetched_at IS NULL
       OR last_fetched_at + fetch_interval_seconds * interval '1 second' < now())
LIMIT 100
```
For each one, `river.Insert(CrawlFeedJob{FeedID: id})`.

### CrawlFeedJob

1. HTTP GET the feed with `If-None-Match` (etag) and `If-Modified-Since`. On a
   304, only update `last_fetched_at`.
2. Parse it with `gofeed`.
3. For each item:
   `INSERT INTO radar_findings (content_id=NULL, embedding=NULL) ON CONFLICT (feed_id, external_id) DO NOTHING`.
4. For each new insert, `river.Insert(EmbedJob{FindingID: id})`.
5. `UPDATE radar_feeds SET last_fetched_at, etag, last_modified, last_error=NULL`.

**Important:** this step does NOT parse full content. Only what the feed itself
supplies — title, summary, published_at, url.

River retry: exponential backoff, up to 25 attempts over ~24 hours. On a failed
job, `last_error` is saved into `radar_feeds`.

### EmbedJob

1. Load the finding. If `embedding IS NOT NULL`, exit (idempotency).
2. `text := title + "\n" + summary`.
3. `vec := ollama.Embed(text)` — an HTTP call to Ollama, the `bge-m3` model,
   1024 dim.
4. `UPDATE radar_findings SET embedding=$vec`.
5. `river.Insert(MatchJob{FindingID: id})`.

**We embed title+summary, not the full text.** It is cheaper, and it is enough
for a short news item. Fetching the full content happens on demand (see step 6).

### MatchJob

One SQL statement for everything:
```sql
INSERT INTO radar_topic_matches (topic_id, finding_id, similarity, state)
SELECT rt.id, $1, 1 - (rt.embedding <=> $2), 'new'
FROM radar_topics rt
JOIN radar_feed_subscriptions rfs ON rfs.user_id = rt.user_id
WHERE rfs.feed_id = $3
  AND rt.is_active
  AND 1 - (rt.embedding <=> $2) >= rt.match_threshold
ON CONFLICT (topic_id, finding_id) DO NOTHING;
```

`<=>` is pgvector's cosine distance. The user sets `match_threshold` in the UI
(range 0..1).

### Displaying it in the UI

`GET /radar/feed`:
```sql
SELECT f.*, m.similarity, m.state, t.name AS topic_name,
       EXISTS(SELECT 1 FROM library_items li
              WHERE li.user_id = $userID AND li.content_id = f.content_id)
         AS in_library
FROM radar_topic_matches m
JOIN radar_findings f ON f.id = m.finding_id
JOIN radar_topics t ON t.id = m.topic_id
WHERE t.user_id = $userID AND m.state = 'new'
ORDER BY m.matched_at DESC
LIMIT 50;
```

### Fetching full content on demand

`GET /radar/findings/:id/read`:
1. If `finding.content_id IS NULL` — `content.Extract(url)` → INSERT into
   `article_contents` → UPDATE the finding.
2. Return the content.

The same mechanism serves Library: when a URL is saved, `content.Extract` is
called synchronously in the handler. If that URL is already in
`article_contents`, it is reused.

### Edge cases

- **A topic's embedding changes** (the user edited the description): we
  regenerate `radar_topics.embedding`. Existing matches are not recomputed —
  they are "historical". New findings match against the new embedding.
- **A feed breaks**: River retries, `last_error` is saved, and `last_fetched_at`
  is updated anyway (so we do not hammer it). The admin sees broken feeds in the
  UI (phase 2).

### Out of MVP

Notifications (of any kind), HTML scraping of non-RSS sources, feed
autodiscovery on sites, reranking/cross-encoders, deduplicating similar
findings, tuning the threshold from user feedback.

## 6. Embeddings

> **Update 2026-05-06:** the current justification of the model and inference
> server lives in `2026-05-06-embedding-model-decision.md`. The inference server
> was replaced with TEI (see
> `2026-04-22-phase-3a-radar-pipeline-design.md`). The section below is kept as a
> historical snapshot from April 2026.

**Server:** Ollama — a separate service in compose, with an HTTP API.

**Why Ollama (not TEI, not in-process):**
- We will certainly want LLM features later (auto-summarizing articles, help with
  topic descriptions).
- Ollama covers both embeddings and LLMs in one service — adding a new model is
  an `ollama pull`.
- TEI would be a better fit for embeddings only, but it does not extend to LLMs.
- fastembed-go saves a container but drags C dependencies into the Go binary.

**Model:** `bge-m3`, 1024 dim, 2.3 GB on disk.

**Why bge-m3:** multilingual, and good at Russian and English simultaneously.
Disk size is not a constraint in a self-hosted setup.

**A critical constraint:** the embedding dimensionality cannot change without
recomputing every vector and rebuilding the HNSW indexes. Choosing the model is
a long-term decision.

**The Go client:** a simple HTTP call,
`POST http://ollama:11434/api/embeddings { model: "bge-m3", prompt: text }`,
parse the `float32` array, wrap it in a `pgvector.Vector`. The
`embeddings.Client.Embed(ctx, text) ([]float32, error)` interface is swapped for
a deterministic mock in tests.

## 7. Frontend

### Stack

| Concern | Choice |
|---|---|
| Build | Vite |
| Language | TypeScript (strict) |
| UI framework | React 19 |
| Routing | React Router v7 (data mode) |
| Server state | TanStack Query v5 |
| Client state | Zustand |
| Styling | Tailwind CSS v4 |
| Components | shadcn/ui + Radix primitives (copied into the project) |
| Fonts | Fraunces + Newsreader + JetBrains Mono (from the prototype) through @fontsource |
| Forms | React Hook Form + Zod |
| Icons | lucide-react |
| Tests: unit | Vitest + React Testing Library + MSW |
| Tests: e2e | Playwright (phase 2, after the MVP) |

**Not used:** Next.js/Remix (we need an SPA), Redux (Zustand is enough),
MUI/Chakra (they dictate the design and would override the prototype's editorial
aesthetic), Emotion/styled-components (Tailwind).

### The `web/src/` structure

```
main.tsx, App.tsx

routes/                             # the React Router tree
    index, login, register
    library/ (index, $id)
    radar/ (index, topics, findings.$id)
    settings/

features/                           # feature-based, mirroring the backend
    auth/     (api, hooks, store, components)
    library/  (api, hooks, components)
    radar/    (api, hooks, components)

shared/
    api/      (client, errors, types — the types are generated from OpenAPI)
    ui/       (shadcn/ui components)
    layout/   (AppShell, ReaderLayout)
    hooks/
    lib/

styles/globals.css
```

The principle is **feature-based, not type-based**. Everything to do with Library
lives in `features/library`.

### The API client

- **The OpenAPI spec is maintained by hand** (`openapi.yaml` at the root or in
  `web/`).
- `openapi-typescript` generates `web/src/shared/api/types.ts` through the
  `npm run gen:api` dev script.
- A thin fetch wrapper in `shared/api/client.ts`: it adds the Bearer token and
  handles 401 (an automatic refresh plus retry, and a logout if that fails).
- The functions in `features/*/api.ts` wrap `apiFetch` with types from the
  generated `types.ts`.
- TanStack Query hooks in `features/*/hooks.ts` sit on top of the API functions.

### The dev setup

Three terminals:
1. `docker compose -f docker-compose.dev.yml up` — Postgres plus Ollama.
2. `air` or `make dev-backend` — the backend with auto-reload.
3. `cd web && npm run dev` — the Vite dev server with HMR.

Vite proxies `/api/*` to `localhost:8080`, so CORS is not needed.

### Deployment

A separate `web` container:
- A multi-stage Dockerfile: `node:22` for the build → `nginx:alpine` for serving.
- Nginx serves `dist/` and proxies `/api/*` to `backend:8080`.

## 8. Testing

### Backend

| Level | Tools | Coverage |
|---|---|---|
| Unit | `testing` + `testify/assert` | `service.go` with a mock store through interfaces |
| Integration (store) | `testcontainers-go` + `pgx` | SQL against a real Postgres with pgvector |
| HTTP | `httptest` + testcontainers | The full path HTTP → service → store → database |

**The principle: no database mocks.** pgvector, HNSW, `<=>`, and FTS are
Postgres-specific and cannot be mocked.

**The `testdb.New(t)` pattern:**
- Once per test run, a `sync.Once` brings up a Postgres testcontainer.
- Per `t.Run`, `CREATE SCHEMA test_xyz`, migrate it, and return a pool with a
  `search_path`.
- `t.Cleanup` drops the schema.

**Ollama in tests:** we never call the real service. `embeddings.Client` is an
interface, replaced in tests by a deterministic mock (hash → vector, for
instance). A smoke test against a real Ollama lives behind the `-tags=smoke`
build tag, outside the main run.

### Frontend

| Level | Tools |
|---|---|
| Component | Vitest + React Testing Library + MSW |
| E2E (phase 2) | Playwright |

Component tests cover forms (validation, submit, errors), hooks
(loading/error/success), and complex stateful components.

**Not tested:** visual regression, storybook, every button.

E2E of the critical paths is deferred to phase 2, after the MVP.

### CI

GitHub Actions:
- `backend`: `go vet` plus `go test ./... -race`
- `frontend`: lint plus type-check plus vitest plus build
- `e2e`: after backend and frontend, on PRs into main only (phase 2)

## 9. Developer experience

### Makefile

```
make dev          # compose dev + air + vite (all in one)
make test         # go test + npm test
make test-e2e     # playwright
make migrate      # goose up
make lint         # go vet + npm lint
make build        # the full production build
```

### Config

Through env vars, parsed by `caarlos0/env` into a `Config` struct. The full list
of MVP settings:

```
# Server
LINKTHECA_HTTP_ADDR=:8080
LINKTHECA_LOG_LEVEL=info              # debug | info | warn | error
LINKTHECA_LOG_FORMAT=text              # text | json

# Database
LINKTHECA_DB_DSN=postgres://linktheca:linktheca@postgres:5432/linktheca?sslmode=disable

# Auth
LINKTHECA_JWT_SECRET=<32+ bytes random>
LINKTHECA_JWT_ACCESS_TTL=15m
LINKTHECA_JWT_REFRESH_TTL=720h         # 30 days
LINKTHECA_REGISTRATION_ENABLED=true    # controls POST /auth/register

# CORS (empty in dev, since Vite proxies through /api)
LINKTHECA_CORS_ORIGINS=

# Ollama / embeddings
LINKTHECA_OLLAMA_URL=http://ollama:11434
LINKTHECA_EMBEDDING_MODEL=bge-m3

# Radar
LINKTHECA_RADAR_SCHEDULER_INTERVAL=5m  # how often we scan radar_feeds
```

Files:
```
.env                  # shared defaults, committed (no secrets)
.env.local            # gitignored, local secrets
docker-compose.yml    # base
docker-compose.dev.yml
docker-compose.prod.yml
```

### Observability in the MVP

**Structured logs** through `log/slog`, and nothing else:
- Format: JSON in production, text in dev.
- The request ID from the middleware is threaded into every log line of the
  request.

**Not in the MVP:** Prometheus metrics, OpenTelemetry tracing, Sentry,
dashboards.

## Explicitly out of the MVP

Collected from every section:

- Tags in Library.
- A `reading` state in Library (only `unread`, `read`, `archived`).
- `dismissed` and `saved` states on Radar matches (only `new`, `seen`).
- Email verification, password reset, 2FA, OAuth/SSO, an audit log, an admin UI.
- Notifications (email/push/in-UI).
- HTML scraping of non-RSS sources.
- Feed autodiscovery from a site URL.
- Reranking/cross-encoders in Radar.
- Deduplicating similar findings.
- LLM features (summarization and so on) — Ollama is ready for them, but the
  features themselves are not built.
- E2E tests (Playwright).
- Metrics, tracing, dashboards.
- The mobile client (a separate repository, a separate project).

## Next steps

After this document is approved — move to the `superpowers:writing-plans` skill
to create a step-by-step implementation plan.
