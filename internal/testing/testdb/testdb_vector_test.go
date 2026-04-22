package testdb_test

import (
	"context"
	"testing"

	"github.com/ismd/linktheca/internal/testing/testdb"
	"github.com/pgvector/pgvector-go"
	"github.com/stretchr/testify/require"
)

// TestVectorTypeAvailable asserts that the pgvector `vector` type is usable inside the per-test schema created by testdb.New
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
