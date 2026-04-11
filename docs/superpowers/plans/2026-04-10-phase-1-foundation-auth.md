# Phase 1: Foundation + Auth — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the foundational backend for Linktheca — a working Go HTTP service with Postgres, migrations, configuration, logging, CI, and a complete authentication flow (register, login, refresh rotation, logout, me). End state: you can register the first admin user, log in, call `/auth/me`, and refresh tokens — all via curl or Postman.

**Architecture:** Single Go module with `cmd/linktheca` as the only binary. `internal/core/*` holds cross-cutting primitives (config, db, logging, httpx, auth primitives). `internal/auth/` is the first feature module (parallel to future library/radar), following the `store → service → http` pattern. Postgres 18 + pgvector runs in Docker Compose. Integration tests use testcontainers-go for real Postgres.

**Tech Stack:** Go 1.26+, `go-chi/chi/v5`, `jackc/pgx/v5`, `pressly/goose/v3`, `caarlos0/env/v11`, `golang-jwt/jwt/v5`, `alexedwards/argon2id`, `go-chi/httprate`, `go-chi/cors`, `testcontainers-go`, `stretchr/testify`. Frontend is NOT part of this phase.

**Module path:** `github.com/ismd/linktheca` (change in `go.mod` if you prefer a different path; all imports will follow).

**Working directory:** `/home/ismd/coding/linktheca`

---

## File structure created by this phase

```
linktheca/
├── .github/workflows/ci.yml
├── .gitignore
├── Makefile
├── compose.dev.yaml
├── go.mod
├── go.sum
│
├── cmd/linktheca/main.go
│
├── migrations/
│   ├── 001_init.sql
│   ├── 002_users.sql
│   └── 003_refresh_tokens.sql
│
├── internal/
│   ├── core/
│   │   ├── config/config.go
│   │   ├── config/config_test.go
│   │   ├── db/pool.go
│   │   ├── db/migrate.go
│   │   ├── logging/slog.go
│   │   ├── httpx/middleware.go
│   │   ├── httpx/responses.go
│   │   └── auth/
│   │       ├── types.go              # Claims, ctxKey, UserID helper
│   │       ├── password.go           # argon2id Hash/Verify
│   │       ├── password_test.go
│   │       ├── jwt.go                # Issue/Parse access JWT
│   │       ├── jwt_test.go
│   │       ├── refresh.go            # Generate raw token + hash
│   │       ├── refresh_test.go
│   │       └── middleware.go         # RequireUser, RequireAdmin
│   ├── auth/                         # auth FEATURE module
│   │   ├── types.go                  # User, DTOs (RegisterRequest, ...)
│   │   ├── store.go                  # UsersStore, RefreshStore
│   │   ├── store_test.go             # integration with testdb
│   │   ├── service.go                # Register/Login/Refresh/Logout/Me
│   │   ├── service_test.go           # unit with mock store
│   │   ├── http.go                   # HTTP handlers
│   │   └── http_test.go              # HTTP-level integration
│   ├── server/
│   │   └── server.go                 # DI, chi router assembly
│   └── testing/testdb/testdb.go      # testcontainers helper
```

**Not in this phase:** library module, radar module, embeddings, frontend, OpenAPI spec generation, production Dockerfile (dev compose only), tracing/metrics.

---

## Conventions for every task

- **TDD everywhere.** Every non-trivial function gets a failing test first, then minimal implementation, then verification.
- **Commit after each task.** Small, focused commits make review easy and rollback cheap.
- **Run from the repo root** (`/home/ismd/coding/linktheca`) unless otherwise noted.
- **Do not use `git add .`** — stage files explicitly to avoid accidentally including secrets or build artifacts.
- **Commit messages** follow `<type>: <subject>` (e.g., `feat: add users table migration`). No `Co-Authored-By` lines unless you want them.
- **Go version:** Go 1.26 or later. Check with `go version`.

---

## Part A: Bootstrap

### Task 1: Initialize Go module and .gitignore

**Files:**
- Create: `go.mod` (via `go mod init`)
- Create: `.gitignore`

- [ ] **Step 1: Initialize Go module**

Run from repo root:
```bash
go mod init github.com/ismd/linktheca
```

Expected: creates `go.mod` with content like:
```
module github.com/ismd/linktheca

go 1.26
```

- [ ] **Step 2: Create .gitignore**

Create `.gitignore` with:
```gitignore
# Go
/bin/
/tmp/
*.test
*.out
coverage.out

# Env and secrets
.env.local
.env.*.local

# IDE
.idea/
.vscode/
*.swp

# OS
.DS_Store

# Air
tmp/
```

- [ ] **Step 3: Verify no extra files**

Run:
```bash
git status
```

Expected: shows `.gitignore` and `go.mod` as new files, nothing else.

- [ ] **Step 4: Commit**

```bash
git add .gitignore go.mod
git commit -m "chore: initialize Go module and gitignore"
```

---

### Task 2: Docker Compose for dev (Postgres + pgvector)

**Files:**
- Create: `compose.dev.yaml`

- [ ] **Step 1: Create compose.dev.yaml**

Create `compose.dev.yaml`:
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

volumes:
  linktheca_pg_data:
```

- [ ] **Step 2: Start Postgres and verify it's alive**

```bash
docker compose -f compose.dev.yaml up -d
docker compose -f compose.dev.yaml ps
```

Expected: `postgres` service running, health: `healthy` (may take ~10 seconds to become healthy).

- [ ] **Step 3: Verify pgvector extension is available**

```bash
docker compose -f compose.dev.yaml exec postgres psql -U linktheca -d linktheca -c "CREATE EXTENSION IF NOT EXISTS vector; SELECT extversion FROM pg_extension WHERE extname='vector';"
```

Expected: prints the vector extension version (e.g., `0.7.4`).

- [ ] **Step 4: Stop Postgres (we'll restart when we need it)**

```bash
docker compose -f compose.dev.yaml down
```

- [ ] **Step 5: Commit**

```bash
git add compose.dev.yaml
git commit -m "chore: add dev docker compose with postgres + pgvector"
```

---

### Task 3: Makefile with basic targets

**Files:**
- Create: `Makefile`

- [ ] **Step 1: Create Makefile**

Create `Makefile`:
```makefile
.PHONY: help dev-db dev-db-down dev-db-logs run test test-unit test-integration lint tidy build clean

help:
	@echo "Available targets:"
	@echo "  dev-db          - start Postgres in Docker"
	@echo "  dev-db-down     - stop Postgres"
	@echo "  dev-db-logs     - follow Postgres logs"
	@echo "  run             - run backend locally"
	@echo "  test            - run all Go tests with race detector"
	@echo "  test-unit       - run only unit tests (skip integration)"
	@echo "  test-integration - run only integration tests"
	@echo "  lint            - go vet"
	@echo "  tidy            - go mod tidy"
	@echo "  build           - build the backend binary"
	@echo "  clean           - remove build artifacts"

dev-db:
	docker compose -f compose.dev.yaml up -d

dev-db-down:
	docker compose -f compose.dev.yaml down

dev-db-logs:
	docker compose -f compose.dev.yaml logs -f postgres

run:
	go run ./cmd/linktheca

test:
	go test ./... -race -count=1

test-unit:
	go test ./... -race -count=1 -short

test-integration:
	go test ./... -race -count=1 -run Integration

lint:
	go vet ./...

tidy:
	go mod tidy

build:
	mkdir -p bin
	go build -o bin/linktheca ./cmd/linktheca

clean:
	rm -rf bin tmp
```

- [ ] **Step 2: Verify Makefile parses**

```bash
make help
```

Expected: prints the list of targets.

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "chore: add Makefile with common dev targets"
```

---

### Task 4: Config package with env loader (TDD)

**Files:**
- Create: `internal/core/config/config.go`
- Test: `internal/core/config/config_test.go`

- [ ] **Step 1: Add dependencies**

```bash
go get github.com/caarlos0/env/v11
go get github.com/stretchr/testify/require
```

Expected: downloads are added to `go.sum`.

- [ ] **Step 2: Write the failing test**

Create `internal/core/config/config_test.go`:
```go
package config_test

import (
	"testing"
	"time"

	"github.com/ismd/linktheca/internal/core/config"
	"github.com/stretchr/testify/require"
)

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("LINKTHECA_HTTP_ADDR", ":9999")
	t.Setenv("LINKTHECA_DB_DSN", "postgres://u:p@h:5432/db")
	t.Setenv("LINKTHECA_JWT_SECRET", "testsecret-with-enough-bytes-to-pass-checks")
	t.Setenv("LINKTHECA_JWT_ACCESS_TTL", "5m")
	t.Setenv("LINKTHECA_JWT_REFRESH_TTL", "24h")
	t.Setenv("LINKTHECA_REGISTRATION_ENABLED", "false")
	t.Setenv("LINKTHECA_LOG_LEVEL", "debug")
	t.Setenv("LINKTHECA_LOG_FORMAT", "json")

	cfg, err := config.Load()
	require.NoError(t, err)

	require.Equal(t, ":9999", cfg.HTTPAddr)
	require.Equal(t, "postgres://u:p@h:5432/db", cfg.DBDSN)
	require.Equal(t, "testsecret-with-enough-bytes-to-pass-checks", cfg.JWTSecret)
	require.Equal(t, 5*time.Minute, cfg.JWTAccessTTL)
	require.Equal(t, 24*time.Hour, cfg.JWTRefreshTTL)
	require.False(t, cfg.RegistrationEnabled)
	require.Equal(t, "debug", cfg.LogLevel)
	require.Equal(t, "json", cfg.LogFormat)
}

func TestLoadRequiresJWTSecret(t *testing.T) {
	// Clear required var and make sure Load fails
	t.Setenv("LINKTHECA_JWT_SECRET", "")
	t.Setenv("LINKTHECA_DB_DSN", "postgres://localhost/x")

	_, err := config.Load()
	require.Error(t, err)
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("LINKTHECA_JWT_SECRET", "a-strong-secret-at-least-32-bytes-long")
	t.Setenv("LINKTHECA_DB_DSN", "postgres://localhost/x")

	cfg, err := config.Load()
	require.NoError(t, err)

	require.Equal(t, ":8080", cfg.HTTPAddr)
	require.Equal(t, 15*time.Minute, cfg.JWTAccessTTL)
	require.Equal(t, 720*time.Hour, cfg.JWTRefreshTTL)
	require.True(t, cfg.RegistrationEnabled)
	require.Equal(t, "info", cfg.LogLevel)
	require.Equal(t, "text", cfg.LogFormat)
}
```

- [ ] **Step 3: Run the test to verify it fails**

```bash
go test ./internal/core/config/...
```

Expected: FAIL — `package config` does not exist.

- [ ] **Step 4: Implement config.Load**

Create `internal/core/config/config.go`:
```go
package config

import (
	"errors"
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	HTTPAddr  string `env:"LINKTHECA_HTTP_ADDR" envDefault:":8080"`
	LogLevel  string `env:"LINKTHECA_LOG_LEVEL" envDefault:"info"`
	LogFormat string `env:"LINKTHECA_LOG_FORMAT" envDefault:"text"`

	DBDSN string `env:"LINKTHECA_DB_DSN,required"`

	JWTSecret     string        `env:"LINKTHECA_JWT_SECRET,required"`
	JWTAccessTTL  time.Duration `env:"LINKTHECA_JWT_ACCESS_TTL" envDefault:"15m"`
	JWTRefreshTTL time.Duration `env:"LINKTHECA_JWT_REFRESH_TTL" envDefault:"720h"`

	RegistrationEnabled bool `env:"LINKTHECA_REGISTRATION_ENABLED" envDefault:"true"`

	CORSOrigins []string `env:"LINKTHECA_CORS_ORIGINS" envSeparator:","`
}

// Load reads configuration from environment variables and validates required fields.
func Load() (*Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("parse env: %w", err)
	}
	if len(cfg.JWTSecret) < 32 {
		return nil, errors.New("LINKTHECA_JWT_SECRET must be at least 32 bytes")
	}
	return &cfg, nil
}
```

- [ ] **Step 5: Run tests to verify pass**

```bash
go test ./internal/core/config/... -v
```

Expected: `PASS` for all three tests.

- [ ] **Step 6: Commit**

```bash
go mod tidy
git add go.mod go.sum internal/core/config/
git commit -m "feat(config): env-based config loader with validation"
```

---

### Task 5: Structured logging package

**Files:**
- Create: `internal/core/logging/slog.go`

- [ ] **Step 1: Create logging package**

Create `internal/core/logging/slog.go`:
```go
package logging

import (
	"io"
	"log/slog"
	"strings"
)

// New creates a slog.Logger based on format ("text" or "json") and level
// ("debug", "info", "warn", "error"). Unknown values fall back to sensible defaults.
func New(out io.Writer, format, level string) *slog.Logger {
	opts := &slog.HandlerOptions{
		Level: parseLevel(level),
	}

	var handler slog.Handler
	switch strings.ToLower(format) {
	case "json":
		handler = slog.NewJSONHandler(out, opts)
	default:
		handler = slog.NewTextHandler(out, opts)
	}

	return slog.New(handler)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/core/logging/...
```

Expected: no output, no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/core/logging/
git commit -m "feat(logging): slog-based logger factory"
```

---

### Task 6: Minimal main.go with health endpoint

**Files:**
- Create: `cmd/linktheca/main.go`

- [ ] **Step 1: Add chi dependency**

```bash
go get github.com/go-chi/chi/v5
```

- [ ] **Step 2: Create main.go**

Create `cmd/linktheca/main.go`:
```go
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ismd/linktheca/internal/core/config"
	"github.com/ismd/linktheca/internal/core/logging"
)

func main() {
	if err := run(); err != nil {
		// logger may not be up yet; use stderr directly
		slog.Error("startup failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := logging.New(os.Stdout, cfg.LogFormat, cfg.LogLevel)
	slog.SetDefault(logger)

	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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
	return nil
}
```

- [ ] **Step 3: Build and run manually**

```bash
go mod tidy
LINKTHECA_DB_DSN="postgres://linktheca:linktheca@localhost:5432/linktheca?sslmode=disable" \
LINKTHECA_JWT_SECRET="dev-only-secret-that-is-at-least-32-bytes-long" \
go run ./cmd/linktheca &
sleep 1
curl -s http://localhost:8080/healthz
kill %1
wait 2>/dev/null
```

Expected: prints `ok`, then server shuts down cleanly.

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum cmd/linktheca/
git commit -m "feat(cmd): main with chi router, config, logging, graceful shutdown"
```

---

## Part B: Database and migrations

### Task 7: First migration — pgvector extension

**Files:**
- Create: `migrations/001_init.sql`

- [ ] **Step 1: Create migration file**

Create `migrations/001_init.sql`:
```sql
-- +goose Up
CREATE EXTENSION IF NOT EXISTS vector;

-- +goose Down
DROP EXTENSION IF EXISTS vector;
```

- [ ] **Step 2: Commit**

```bash
git add migrations/001_init.sql
git commit -m "feat(db): initial migration with pgvector extension"
```

---

### Task 8: pgx connection pool

**Files:**
- Create: `internal/core/db/pool.go`

- [ ] **Step 1: Add pgx dependency**

```bash
go get github.com/jackc/pgx/v5
go get github.com/jackc/pgx/v5/pgxpool
```

- [ ] **Step 2: Create pool.go**

Create `internal/core/db/pool.go`:
```go
package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool creates a pgxpool.Pool with sensible defaults and verifies the connection.
func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}
	cfg.MaxConns = 20
	cfg.MinConns = 2
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.MaxConnLifetime = 1 * time.Hour

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("new pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}
```

- [ ] **Step 3: Verify it compiles**

```bash
go build ./internal/core/db/...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
go mod tidy
git add go.mod go.sum internal/core/db/
git commit -m "feat(db): pgx pool factory with ping verification"
```

---

### Task 9: Goose migrations embedded in binary

**Files:**
- Create: `internal/core/db/migrate.go`
- Modify: `cmd/linktheca/main.go` (wire migrations into startup)

- [ ] **Step 1: Add goose dependency**

```bash
go get github.com/pressly/goose/v3
```

- [ ] **Step 2: Create migrate.go with embedded FS**

Create `internal/core/db/migrate.go`:
```go
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

//go:embed ../../../migrations/*.sql
var migrationsFS embed.FS

// Migrate runs all pending migrations on the given pool.
// Uses goose over a *sql.DB adapter (pgx-stdlib).
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}

	connCfg := pool.Config().ConnConfig
	sqlDB := stdlib.OpenDB(*connCfg)
	defer sqlDB.Close()

	if err := goose.UpContext(ctx, sqlDB, "../../../migrations"); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}
```

**Note on the embed path:** `//go:embed` paths are relative to the package file. From `internal/core/db/migrate.go`, the migrations directory is at `../../../migrations`. Go's embed **does not support `..` in the path** — it only walks downward. We need to move or symlink.

- [ ] **Step 3: Fix the embed path — use a top-level loader**

Replace the file at `internal/core/db/migrate.go` — we'll use a different approach: embed from an `embeds.go` file located at the module root so the path walks downward.

Actually, the cleanest fix is to embed the migrations from the `db` package directly. We do this by **copying** the migrations path into the db package via embed. Since embed doesn't allow `..`, we restructure: put the `embed` directive in a file that lives above `migrations/`.

Replace `internal/core/db/migrate.go` with this version that receives the filesystem from the caller:

```go
package db

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3"
)

// Migrate runs all pending migrations using the provided filesystem.
// The caller is responsible for embedding the migrations directory.
func Migrate(ctx context.Context, pool *pgxpool.Pool, migrations fs.FS, dir string) error {
	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}

	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()

	if err := goose.UpContext(ctx, sqlDB, dir); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}
```

**Note:** `stdlib.OpenDBFromPool` may not exist on all pgx versions. Fall back to opening a separate `sql.DB` via `stdlib.OpenDB(*pool.Config().ConnConfig)` if the compile fails:

```go
sqlDB := stdlib.OpenDB(*pool.Config().ConnConfig)
```

Use whichever compiles. Both create an independent `sql.DB` handle from the pool's config.

- [ ] **Step 4: Create the embeds file at the module root**

Create `embeds.go` at the repo root (`/home/ismd/coding/linktheca/embeds.go`):

```go
package linktheca

import "embed"

// MigrationsFS embeds all SQL migrations so they are shipped with the binary.
//
//go:embed migrations/*.sql
var MigrationsFS embed.FS
```

- [ ] **Step 5: Verify compilation**

```bash
go mod tidy
go build ./...
```

Expected: no errors. (If `OpenDBFromPool` does not exist, swap to `stdlib.OpenDB(*pool.Config().ConnConfig)` in `migrate.go`.)

- [ ] **Step 6: Wire migrations into main.go**

Modify `cmd/linktheca/main.go`. Add imports:

```go
	linktheca "github.com/ismd/linktheca"
	"github.com/ismd/linktheca/internal/core/db"
```

Inside `run()`, after `logger := ...` and before setting up the chi router, add:

```go
	pool, err := db.NewPool(ctx, cfg.DBDSN)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool, linktheca.MigrationsFS, "migrations"); err != nil {
		return err
	}
	logger.Info("migrations applied")
```

Note: `ctx` in this scope is the signal-cancelled context from `signal.NotifyContext`. Since migrations happen before the HTTP server starts, move the `signal.NotifyContext` call earlier, right after logger setup:

Final shape of `run()`:
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
	logger.Info("migrations applied")

	r := chi.NewRouter()
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

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
	return nil
}
```

- [ ] **Step 7: Run Postgres and start the backend to verify migrations apply**

```bash
make dev-db
sleep 3
LINKTHECA_DB_DSN="postgres://linktheca:linktheca@localhost:5432/linktheca?sslmode=disable" \
LINKTHECA_JWT_SECRET="dev-only-secret-that-is-at-least-32-bytes-long" \
go run ./cmd/linktheca &
sleep 2
curl -s http://localhost:8080/healthz
kill %1
wait 2>/dev/null
```

Expected: log lines include `migrations applied` and curl prints `ok`.

Verify extension was installed:
```bash
docker compose -f compose.dev.yaml exec postgres psql -U linktheca -d linktheca -c "\dx"
```

Expected: table includes `vector`.

- [ ] **Step 8: Commit**

```bash
git add go.mod go.sum embeds.go internal/core/db/migrate.go cmd/linktheca/main.go
git commit -m "feat(db): embedded goose migrations, run on startup"
```

---

## Part C: Test infrastructure

### Task 10: testdb helper with testcontainers

**Files:**
- Create: `internal/testing/testdb/testdb.go`

- [ ] **Step 1: Add testcontainers dependency**

```bash
go get github.com/testcontainers/testcontainers-go
go get github.com/testcontainers/testcontainers-go/modules/postgres
```

- [ ] **Step 2: Create testdb.go**

Create `internal/testing/testdb/testdb.go`:
```go
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"testing"
	"time"

	linktheca "github.com/ismd/linktheca"
	"github.com/ismd/linktheca/internal/core/db"
	"github.com/jackc/pgx/v5/pgxpool"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	once      sync.Once
	sharedDSN string
	initErr   error
)

// New returns a pgxpool.Pool connected to a freshly-created schema
// inside a shared Postgres test container. The schema is dropped on t.Cleanup.
func New(t *testing.T) *pgxpool.Pool {
	t.Helper()

	once.Do(func() {
		sharedDSN, initErr = startContainer()
	})
	if initErr != nil {
		t.Fatalf("start test container: %v", initErr)
	}

	ctx := context.Background()

	// 1. Connect to the shared DB as superuser (the default user from container) to create a schema.
	adminPool, err := db.NewPool(ctx, sharedDSN)
	if err != nil {
		t.Fatalf("connect admin: %v", err)
	}
	defer adminPool.Close()

	schema := "test_" + randHex(8)
	if _, err := adminPool.Exec(ctx, fmt.Sprintf(`CREATE SCHEMA %q`, schema)); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	// 2. Build a DSN with search_path set to the new schema.
	scopedDSN := sharedDSN + "&search_path=" + schema

	pool, err := db.NewPool(ctx, scopedDSN)
	if err != nil {
		t.Fatalf("connect scoped: %v", err)
	}

	// 3. Run migrations inside this schema.
	if err := db.Migrate(ctx, pool, linktheca.MigrationsFS, "migrations"); err != nil {
		pool.Close()
		t.Fatalf("migrate: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		admin, err := db.NewPool(cleanupCtx, sharedDSN)
		if err != nil {
			t.Logf("cleanup admin connect: %v", err)
			return
		}
		defer admin.Close()
		_, _ = admin.Exec(cleanupCtx, fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema))
	})

	return pool
}

func startContainer() (string, error) {
	ctx := context.Background()
	container, err := tcpostgres.Run(ctx,
		"pgvector/pgvector:0.8.2-pg18-trixie",
		tcpostgres.WithDatabase("linktheca_test"),
		tcpostgres.WithUsername("linktheca"),
		tcpostgres.WithPassword("linktheca"),
		tcpostgres.BasicWaitStrategies(),
		tcpostgres.WithSQLDriver("pgx"),
	)
	if err != nil {
		return "", fmt.Errorf("start container: %w", err)
	}

	waitStrategy := wait.ForLog("database system is ready to accept connections").WithOccurrence(2)
	if err := waitStrategy.WaitUntilReady(ctx, container); err != nil {
		return "", fmt.Errorf("wait: %w", err)
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		return "", fmt.Errorf("connection string: %w", err)
	}
	return dsn, nil
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
```

**Note:** The `tcpostgres.Run` signature varies by testcontainers-go version. If the exact shape above fails to compile, adjust to the signatures listed in your installed version (`go doc github.com/testcontainers/testcontainers-go/modules/postgres`). The key requirement is: start a `pgvector/pgvector:0.8.2-pg18-trixie` container, wait until ready, return the DSN.

- [ ] **Step 3: Write a smoke test that uses testdb**

Create `internal/testing/testdb/testdb_test.go`:
```go
package testdb_test

import (
	"context"
	"testing"

	"github.com/ismd/linktheca/internal/testing/testdb"
	"github.com/stretchr/testify/require"
)

func TestIntegrationTestdbConnects(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	pool := testdb.New(t)

	var got int
	err := pool.QueryRow(context.Background(), "SELECT 1").Scan(&got)
	require.NoError(t, err)
	require.Equal(t, 1, got)
}

func TestIntegrationMigrationsRan(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	pool := testdb.New(t)

	var hasVector bool
	err := pool.QueryRow(context.Background(),
		`SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'vector')`).Scan(&hasVector)
	require.NoError(t, err)
	require.True(t, hasVector)
}
```

- [ ] **Step 4: Run the smoke test**

```bash
go mod tidy
go test ./internal/testing/testdb/... -v -count=1
```

Expected: both tests pass (first run may take 20-30 seconds while it pulls the pgvector image).

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum internal/testing/testdb/
git commit -m "test: testdb helper using testcontainers with per-test schemas"
```

---

## Part D: Users table and store

### Task 11: Users table migration

**Files:**
- Create: `migrations/002_users.sql`

- [ ] **Step 1: Create migration**

Create `migrations/002_users.sql`:
```sql
-- +goose Up
CREATE TABLE users (
    id            BIGSERIAL PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    display_name  TEXT NOT NULL,
    is_admin      BOOLEAN NOT NULL DEFAULT FALSE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE users;
```

- [ ] **Step 2: Commit**

```bash
git add migrations/002_users.sql
git commit -m "feat(db): users table migration"
```

---

### Task 12: Auth module types

**Files:**
- Create: `internal/auth/types.go`

- [ ] **Step 1: Create types**

Create `internal/auth/types.go`:
```go
package auth

import "time"

// User is the domain representation of a registered user.
type User struct {
	ID           int64     `json:"id"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"display_name"`
	IsAdmin      bool      `json:"is_admin"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	PasswordHash string    `json:"-"` // never serialize
}

// RegisterRequest is the payload for POST /auth/register.
type RegisterRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

// LoginRequest is the payload for POST /auth/login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// RefreshRequest is the payload for POST /auth/refresh and /auth/logout.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// TokenPair is what /auth/register, /auth/login, /auth/refresh return.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

// AuthResponse is returned on register/login (token pair + user).
type AuthResponse struct {
	User   User      `json:"user"`
	Tokens TokenPair `json:"tokens"`
}

// RefreshToken is the domain representation of a stored refresh token record.
type RefreshToken struct {
	ID        int64
	UserID    int64
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	UserAgent string
	CreatedAt time.Time
}
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/auth/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/auth/
git commit -m "feat(auth): domain types (User, TokenPair, RefreshToken)"
```

---

### Task 13: Users store with CreateUser, GetByEmail, CountUsers (TDD)

**Files:**
- Create: `internal/auth/store.go`
- Test: `internal/auth/store_test.go`

- [ ] **Step 1: Write failing integration tests**

Create `internal/auth/store_test.go`:
```go
package auth_test

import (
	"context"
	"testing"

	"github.com/ismd/linktheca/internal/auth"
	"github.com/ismd/linktheca/internal/testing/testdb"
	"github.com/stretchr/testify/require"
)

func TestIntegrationUsersStoreCreateAndGet(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	pool := testdb.New(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	created, err := store.CreateUser(ctx, "alice@example.com", "hashed", "Alice", true)
	require.NoError(t, err)
	require.NotZero(t, created.ID)
	require.Equal(t, "alice@example.com", created.Email)
	require.True(t, created.IsAdmin)
	require.Equal(t, "hashed", created.PasswordHash)

	got, err := store.GetUserByEmail(ctx, "alice@example.com")
	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID)
	require.Equal(t, "hashed", got.PasswordHash)

	gotByID, err := store.GetUserByID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.Email, gotByID.Email)
}

func TestIntegrationUsersStoreCountUsers(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	pool := testdb.New(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	n, err := store.CountUsers(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(0), n)

	_, err = store.CreateUser(ctx, "a@example.com", "h", "A", false)
	require.NoError(t, err)
	_, err = store.CreateUser(ctx, "b@example.com", "h", "B", false)
	require.NoError(t, err)

	n, err = store.CountUsers(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(2), n)
}

func TestIntegrationUsersStoreDuplicateEmail(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	pool := testdb.New(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	_, err := store.CreateUser(ctx, "dup@example.com", "h", "Dup", false)
	require.NoError(t, err)

	_, err = store.CreateUser(ctx, "dup@example.com", "h", "Dup2", false)
	require.ErrorIs(t, err, auth.ErrEmailTaken)
}

func TestIntegrationUsersStoreGetUnknownEmail(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	pool := testdb.New(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	_, err := store.GetUserByEmail(ctx, "nobody@example.com")
	require.ErrorIs(t, err, auth.ErrNotFound)
}
```

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./internal/auth/... -v
```

Expected: FAIL — `auth.NewStore` does not exist.

- [ ] **Step 3: Implement the store**

Create `internal/auth/store.go`:
```go
package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when the requested row does not exist.
var ErrNotFound = errors.New("not found")

// ErrEmailTaken is returned when attempting to create a user with an existing email.
var ErrEmailTaken = errors.New("email already registered")

// Store provides persistence for users and refresh tokens.
type Store struct {
	db *pgxpool.Pool
}

// NewStore constructs a Store bound to the given pool.
func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

// CreateUser inserts a new user row and returns the created User.
func (s *Store) CreateUser(ctx context.Context, email, passwordHash, displayName string, isAdmin bool) (*User, error) {
	row := s.db.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, display_name, is_admin)
		VALUES ($1, $2, $3, $4)
		RETURNING id, email, password_hash, display_name, is_admin, created_at, updated_at
	`, email, passwordHash, displayName, isAdmin)

	u, err := scanUser(row)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrEmailTaken
		}
		return nil, fmt.Errorf("create user: %w", err)
	}
	return u, nil
}

// GetUserByEmail looks up a user by email address.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, email, password_hash, display_name, is_admin, created_at, updated_at
		FROM users
		WHERE email = $1
	`, email)
	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return u, nil
}

// GetUserByID looks up a user by primary key.
func (s *Store) GetUserByID(ctx context.Context, id int64) (*User, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, email, password_hash, display_name, is_admin, created_at, updated_at
		FROM users
		WHERE id = $1
	`, id)
	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return u, nil
}

// CountUsers returns the total number of user rows.
func (s *Store) CountUsers(ctx context.Context) (int64, error) {
	var n int64
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return n, nil
}

func scanUser(row pgx.Row) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.DisplayName, &u.IsAdmin, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}
```

- [ ] **Step 4: Run tests to verify pass**

```bash
go test ./internal/auth/... -v -count=1
```

Expected: all four integration tests pass.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum internal/auth/store.go internal/auth/store_test.go
git commit -m "feat(auth): users store with Create/GetByEmail/GetByID/Count"
```

---

## Part E: Password hashing

### Task 14: argon2id password hash (TDD)

**Files:**
- Create: `internal/core/auth/password.go`
- Test: `internal/core/auth/password_test.go`

- [ ] **Step 1: Add argon2id dependency**

```bash
go get github.com/alexedwards/argon2id
```

- [ ] **Step 2: Write failing test**

Create `internal/core/auth/password_test.go`:
```go
package auth_test

import (
	"testing"

	"github.com/ismd/linktheca/internal/core/auth"
	"github.com/stretchr/testify/require"
)

func TestHashAndVerifyPassword(t *testing.T) {
	pw := "correct-horse-battery-staple"

	hash, err := auth.HashPassword(pw)
	require.NoError(t, err)
	require.NotEmpty(t, hash)
	require.Contains(t, hash, "$argon2id$")

	ok, err := auth.VerifyPassword(pw, hash)
	require.NoError(t, err)
	require.True(t, ok)
}

func TestVerifyPasswordWrongPassword(t *testing.T) {
	hash, err := auth.HashPassword("right-password")
	require.NoError(t, err)

	ok, err := auth.VerifyPassword("wrong-password", hash)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestVerifyPasswordInvalidHash(t *testing.T) {
	ok, err := auth.VerifyPassword("anything", "not-a-valid-hash")
	require.Error(t, err)
	require.False(t, ok)
}
```

- [ ] **Step 3: Run to confirm failure**

```bash
go test ./internal/core/auth/...
```

Expected: FAIL — `auth.HashPassword` does not exist.

- [ ] **Step 4: Implement**

Create `internal/core/auth/password.go`:
```go
package auth

import (
	"github.com/alexedwards/argon2id"
)

// argonParams matches OWASP 2024 recommendations for argon2id.
var argonParams = &argon2id.Params{
	Memory:      64 * 1024, // 64 MB
	Iterations:  2,
	Parallelism: 2,
	SaltLength:  16,
	KeyLength:   32,
}

// HashPassword returns a PHC-encoded argon2id hash for the given plaintext.
func HashPassword(plaintext string) (string, error) {
	return argon2id.CreateHash(plaintext, argonParams)
}

// VerifyPassword reports whether the plaintext matches the stored hash.
// Returns (false, nil) for a valid hash that does not match, and a non-nil error
// only if the hash itself is malformed.
func VerifyPassword(plaintext, hash string) (bool, error) {
	return argon2id.ComparePasswordAndHash(plaintext, hash)
}
```

- [ ] **Step 5: Run tests**

```bash
go mod tidy
go test ./internal/core/auth/... -v
```

Expected: all three tests pass.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/core/auth/
git commit -m "feat(auth): argon2id password hashing with OWASP-recommended params"
```

---

## Part F: JWT access tokens

### Task 15: JWT issue and parse (TDD)

**Files:**
- Create: `internal/core/auth/types.go`
- Create: `internal/core/auth/jwt.go`
- Test: `internal/core/auth/jwt_test.go`

- [ ] **Step 1: Add JWT dependency**

```bash
go get github.com/golang-jwt/jwt/v5
```

- [ ] **Step 2: Create core/auth/types.go**

Create `internal/core/auth/types.go`:
```go
package auth

import (
	"context"

	"github.com/golang-jwt/jwt/v5"
)

// Claims are the custom claims carried in an access JWT.
type Claims struct {
	IsAdmin bool `json:"is_admin"`
	jwt.RegisteredClaims
}

type ctxKey int

const (
	userIDKey ctxKey = iota
	isAdminKey
)

// WithUser puts the authenticated user identity on the context.
func WithUser(ctx context.Context, userID int64, isAdmin bool) context.Context {
	ctx = context.WithValue(ctx, userIDKey, userID)
	ctx = context.WithValue(ctx, isAdminKey, isAdmin)
	return ctx
}

// UserID returns the authenticated user id from the context.
// It panics if called without the auth middleware having run.
func UserID(ctx context.Context) int64 {
	v, ok := ctx.Value(userIDKey).(int64)
	if !ok {
		panic("auth: UserID called without auth middleware in chain")
	}
	return v
}

// IsAdmin returns whether the authenticated user is an admin.
// Returns false if not authenticated (safe default).
func IsAdmin(ctx context.Context) bool {
	v, _ := ctx.Value(isAdminKey).(bool)
	return v
}
```

- [ ] **Step 3: Write failing JWT test**

Create `internal/core/auth/jwt_test.go`:
```go
package auth_test

import (
	"testing"
	"time"

	"github.com/ismd/linktheca/internal/core/auth"
	"github.com/stretchr/testify/require"
)

const testSecret = "test-secret-at-least-32-bytes-long-for-hmac"

func TestIssueAndParseAccessToken(t *testing.T) {
	issuer := auth.NewJWTIssuer(testSecret, 15*time.Minute)

	token, err := issuer.Issue(42, true)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	claims, err := issuer.Parse(token)
	require.NoError(t, err)
	require.Equal(t, "42", claims.Subject)
	require.True(t, claims.IsAdmin)
}

func TestParseExpiredTokenFails(t *testing.T) {
	issuer := auth.NewJWTIssuer(testSecret, -1*time.Second) // already expired
	token, err := issuer.Issue(1, false)
	require.NoError(t, err)

	_, err = issuer.Parse(token)
	require.Error(t, err)
}

func TestParseWithWrongSecretFails(t *testing.T) {
	issuerA := auth.NewJWTIssuer(testSecret, 15*time.Minute)
	issuerB := auth.NewJWTIssuer("a-completely-different-secret-32bytes", 15*time.Minute)

	token, err := issuerA.Issue(1, false)
	require.NoError(t, err)

	_, err = issuerB.Parse(token)
	require.Error(t, err)
}
```

- [ ] **Step 4: Run to confirm failure**

```bash
go test ./internal/core/auth/...
```

Expected: FAIL — `auth.NewJWTIssuer` does not exist.

- [ ] **Step 5: Implement jwt.go**

Create `internal/core/auth/jwt.go`:
```go
package auth

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTIssuer issues and parses HS256 access tokens.
type JWTIssuer struct {
	secret []byte
	ttl    time.Duration
}

// NewJWTIssuer constructs an issuer bound to a secret and access TTL.
func NewJWTIssuer(secret string, ttl time.Duration) *JWTIssuer {
	return &JWTIssuer{secret: []byte(secret), ttl: ttl}
}

// Issue creates a signed access token for the given user id and admin flag.
func (j *JWTIssuer) Issue(userID int64, isAdmin bool) (string, error) {
	now := time.Now()
	claims := Claims{
		IsAdmin: isAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(userID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(j.ttl)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(j.secret)
	if err != nil {
		return "", fmt.Errorf("sign: %w", err)
	}
	return signed, nil
}

// Parse validates the token and returns the claims.
func (j *JWTIssuer) Parse(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return j.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid claims")
	}
	return claims, nil
}
```

- [ ] **Step 6: Run tests to verify pass**

```bash
go mod tidy
go test ./internal/core/auth/... -v
```

Expected: all password and jwt tests pass.

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum internal/core/auth/
git commit -m "feat(auth): JWT issuer/parser with HS256 and Claims type"
```

---

## Part G: Refresh tokens

### Task 16: refresh_tokens migration

**Files:**
- Create: `migrations/003_refresh_tokens.sql`

- [ ] **Step 1: Create migration**

Create `migrations/003_refresh_tokens.sql`:
```sql
-- +goose Up
CREATE TABLE refresh_tokens (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    user_agent TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX refresh_tokens_user_active_idx ON refresh_tokens (user_id) WHERE revoked_at IS NULL;

-- +goose Down
DROP TABLE refresh_tokens;
```

- [ ] **Step 2: Commit**

```bash
git add migrations/003_refresh_tokens.sql
git commit -m "feat(db): refresh_tokens table migration"
```

---

### Task 17: Refresh token generation and hashing (TDD)

**Files:**
- Create: `internal/core/auth/refresh.go`
- Test: `internal/core/auth/refresh_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/core/auth/refresh_test.go`:
```go
package auth_test

import (
	"testing"

	"github.com/ismd/linktheca/internal/core/auth"
	"github.com/stretchr/testify/require"
)

func TestGenerateRefreshToken(t *testing.T) {
	a, err := auth.GenerateRefreshToken()
	require.NoError(t, err)
	require.NotEmpty(t, a)

	b, err := auth.GenerateRefreshToken()
	require.NoError(t, err)
	require.NotEqual(t, a, b, "tokens must be unique per call")
}

func TestHashRefreshTokenIsDeterministic(t *testing.T) {
	token := "sample-refresh-token-value"
	h1 := auth.HashRefreshToken(token)
	h2 := auth.HashRefreshToken(token)
	require.Equal(t, h1, h2)
	require.NotEqual(t, token, h1, "hash must differ from plaintext")
}

func TestHashRefreshTokenDifferentInputs(t *testing.T) {
	h1 := auth.HashRefreshToken("token-a")
	h2 := auth.HashRefreshToken("token-b")
	require.NotEqual(t, h1, h2)
}
```

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./internal/core/auth/...
```

Expected: FAIL — `auth.GenerateRefreshToken` does not exist.

- [ ] **Step 3: Implement refresh.go**

Create `internal/core/auth/refresh.go`:
```go
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// GenerateRefreshToken returns a cryptographically random 32-byte token,
// base64-url encoded without padding. The caller stores only the hash.
func GenerateRefreshToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read rand: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// HashRefreshToken returns a hex-encoded SHA-256 of the token.
// This hash is what the server stores; a leaked DB never exposes live tokens.
func HashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/core/auth/... -v
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/core/auth/refresh.go internal/core/auth/refresh_test.go
git commit -m "feat(auth): refresh token generation and SHA-256 hashing"
```

---

### Task 18: Extend auth store with refresh token operations (TDD)

**Files:**
- Modify: `internal/auth/store.go`
- Modify: `internal/auth/store_test.go`

- [ ] **Step 1: Append failing tests**

Append to `internal/auth/store_test.go` (inside the `package auth_test`, below the existing tests):
```go
func TestIntegrationRefreshStoreCreateAndFind(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	pool := testdb.New(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	user, err := store.CreateUser(ctx, "rt@example.com", "h", "RT", false)
	require.NoError(t, err)

	hash := "abc123"
	exp := time.Now().Add(1 * time.Hour)
	rt, err := store.CreateRefreshToken(ctx, user.ID, hash, "test-ua", exp)
	require.NoError(t, err)
	require.NotZero(t, rt.ID)
	require.Equal(t, user.ID, rt.UserID)

	found, err := store.FindActiveRefreshToken(ctx, hash)
	require.NoError(t, err)
	require.Equal(t, rt.ID, found.ID)
	require.Nil(t, found.RevokedAt)
}

func TestIntegrationRefreshStoreRevoke(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	pool := testdb.New(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	user, err := store.CreateUser(ctx, "rv@example.com", "h", "RV", false)
	require.NoError(t, err)

	rt, err := store.CreateRefreshToken(ctx, user.ID, "h1", "ua", time.Now().Add(1*time.Hour))
	require.NoError(t, err)

	err = store.RevokeRefreshToken(ctx, rt.ID)
	require.NoError(t, err)

	_, err = store.FindActiveRefreshToken(ctx, "h1")
	require.ErrorIs(t, err, auth.ErrNotFound)
}

func TestIntegrationRefreshStoreFindExpired(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	pool := testdb.New(t)
	store := auth.NewStore(pool)
	ctx := context.Background()

	user, err := store.CreateUser(ctx, "exp@example.com", "h", "EXP", false)
	require.NoError(t, err)

	_, err = store.CreateRefreshToken(ctx, user.ID, "h2", "ua", time.Now().Add(-1*time.Hour))
	require.NoError(t, err)

	// Expired tokens must be treated as not found.
	_, err = store.FindActiveRefreshToken(ctx, "h2")
	require.ErrorIs(t, err, auth.ErrNotFound)
}
```

Also add `"time"` to the imports of `store_test.go` if not already present.

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./internal/auth/... -count=1
```

Expected: FAIL — `CreateRefreshToken` does not exist.

- [ ] **Step 3: Implement in store.go**

Append to `internal/auth/store.go`:
```go
import (
	// existing imports plus:
	"time"
)
```
(add `"time"` to the existing import block)

Then append methods to the `Store` type at the end of `store.go`:
```go
// CreateRefreshToken inserts a refresh token row and returns the created record.
func (s *Store) CreateRefreshToken(ctx context.Context, userID int64, tokenHash, userAgent string, expiresAt time.Time) (*RefreshToken, error) {
	var rt RefreshToken
	err := s.db.QueryRow(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, user_agent, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, token_hash, expires_at, revoked_at, user_agent, created_at
	`, userID, tokenHash, userAgent, expiresAt).Scan(
		&rt.ID, &rt.UserID, &rt.TokenHash, &rt.ExpiresAt, &rt.RevokedAt, &rt.UserAgent, &rt.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create refresh token: %w", err)
	}
	return &rt, nil
}

// FindActiveRefreshToken returns a refresh token that is non-revoked and non-expired.
// Returns ErrNotFound otherwise.
func (s *Store) FindActiveRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error) {
	var rt RefreshToken
	err := s.db.QueryRow(ctx, `
		SELECT id, user_id, token_hash, expires_at, revoked_at, user_agent, created_at
		FROM refresh_tokens
		WHERE token_hash = $1
		  AND revoked_at IS NULL
		  AND expires_at > now()
	`, tokenHash).Scan(
		&rt.ID, &rt.UserID, &rt.TokenHash, &rt.ExpiresAt, &rt.RevokedAt, &rt.UserAgent, &rt.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find refresh token: %w", err)
	}
	return &rt, nil
}

// RevokeRefreshToken marks the token as revoked. Idempotent: revoking an already-revoked
// token is not an error.
func (s *Store) RevokeRefreshToken(ctx context.Context, id int64) error {
	_, err := s.db.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL
	`, id)
	if err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/auth/... -v -count=1
```

Expected: all refresh token tests pass along with existing user tests.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/store.go internal/auth/store_test.go
git commit -m "feat(auth): refresh token store operations (create, find active, revoke)"
```

---

## Part H: Auth service (business logic)

### Task 19: Auth service with Register (TDD, unit with mock store)

**Files:**
- Create: `internal/auth/service.go`
- Create: `internal/auth/service_test.go`

- [ ] **Step 1: Write failing unit test for Register**

Create `internal/auth/service_test.go`:
```go
package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ismd/linktheca/internal/auth"
	coreauth "github.com/ismd/linktheca/internal/core/auth"
	"github.com/stretchr/testify/require"
)

type mockStore struct {
	users        map[string]*auth.User
	usersByID    map[int64]*auth.User
	refreshByHash map[string]*auth.RefreshToken
	nextUserID   int64
	nextRTID     int64
	countErr     error
}

func newMockStore() *mockStore {
	return &mockStore{
		users:         make(map[string]*auth.User),
		usersByID:     make(map[int64]*auth.User),
		refreshByHash: make(map[string]*auth.RefreshToken),
	}
}

func (m *mockStore) CreateUser(_ context.Context, email, hash, name string, isAdmin bool) (*auth.User, error) {
	if _, ok := m.users[email]; ok {
		return nil, auth.ErrEmailTaken
	}
	m.nextUserID++
	u := &auth.User{
		ID: m.nextUserID, Email: email, PasswordHash: hash, DisplayName: name, IsAdmin: isAdmin,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	m.users[email] = u
	m.usersByID[u.ID] = u
	return u, nil
}

func (m *mockStore) GetUserByEmail(_ context.Context, email string) (*auth.User, error) {
	u, ok := m.users[email]
	if !ok {
		return nil, auth.ErrNotFound
	}
	return u, nil
}

func (m *mockStore) GetUserByID(_ context.Context, id int64) (*auth.User, error) {
	u, ok := m.usersByID[id]
	if !ok {
		return nil, auth.ErrNotFound
	}
	return u, nil
}

func (m *mockStore) CountUsers(_ context.Context) (int64, error) {
	if m.countErr != nil {
		return 0, m.countErr
	}
	return int64(len(m.users)), nil
}

func (m *mockStore) CreateRefreshToken(_ context.Context, userID int64, hash, ua string, exp time.Time) (*auth.RefreshToken, error) {
	m.nextRTID++
	rt := &auth.RefreshToken{
		ID: m.nextRTID, UserID: userID, TokenHash: hash, ExpiresAt: exp, UserAgent: ua, CreatedAt: time.Now(),
	}
	m.refreshByHash[hash] = rt
	return rt, nil
}

func (m *mockStore) FindActiveRefreshToken(_ context.Context, hash string) (*auth.RefreshToken, error) {
	rt, ok := m.refreshByHash[hash]
	if !ok {
		return nil, auth.ErrNotFound
	}
	if rt.RevokedAt != nil {
		return nil, auth.ErrNotFound
	}
	if time.Now().After(rt.ExpiresAt) {
		return nil, auth.ErrNotFound
	}
	return rt, nil
}

func (m *mockStore) RevokeRefreshToken(_ context.Context, id int64) error {
	for _, rt := range m.refreshByHash {
		if rt.ID == id {
			now := time.Now()
			rt.RevokedAt = &now
			return nil
		}
	}
	return nil
}

func newTestService(t *testing.T, store auth.StoreAPI, registration bool) *auth.Service {
	t.Helper()
	issuer := coreauth.NewJWTIssuer("test-secret-at-least-32-bytes-long-for-hmac", 15*time.Minute)
	return auth.NewService(store, issuer, auth.ServiceConfig{
		RefreshTTL:          720 * time.Hour,
		RegistrationEnabled: registration,
	})
}

func TestRegisterFirstUserBecomesAdmin(t *testing.T) {
	store := newMockStore()
	svc := newTestService(t, store, true)

	resp, err := svc.Register(context.Background(), auth.RegisterRequest{
		Email: "root@example.com", Password: "a-strong-password", DisplayName: "Root",
	}, "ua")
	require.NoError(t, err)
	require.True(t, resp.User.IsAdmin)
	require.NotEmpty(t, resp.Tokens.AccessToken)
	require.NotEmpty(t, resp.Tokens.RefreshToken)
}

func TestRegisterSecondUserIsNotAdmin(t *testing.T) {
	store := newMockStore()
	svc := newTestService(t, store, true)

	_, err := svc.Register(context.Background(), auth.RegisterRequest{
		Email: "a@example.com", Password: "a-strong-password", DisplayName: "A",
	}, "ua")
	require.NoError(t, err)

	resp, err := svc.Register(context.Background(), auth.RegisterRequest{
		Email: "b@example.com", Password: "another-strong-password", DisplayName: "B",
	}, "ua")
	require.NoError(t, err)
	require.False(t, resp.User.IsAdmin)
}

func TestRegisterDisabledReturnsError(t *testing.T) {
	store := newMockStore()
	svc := newTestService(t, store, false)

	_, err := svc.Register(context.Background(), auth.RegisterRequest{
		Email: "a@example.com", Password: "a-strong-password", DisplayName: "A",
	}, "ua")
	require.ErrorIs(t, err, auth.ErrRegistrationDisabled)
}

func TestRegisterShortPassword(t *testing.T) {
	store := newMockStore()
	svc := newTestService(t, store, true)

	_, err := svc.Register(context.Background(), auth.RegisterRequest{
		Email: "a@example.com", Password: "short", DisplayName: "A",
	}, "ua")
	require.ErrorIs(t, err, auth.ErrWeakPassword)
}

func TestRegisterDuplicateEmail(t *testing.T) {
	store := newMockStore()
	svc := newTestService(t, store, true)

	_, err := svc.Register(context.Background(), auth.RegisterRequest{
		Email: "dup@example.com", Password: "a-strong-password", DisplayName: "Dup",
	}, "ua")
	require.NoError(t, err)

	_, err = svc.Register(context.Background(), auth.RegisterRequest{
		Email: "dup@example.com", Password: "another-password", DisplayName: "Dup2",
	}, "ua")
	require.ErrorIs(t, err, auth.ErrEmailTaken)
}

// Sanity: interface compile-time check.
var _ auth.StoreAPI = (*mockStore)(nil)
var _ = errors.New
```

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./internal/auth/... -short
```

Expected: FAIL — `auth.Service`, `auth.StoreAPI`, `auth.NewService`, `auth.ErrRegistrationDisabled`, `auth.ErrWeakPassword` do not exist.

- [ ] **Step 3: Implement service.go (Register only for now)**

Create `internal/auth/service.go`:
```go
package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	coreauth "github.com/ismd/linktheca/internal/core/auth"
)

var (
	ErrRegistrationDisabled = errors.New("registration disabled")
	ErrWeakPassword         = errors.New("password too short")
	ErrInvalidCredentials   = errors.New("invalid credentials")
)

// minPasswordLen is the enforced minimum password length.
const minPasswordLen = 10

// StoreAPI is the persistence surface the Service depends on.
// It is an interface so tests can substitute a mock.
type StoreAPI interface {
	CreateUser(ctx context.Context, email, passwordHash, displayName string, isAdmin bool) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetUserByID(ctx context.Context, id int64) (*User, error)
	CountUsers(ctx context.Context) (int64, error)
	CreateRefreshToken(ctx context.Context, userID int64, tokenHash, userAgent string, expiresAt time.Time) (*RefreshToken, error)
	FindActiveRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, id int64) error
}

// ServiceConfig collects runtime settings for the service.
type ServiceConfig struct {
	RefreshTTL          time.Duration
	RegistrationEnabled bool
}

// Service is the auth feature's business logic layer.
type Service struct {
	store  StoreAPI
	jwt    *coreauth.JWTIssuer
	cfg    ServiceConfig
}

// NewService wires dependencies into a Service.
func NewService(store StoreAPI, jwt *coreauth.JWTIssuer, cfg ServiceConfig) *Service {
	return &Service{store: store, jwt: jwt, cfg: cfg}
}

// Register creates a new user account and returns tokens plus the user record.
// The first user created on the instance is promoted to admin.
func (s *Service) Register(ctx context.Context, req RegisterRequest, userAgent string) (*AuthResponse, error) {
	if !s.cfg.RegistrationEnabled {
		return nil, ErrRegistrationDisabled
	}
	if len(req.Password) < minPasswordLen {
		return nil, ErrWeakPassword
	}

	count, err := s.store.CountUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("count users: %w", err)
	}
	isAdmin := count == 0

	hash, err := coreauth.HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	user, err := s.store.CreateUser(ctx, req.Email, hash, req.DisplayName, isAdmin)
	if err != nil {
		return nil, err
	}

	tokens, err := s.issueTokens(ctx, user, userAgent)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{User: *user, Tokens: *tokens}, nil
}

// issueTokens creates an access token and a refresh token record.
func (s *Service) issueTokens(ctx context.Context, user *User, userAgent string) (*TokenPair, error) {
	access, err := s.jwt.Issue(user.ID, user.IsAdmin)
	if err != nil {
		return nil, fmt.Errorf("issue access: %w", err)
	}

	refresh, err := coreauth.GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("generate refresh: %w", err)
	}

	_, err = s.store.CreateRefreshToken(ctx,
		user.ID,
		coreauth.HashRefreshToken(refresh),
		userAgent,
		time.Now().Add(s.cfg.RefreshTTL),
	)
	if err != nil {
		return nil, fmt.Errorf("persist refresh: %w", err)
	}

	return &TokenPair{AccessToken: access, RefreshToken: refresh}, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/auth/... -short -v
```

Expected: all Register tests pass. Integration tests are skipped due to `-short`.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/service.go internal/auth/service_test.go
git commit -m "feat(auth): service with Register (first user = admin, validations)"
```

---

### Task 20: Service Login method (TDD)

**Files:**
- Modify: `internal/auth/service.go`
- Modify: `internal/auth/service_test.go`

- [ ] **Step 1: Add failing tests**

Append to `internal/auth/service_test.go`:
```go
func TestLoginSuccess(t *testing.T) {
	store := newMockStore()
	svc := newTestService(t, store, true)

	_, err := svc.Register(context.Background(), auth.RegisterRequest{
		Email: "a@example.com", Password: "a-strong-password", DisplayName: "A",
	}, "ua")
	require.NoError(t, err)

	resp, err := svc.Login(context.Background(), auth.LoginRequest{
		Email: "a@example.com", Password: "a-strong-password",
	}, "ua")
	require.NoError(t, err)
	require.Equal(t, "a@example.com", resp.User.Email)
	require.NotEmpty(t, resp.Tokens.AccessToken)
	require.NotEmpty(t, resp.Tokens.RefreshToken)
}

func TestLoginWrongPassword(t *testing.T) {
	store := newMockStore()
	svc := newTestService(t, store, true)

	_, err := svc.Register(context.Background(), auth.RegisterRequest{
		Email: "a@example.com", Password: "a-strong-password", DisplayName: "A",
	}, "ua")
	require.NoError(t, err)

	_, err = svc.Login(context.Background(), auth.LoginRequest{
		Email: "a@example.com", Password: "wrong-password",
	}, "ua")
	require.ErrorIs(t, err, auth.ErrInvalidCredentials)
}

func TestLoginUnknownUser(t *testing.T) {
	store := newMockStore()
	svc := newTestService(t, store, true)

	_, err := svc.Login(context.Background(), auth.LoginRequest{
		Email: "nobody@example.com", Password: "a-strong-password",
	}, "ua")
	require.ErrorIs(t, err, auth.ErrInvalidCredentials)
}
```

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./internal/auth/... -short -run Login
```

Expected: FAIL — `svc.Login` not defined.

- [ ] **Step 3: Implement Login**

Append to `internal/auth/service.go`:
```go
// Login authenticates a user by email and password.
// Returns ErrInvalidCredentials for any authentication failure (do not leak
// whether the email exists).
func (s *Service) Login(ctx context.Context, req LoginRequest, userAgent string) (*AuthResponse, error) {
	user, err := s.store.GetUserByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	ok, err := coreauth.VerifyPassword(req.Password, user.PasswordHash)
	if err != nil || !ok {
		return nil, ErrInvalidCredentials
	}

	tokens, err := s.issueTokens(ctx, user, userAgent)
	if err != nil {
		return nil, err
	}
	return &AuthResponse{User: *user, Tokens: *tokens}, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/auth/... -short -v
```

Expected: all service tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/service.go internal/auth/service_test.go
git commit -m "feat(auth): service Login with uniform invalid-credentials error"
```

---

### Task 21: Service Refresh (with rotation) and Logout (TDD)

**Files:**
- Modify: `internal/auth/service.go`
- Modify: `internal/auth/service_test.go`

- [ ] **Step 1: Add failing tests**

Append to `internal/auth/service_test.go`:
```go
func TestRefreshRotatesToken(t *testing.T) {
	store := newMockStore()
	svc := newTestService(t, store, true)

	reg, err := svc.Register(context.Background(), auth.RegisterRequest{
		Email: "r@example.com", Password: "a-strong-password", DisplayName: "R",
	}, "ua")
	require.NoError(t, err)

	// Use the refresh token to obtain a new pair.
	resp, err := svc.Refresh(context.Background(), auth.RefreshRequest{
		RefreshToken: reg.Tokens.RefreshToken,
	}, "ua")
	require.NoError(t, err)
	require.NotEmpty(t, resp.Tokens.AccessToken)
	require.NotEqual(t, reg.Tokens.RefreshToken, resp.Tokens.RefreshToken, "rotation must produce a new refresh")

	// The old refresh token must now be invalid.
	_, err = svc.Refresh(context.Background(), auth.RefreshRequest{
		RefreshToken: reg.Tokens.RefreshToken,
	}, "ua")
	require.ErrorIs(t, err, auth.ErrInvalidCredentials)
}

func TestRefreshUnknownToken(t *testing.T) {
	store := newMockStore()
	svc := newTestService(t, store, true)

	_, err := svc.Refresh(context.Background(), auth.RefreshRequest{
		RefreshToken: "definitely-not-a-valid-token",
	}, "ua")
	require.ErrorIs(t, err, auth.ErrInvalidCredentials)
}

func TestLogoutRevokesRefreshToken(t *testing.T) {
	store := newMockStore()
	svc := newTestService(t, store, true)

	reg, err := svc.Register(context.Background(), auth.RegisterRequest{
		Email: "lo@example.com", Password: "a-strong-password", DisplayName: "LO",
	}, "ua")
	require.NoError(t, err)

	err = svc.Logout(context.Background(), auth.RefreshRequest{
		RefreshToken: reg.Tokens.RefreshToken,
	})
	require.NoError(t, err)

	_, err = svc.Refresh(context.Background(), auth.RefreshRequest{
		RefreshToken: reg.Tokens.RefreshToken,
	}, "ua")
	require.ErrorIs(t, err, auth.ErrInvalidCredentials)
}
```

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./internal/auth/... -short -run "Refresh|Logout"
```

Expected: FAIL — methods not defined.

- [ ] **Step 3: Implement Refresh and Logout**

Append to `internal/auth/service.go`:
```go
// Refresh exchanges a valid refresh token for a new access+refresh pair.
// Implements rotation: the old refresh is revoked immediately.
func (s *Service) Refresh(ctx context.Context, req RefreshRequest, userAgent string) (*AuthResponse, error) {
	hash := coreauth.HashRefreshToken(req.RefreshToken)
	existing, err := s.store.FindActiveRefreshToken(ctx, hash)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	user, err := s.store.GetUserByID(ctx, existing.UserID)
	if err != nil {
		return nil, fmt.Errorf("load user: %w", err)
	}

	// Revoke the old token first. Even if token issuance fails, the old one is dead.
	if err := s.store.RevokeRefreshToken(ctx, existing.ID); err != nil {
		return nil, fmt.Errorf("revoke previous: %w", err)
	}

	tokens, err := s.issueTokens(ctx, user, userAgent)
	if err != nil {
		return nil, err
	}
	return &AuthResponse{User: *user, Tokens: *tokens}, nil
}

// Logout revokes the provided refresh token. Idempotent: unknown or already-revoked
// tokens return nil (we do not leak whether the token existed).
func (s *Service) Logout(ctx context.Context, req RefreshRequest) error {
	hash := coreauth.HashRefreshToken(req.RefreshToken)
	existing, err := s.store.FindActiveRefreshToken(ctx, hash)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}
	return s.store.RevokeRefreshToken(ctx, existing.ID)
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/auth/... -short -v
```

Expected: all service tests (Register, Login, Refresh, Logout) pass.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/service.go internal/auth/service_test.go
git commit -m "feat(auth): service Refresh with rotation and Logout"
```

---

### Task 22: Service Me method and a GetByID helper (TDD)

**Files:**
- Modify: `internal/auth/service.go`
- Modify: `internal/auth/service_test.go`

- [ ] **Step 1: Add failing test**

Append to `internal/auth/service_test.go`:
```go
func TestMeReturnsCurrentUser(t *testing.T) {
	store := newMockStore()
	svc := newTestService(t, store, true)

	reg, err := svc.Register(context.Background(), auth.RegisterRequest{
		Email: "me@example.com", Password: "a-strong-password", DisplayName: "Me",
	}, "ua")
	require.NoError(t, err)

	got, err := svc.Me(context.Background(), reg.User.ID)
	require.NoError(t, err)
	require.Equal(t, "me@example.com", got.Email)
	require.Equal(t, "Me", got.DisplayName)
}
```

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./internal/auth/... -short -run TestMe
```

Expected: FAIL — `svc.Me` not defined.

- [ ] **Step 3: Implement Me**

Append to `internal/auth/service.go`:
```go
// Me returns the user identified by userID, typically extracted from the context
// by the RequireUser middleware.
func (s *Service) Me(ctx context.Context, userID int64) (*User, error) {
	return s.store.GetUserByID(ctx, userID)
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/auth/... -short -v
```

Expected: all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/service.go internal/auth/service_test.go
git commit -m "feat(auth): service Me method"
```

---

## Part I: HTTP layer

### Task 23: httpx middleware and response helpers

**Files:**
- Create: `internal/core/httpx/responses.go`
- Create: `internal/core/httpx/middleware.go`

- [ ] **Step 1: Create responses.go**

Create `internal/core/httpx/responses.go`:
```go
package httpx

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// WriteJSON writes status and encodes v as JSON. Errors are logged.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write json", "err", err)
	}
}

// ErrorBody is the shape of all error responses.
type ErrorBody struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// WriteError writes a JSON error response.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, ErrorBody{Error: code, Message: message})
}
```

- [ ] **Step 2: Create middleware.go**

Create `internal/core/httpx/middleware.go`:
```go
package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"
)

type ctxKey int

const requestIDKey ctxKey = iota

// RequestID attaches a unique ID to each request context and response header.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := newRequestID()
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID returns the request id previously attached by RequestID middleware,
// or empty string if none.
func GetRequestID(ctx context.Context) string {
	v, _ := ctx.Value(requestIDKey).(string)
	return v
}

// RequestLogger logs method, path, status and duration of each request.
func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rw, r)
			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rw.status,
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", GetRequestID(r.Context()),
			)
		})
	}
}

// Recover catches panics, logs them, and returns 500.
func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic", "err", rec, "path", r.URL.Path)
					WriteError(w, http.StatusInternalServerError, "internal", "")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func newRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
```

- [ ] **Step 3: Verify compilation**

```bash
go build ./internal/core/httpx/...
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/core/httpx/
git commit -m "feat(httpx): JSON responses, RequestID, RequestLogger, Recover middleware"
```

---

### Task 24: RequireUser and RequireAdmin middleware (TDD)

**Files:**
- Create: `internal/core/auth/middleware.go`
- Create: `internal/core/auth/middleware_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/core/auth/middleware_test.go`:
```go
package auth_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ismd/linktheca/internal/core/auth"
	"github.com/stretchr/testify/require"
)

func TestRequireUserAcceptsValidBearer(t *testing.T) {
	issuer := auth.NewJWTIssuer(testSecret, 15*time.Minute)
	token, err := issuer.Issue(42, false)
	require.NoError(t, err)

	var capturedUserID int64
	handler := auth.RequireUser(issuer)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUserID = auth.UserID(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/secret", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, int64(42), capturedUserID)
}

func TestRequireUserRejectsMissingHeader(t *testing.T) {
	issuer := auth.NewJWTIssuer(testSecret, 15*time.Minute)
	handler := auth.RequireUser(issuer)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not run")
	}))

	req := httptest.NewRequest(http.MethodGet, "/secret", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRequireUserRejectsBadBearer(t *testing.T) {
	issuer := auth.NewJWTIssuer(testSecret, 15*time.Minute)
	handler := auth.RequireUser(issuer)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not run")
	}))

	req := httptest.NewRequest(http.MethodGet, "/secret", nil)
	req.Header.Set("Authorization", "Bearer not-a-jwt")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRequireAdminRejectsNonAdmin(t *testing.T) {
	issuer := auth.NewJWTIssuer(testSecret, 15*time.Minute)
	token, err := issuer.Issue(1, false) // not admin
	require.NoError(t, err)

	handler := auth.RequireUser(issuer)(
		auth.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("handler should not run")
		})),
	)

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestRequireAdminAcceptsAdmin(t *testing.T) {
	issuer := auth.NewJWTIssuer(testSecret, 15*time.Minute)
	token, err := issuer.Issue(1, true) // admin
	require.NoError(t, err)

	handler := auth.RequireUser(issuer)(
		auth.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})),
	)

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

// Unused import guard
var _ = strings.TrimSpace
```

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./internal/core/auth/... -short
```

Expected: FAIL — `RequireUser`, `RequireAdmin` not defined.

- [ ] **Step 3: Implement middleware.go**

Create `internal/core/auth/middleware.go`:
```go
package auth

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/ismd/linktheca/internal/core/httpx"
)

// RequireUser parses a Bearer token with the given issuer, validates it, and
// attaches userID + isAdmin to the request context. Responds 401 on failure.
func RequireUser(issuer *JWTIssuer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "missing bearer token")
				return
			}
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

			claims, err := issuer.Parse(tokenStr)
			if err != nil {
				httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid token")
				return
			}

			uid, err := strconv.ParseInt(claims.Subject, 10, 64)
			if err != nil {
				httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "malformed subject")
				return
			}

			ctx := WithUser(r.Context(), uid, claims.IsAdmin)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAdmin must be composed INSIDE RequireUser. It responds 403 if the
// user on the context is not an admin.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !IsAdmin(r.Context()) {
			httpx.WriteError(w, http.StatusForbidden, "forbidden", "admin only")
			return
		}
		next.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/core/auth/... -v
```

Expected: all middleware tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/core/auth/middleware.go internal/core/auth/middleware_test.go
git commit -m "feat(auth): RequireUser and RequireAdmin middleware"
```

---

### Task 25: Auth HTTP handlers — Register and Login

**Files:**
- Create: `internal/auth/http.go`
- Create: `internal/auth/http_test.go`

- [ ] **Step 1: Write failing handler test**

Create `internal/auth/http_test.go`:
```go
package auth_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ismd/linktheca/internal/auth"
	coreauth "github.com/ismd/linktheca/internal/core/auth"
	"github.com/stretchr/testify/require"
)

func newTestHTTP(t *testing.T, registration bool) (*chi.Mux, *auth.Service, *mockStore) {
	t.Helper()
	store := newMockStore()
	issuer := coreauth.NewJWTIssuer(testSecret, 15*time.Minute)
	svc := auth.NewService(store, issuer, auth.ServiceConfig{
		RefreshTTL:          720 * time.Hour,
		RegistrationEnabled: registration,
	})

	r := chi.NewRouter()
	h := auth.NewHTTP(svc, issuer)
	h.Routes(r)
	return r, svc, store
}

func TestHTTPRegisterSuccess(t *testing.T) {
	router, _, _ := newTestHTTP(t, true)

	body, _ := json.Marshal(auth.RegisterRequest{
		Email: "http@example.com", Password: "a-strong-password", DisplayName: "HTTP",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)

	var resp auth.AuthResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.Equal(t, "http@example.com", resp.User.Email)
	require.True(t, resp.User.IsAdmin)
	require.NotEmpty(t, resp.Tokens.AccessToken)
}

func TestHTTPRegisterDisabledReturns403(t *testing.T) {
	router, _, _ := newTestHTTP(t, false)

	body, _ := json.Marshal(auth.RegisterRequest{
		Email: "x@example.com", Password: "a-strong-password", DisplayName: "X",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestHTTPLoginSuccess(t *testing.T) {
	router, svc, _ := newTestHTTP(t, true)
	_, err := svc.Register(context.Background(), auth.RegisterRequest{
		Email: "li@example.com", Password: "a-strong-password", DisplayName: "LI",
	}, "ua")
	require.NoError(t, err)

	body, _ := json.Marshal(auth.LoginRequest{
		Email: "li@example.com", Password: "a-strong-password",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestHTTPLoginWrongPassword(t *testing.T) {
	router, svc, _ := newTestHTTP(t, true)
	_, err := svc.Register(context.Background(), auth.RegisterRequest{
		Email: "lw@example.com", Password: "a-strong-password", DisplayName: "LW",
	}, "ua")
	require.NoError(t, err)

	body, _ := json.Marshal(auth.LoginRequest{
		Email: "lw@example.com", Password: "wrong",
	})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}
```

Add `"context"` to the top imports of `http_test.go` if not already present.

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./internal/auth/... -short -run HTTP
```

Expected: FAIL — `auth.NewHTTP`, `h.Routes` not defined.

- [ ] **Step 3: Implement http.go — Register and Login**

Create `internal/auth/http.go`:
```go
package auth

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	coreauth "github.com/ismd/linktheca/internal/core/auth"
	"github.com/ismd/linktheca/internal/core/httpx"
)

// HTTP hosts the auth HTTP handlers.
type HTTP struct {
	svc    *Service
	issuer *coreauth.JWTIssuer
}

// NewHTTP constructs the handler set.
func NewHTTP(svc *Service, issuer *coreauth.JWTIssuer) *HTTP {
	return &HTTP{svc: svc, issuer: issuer}
}

// Routes registers all auth routes under /auth.
func (h *HTTP) Routes(r chi.Router) {
	r.Route("/auth", func(r chi.Router) {
		r.Post("/register", h.register)
		r.Post("/login", h.login)
		r.Post("/refresh", h.refresh)

		r.Group(func(r chi.Router) {
			r.Use(coreauth.RequireUser(h.issuer))
			r.Post("/logout", h.logout)
			r.Get("/me", h.me)
		})
	})
}

func (h *HTTP) register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}

	resp, err := h.svc.Register(r.Context(), req, r.UserAgent())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, resp)
}

func (h *HTTP) login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}

	resp, err := h.svc.Login(r.Context(), req, r.UserAgent())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// refresh, logout, me are implemented in a later task (Task 26).
func (h *HTTP) refresh(w http.ResponseWriter, r *http.Request) {
	httpx.WriteError(w, http.StatusNotImplemented, "not_implemented", "")
}
func (h *HTTP) logout(w http.ResponseWriter, r *http.Request) {
	httpx.WriteError(w, http.StatusNotImplemented, "not_implemented", "")
}
func (h *HTTP) me(w http.ResponseWriter, r *http.Request) {
	httpx.WriteError(w, http.StatusNotImplemented, "not_implemented", "")
}

// writeServiceError maps domain errors to HTTP statuses uniformly.
func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrRegistrationDisabled):
		httpx.WriteError(w, http.StatusForbidden, "registration_disabled", "registration is closed")
	case errors.Is(err, ErrWeakPassword):
		httpx.WriteError(w, http.StatusBadRequest, "weak_password", "password is too short")
	case errors.Is(err, ErrEmailTaken):
		httpx.WriteError(w, http.StatusConflict, "email_taken", "email already registered")
	case errors.Is(err, ErrInvalidCredentials):
		httpx.WriteError(w, http.StatusUnauthorized, "invalid_credentials", "email or password is incorrect")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "")
	}
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/auth/... -short -v -run HTTP
```

Expected: Register and Login HTTP tests pass. (Refresh/logout/me tests do not exist yet.)

- [ ] **Step 5: Commit**

```bash
git add internal/auth/http.go internal/auth/http_test.go
git commit -m "feat(auth): HTTP handlers for Register and Login"
```

---

### Task 26: Auth HTTP handlers — Refresh, Logout, Me

**Files:**
- Modify: `internal/auth/http.go`
- Modify: `internal/auth/http_test.go`

- [ ] **Step 1: Add failing tests**

Append to `internal/auth/http_test.go`:
```go
func TestHTTPRefreshRotates(t *testing.T) {
	router, svc, _ := newTestHTTP(t, true)
	reg, err := svc.Register(context.Background(), auth.RegisterRequest{
		Email: "rfa@example.com", Password: "a-strong-password", DisplayName: "RFA",
	}, "ua")
	require.NoError(t, err)

	body, _ := json.Marshal(auth.RefreshRequest{RefreshToken: reg.Tokens.RefreshToken})
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp auth.AuthResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.NotEqual(t, reg.Tokens.RefreshToken, resp.Tokens.RefreshToken)
}

func TestHTTPLogout(t *testing.T) {
	router, svc, _ := newTestHTTP(t, true)
	reg, err := svc.Register(context.Background(), auth.RegisterRequest{
		Email: "lgo@example.com", Password: "a-strong-password", DisplayName: "LGO",
	}, "ua")
	require.NoError(t, err)

	body, _ := json.Marshal(auth.RefreshRequest{RefreshToken: reg.Tokens.RefreshToken})
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+reg.Tokens.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
}

func TestHTTPMeReturnsUser(t *testing.T) {
	router, svc, _ := newTestHTTP(t, true)
	reg, err := svc.Register(context.Background(), auth.RegisterRequest{
		Email: "meh@example.com", Password: "a-strong-password", DisplayName: "MEH",
	}, "ua")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+reg.Tokens.AccessToken)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var got auth.User
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
	require.Equal(t, "meh@example.com", got.Email)
}

func TestHTTPMeRejectsUnauthenticated(t *testing.T) {
	router, _, _ := newTestHTTP(t, true)

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}
```

- [ ] **Step 2: Run to confirm failure**

```bash
go test ./internal/auth/... -short -run "Refresh|Logout|Me" -v
```

Expected: all four new tests FAIL with 501 Not Implemented or similar.

- [ ] **Step 3: Implement the handlers**

In `internal/auth/http.go`, replace the three stub functions with:

```go
func (h *HTTP) refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}

	resp, err := h.svc.Refresh(r.Context(), req, r.UserAgent())
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *HTTP) logout(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "bad_request", "invalid json body")
		return
	}
	if err := h.svc.Logout(r.Context(), req); err != nil {
		writeServiceError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTP) me(w http.ResponseWriter, r *http.Request) {
	userID := coreauth.UserID(r.Context())
	user, err := h.svc.Me(r.Context(), userID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, user)
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/auth/... -short -v
```

Expected: all HTTP tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/http.go internal/auth/http_test.go
git commit -m "feat(auth): HTTP handlers for Refresh, Logout, Me"
```

---

## Part J: Server assembly and rate limiting

### Task 27: Server wiring in internal/server

**Files:**
- Create: `internal/server/server.go`
- Modify: `cmd/linktheca/main.go`

- [ ] **Step 1: Add CORS and httprate dependencies**

```bash
go get github.com/go-chi/cors
go get github.com/go-chi/httprate
```

- [ ] **Step 2: Create server.go**

Create `internal/server/server.go`:
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
	"github.com/ismd/linktheca/internal/core/config"
	coreauth "github.com/ismd/linktheca/internal/core/auth"
	"github.com/ismd/linktheca/internal/core/httpx"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Deps groups runtime dependencies the server needs.
type Deps struct {
	Config *config.Config
	Logger *slog.Logger
	DB     *pgxpool.Pool
}

// New builds a ready-to-serve *http.Server.
func New(deps Deps) *http.Server {
	logger := deps.Logger
	cfg := deps.Config

	issuer := coreauth.NewJWTIssuer(cfg.JWTSecret, cfg.JWTAccessTTL)

	authStore := auth.NewStore(deps.DB)
	authSvc := auth.NewService(authStore, issuer, auth.ServiceConfig{
		RefreshTTL:          cfg.JWTRefreshTTL,
		RegistrationEnabled: cfg.RegistrationEnabled,
	})
	authHTTP := auth.NewHTTP(authSvc, issuer)

	r := chi.NewRouter()

	// Base middleware chain.
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

	// Health probe — outside auth, not rate limited.
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	// Rate limit auth write endpoints.
	r.Group(func(r chi.Router) {
		r.Use(httprate.LimitByIP(10, 10*time.Minute))
		r.Post("/auth/register", authHTTP.RegisterHandler())
		r.Post("/auth/login", authHTTP.LoginHandler())
		r.Post("/auth/refresh", authHTTP.RefreshHandler())
	})

	// Non-rate-limited auth endpoints.
	r.Group(func(r chi.Router) {
		r.Use(coreauth.RequireUser(issuer))
		r.Post("/auth/logout", authHTTP.LogoutHandler())
		r.Get("/auth/me", authHTTP.MeHandler())
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return srv
}
```

- [ ] **Step 3: Expose handler functions on HTTP type**

The server assembly above calls `authHTTP.RegisterHandler()`, `LoginHandler()`, etc. The current `http.go` uses a `Routes()` method. Replace the `Routes()` method in `internal/auth/http.go` with individual exported handler accessors. Append the following (and delete the `Routes` method):

In `internal/auth/http.go`, remove the `Routes` method and add:
```go
// RegisterHandler returns the http.HandlerFunc for POST /auth/register.
func (h *HTTP) RegisterHandler() http.HandlerFunc { return h.register }

// LoginHandler returns the http.HandlerFunc for POST /auth/login.
func (h *HTTP) LoginHandler() http.HandlerFunc { return h.login }

// RefreshHandler returns the http.HandlerFunc for POST /auth/refresh.
func (h *HTTP) RefreshHandler() http.HandlerFunc { return h.refresh }

// LogoutHandler returns the http.HandlerFunc for POST /auth/logout.
func (h *HTTP) LogoutHandler() http.HandlerFunc { return h.logout }

// MeHandler returns the http.HandlerFunc for GET /auth/me.
func (h *HTTP) MeHandler() http.HandlerFunc { return h.me }
```

Also remove the `chi` import from `http.go` since `Routes` is gone.

- [ ] **Step 4: Update the http_test.go helper to not depend on Routes**

Replace `newTestHTTP` in `internal/auth/http_test.go` with:
```go
func newTestHTTP(t *testing.T, registration bool) (*chi.Mux, *auth.Service, *mockStore) {
	t.Helper()
	store := newMockStore()
	issuer := coreauth.NewJWTIssuer(testSecret, 15*time.Minute)
	svc := auth.NewService(store, issuer, auth.ServiceConfig{
		RefreshTTL:          720 * time.Hour,
		RegistrationEnabled: registration,
	})
	h := auth.NewHTTP(svc, issuer)

	r := chi.NewRouter()
	r.Post("/auth/register", h.RegisterHandler())
	r.Post("/auth/login", h.LoginHandler())
	r.Post("/auth/refresh", h.RefreshHandler())
	r.Group(func(r chi.Router) {
		r.Use(coreauth.RequireUser(issuer))
		r.Post("/auth/logout", h.LogoutHandler())
		r.Get("/auth/me", h.MeHandler())
	})
	return r, svc, store
}
```

- [ ] **Step 5: Update main.go to use server.New**

Replace `cmd/linktheca/main.go` with:
```go
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	linktheca "github.com/ismd/linktheca"
	"github.com/ismd/linktheca/internal/core/config"
	"github.com/ismd/linktheca/internal/core/db"
	"github.com/ismd/linktheca/internal/core/logging"
	"github.com/ismd/linktheca/internal/server"
)

func main() {
	if err := run(); err != nil {
		slog.Error("startup failed", "err", err)
		os.Exit(1)
	}
}

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
	logger.Info("migrations applied")

	srv := server.New(server.Deps{
		Config: cfg,
		Logger: logger,
		DB:     pool,
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
	return nil
}
```

- [ ] **Step 6: Run full test suite (short mode)**

```bash
go mod tidy
go test ./... -short -count=1
```

Expected: all tests pass. Integration tests are skipped.

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum internal/server/ internal/auth/http.go internal/auth/http_test.go cmd/linktheca/main.go
git commit -m "feat(server): wire deps, apply rate limit, expose handler accessors"
```

---

## Part K: End-to-end integration test

### Task 28: Full register → login → refresh → me → logout flow against real Postgres

**Files:**
- Create: `internal/server/server_test.go`

- [ ] **Step 1: Write integration test**

Create `internal/server/server_test.go`:
```go
package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ismd/linktheca/internal/auth"
	"github.com/ismd/linktheca/internal/core/config"
	"github.com/ismd/linktheca/internal/core/logging"
	"github.com/ismd/linktheca/internal/server"
	"github.com/ismd/linktheca/internal/testing/testdb"
	"github.com/stretchr/testify/require"
)

func TestIntegrationFullAuthFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	pool := testdb.New(t)
	cfg := &config.Config{
		HTTPAddr:            ":0",
		LogLevel:            "error",
		LogFormat:           "text",
		JWTSecret:           "integration-test-secret-that-is-long-enough",
		JWTAccessTTL:        15 * 60 * 1_000_000_000, // 15 min in ns
		JWTRefreshTTL:       24 * 60 * 60 * 1_000_000_000,
		RegistrationEnabled: true,
	}

	srv := server.New(server.Deps{
		Config: cfg,
		Logger: logging.New(io.Discard, "text", "error"),
		DB:     pool,
	})
	// Use the handler directly; we don't need to bind a port.
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	// --- Register ---
	regBody, _ := json.Marshal(auth.RegisterRequest{
		Email: "e2e@example.com", Password: "a-strong-password", DisplayName: "E2E",
	})
	resp, err := http.Post(ts.URL+"/auth/register", "application/json", bytes.NewReader(regBody))
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var reg auth.AuthResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&reg))
	resp.Body.Close()
	require.True(t, reg.User.IsAdmin, "first user must be admin")

	// --- GET /auth/me ---
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+reg.Tokens.AccessToken)
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var me auth.User
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&me))
	resp.Body.Close()
	require.Equal(t, "e2e@example.com", me.Email)

	// --- Refresh ---
	rfBody, _ := json.Marshal(auth.RefreshRequest{RefreshToken: reg.Tokens.RefreshToken})
	resp, err = http.Post(ts.URL+"/auth/refresh", "application/json", bytes.NewReader(rfBody))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var refreshed auth.AuthResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&refreshed))
	resp.Body.Close()
	require.NotEqual(t, reg.Tokens.RefreshToken, refreshed.Tokens.RefreshToken)

	// --- Old refresh must now fail ---
	oldRfBody, _ := json.Marshal(auth.RefreshRequest{RefreshToken: reg.Tokens.RefreshToken})
	resp, err = http.Post(ts.URL+"/auth/refresh", "application/json", bytes.NewReader(oldRfBody))
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()

	// --- Logout (with new refresh) ---
	logoutBody, _ := json.Marshal(auth.RefreshRequest{RefreshToken: refreshed.Tokens.RefreshToken})
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/auth/logout", bytes.NewReader(logoutBody))
	req.Header.Set("Authorization", "Bearer "+refreshed.Tokens.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// --- Logged-out refresh must fail ---
	resp, err = http.Post(ts.URL+"/auth/refresh", "application/json", bytes.NewReader(logoutBody))
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()
}

func TestIntegrationRegistrationDisabled(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	pool := testdb.New(t)
	cfg := &config.Config{
		HTTPAddr:            ":0",
		JWTSecret:           "integration-test-secret-that-is-long-enough",
		JWTAccessTTL:        15 * 60 * 1_000_000_000,
		JWTRefreshTTL:       24 * 60 * 60 * 1_000_000_000,
		RegistrationEnabled: false,
	}
	srv := server.New(server.Deps{
		Config: cfg,
		Logger: logging.New(io.Discard, "text", "error"),
		DB:     pool,
	})
	ts := httptest.NewServer(srv.Handler)
	defer ts.Close()

	body, _ := json.Marshal(auth.RegisterRequest{
		Email: "closed@example.com", Password: "a-strong-password", DisplayName: "X",
	})
	resp, err := http.Post(ts.URL+"/auth/register", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
	resp.Body.Close()
}
```

- [ ] **Step 2: Run the integration test**

```bash
go test ./internal/server/... -v -count=1
```

Expected: both integration tests pass. Container startup may take 20-30 seconds on first run.

- [ ] **Step 3: Run the full test suite once to confirm no regressions**

```bash
go test ./... -race -count=1
```

Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add internal/server/server_test.go
git commit -m "test(server): full auth flow integration test against real Postgres"
```

---

## Part L: CI

### Task 29: GitHub Actions CI

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Create workflow**

Create `.github/workflows/ci.yml`:
```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:

jobs:
  backend:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'
          cache: true

      - name: go vet
        run: go vet ./...

      - name: go test (short, unit only)
        run: go test ./... -race -count=1 -short

      - name: go test (integration with testcontainers)
        run: go test ./... -race -count=1
```

**Note:** Integration tests need Docker on the runner. GitHub's `ubuntu-latest` runners have Docker available out of the box.

- [ ] **Step 2: Commit**

```bash
mkdir -p .github/workflows
git add .github/workflows/ci.yml
git commit -m "ci: GitHub Actions for go vet + unit + integration tests"
```

---

### Task 30: Manual smoke test against running backend

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

- [ ] **Step 3: Register the first user**

```bash
curl -s -X POST http://localhost:8080/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@linktheca.local","password":"initial-admin-password","display_name":"Admin"}' | tee /tmp/register.json
```

Expected: JSON response with `"is_admin":true` and a `tokens` object.

- [ ] **Step 4: Extract the access token and call /auth/me**

```bash
ACCESS=$(jq -r '.tokens.access_token' /tmp/register.json)
curl -s -H "Authorization: Bearer $ACCESS" http://localhost:8080/auth/me
```

Expected: JSON with the registered user details.

- [ ] **Step 5: Log in with the same credentials**

```bash
curl -s -X POST http://localhost:8080/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@linktheca.local","password":"initial-admin-password"}' | jq .
```

Expected: 200 with new token pair.

- [ ] **Step 6: Stop backend and DB**

```bash
kill %1
wait 2>/dev/null
make dev-db-down
```

- [ ] **Step 7: No commit needed — this was manual verification only**

If any step failed, fix the issue, commit the fix, and re-run this task.

---

## Phase 1 complete

At this point the Linktheca backend has:

- Working Go binary that boots, connects to Postgres, runs migrations, serves HTTP on `:8080`.
- Structured logging, request IDs, graceful shutdown, panic recovery.
- Full auth flow: register (first user → admin), login, refresh with rotation, logout with revocation, me.
- argon2id password hashing, JWT HS256 access tokens, SHA-256 refresh hashes.
- Rate limiting on `/auth/register`, `/auth/login`, `/auth/refresh`.
- Optional CORS.
- Test infrastructure: unit tests with mock store, integration tests against real Postgres via testcontainers, end-to-end HTTP flow tests.
- GitHub Actions CI running all of the above.

**Next phase:** `Phase 2 — Library backend` (content extraction, article_contents, library_items, HTTP endpoints). Will be planned in its own document after this phase is complete and verified.

## Handoff notes for the executing agent

- Every task commits once at the end. If you have to retry or fix a step, still commit the fix and move to the next task.
- If testcontainers startup is slow or flaky on your machine, you can temporarily skip integration tests with `go test ./... -short`.
- If the pgx `stdlib.OpenDBFromPool` function does not exist in your pgx version, replace it with `stdlib.OpenDB(*pool.Config().ConnConfig)` — both create a `sql.DB` bound to the same Postgres.
- Exact `testcontainers-go` API surface (`tcpostgres.Run` signature, `BasicWaitStrategies`) has changed across versions. If compilation fails, run `go doc github.com/testcontainers/testcontainers-go/modules/postgres` and adapt.
- If you hit rate limits while doing manual curl smoke-testing, wait 10 minutes or restart the backend.
- Never commit `.env.local` or real secrets. Double-check `git status` before every `git add`.
