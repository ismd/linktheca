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
