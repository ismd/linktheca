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
