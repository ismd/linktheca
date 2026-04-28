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
