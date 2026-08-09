package main

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/ismd/linktheca/internal/testing/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	"github.com/stretchr/testify/require"
)

var userCounter atomic.Uint64

// vec builds a 1024-dim vector whose first components are the given values.
func vec(head ...float32) pgvector.Vector {
	v := make([]float32, 1024)
	copy(v, head)
	return pgvector.NewVector(v)
}

func seedUser(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	var id int64
	email := fmt.Sprintf("u+%s+%d@example.com", t.Name(), userCounter.Add(1))
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO users (email, password_hash, display_name)
		 VALUES ($1, 'x', 'Tester') RETURNING id`, email).Scan(&id))
	return id
}

func seedFeed(t *testing.T, pool *pgxpool.Pool, url string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO radar_feeds (url, kind) VALUES ($1, 'rss') RETURNING id`, url).Scan(&id))
	return id
}

func seedFinding(t *testing.T, pool *pgxpool.Pool, feedID int64, title string, embedding *pgvector.Vector) int64 {
	t.Helper()
	var id int64
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO radar_findings (feed_id, url, title, embedding)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		feedID, fmt.Sprintf("https://e.example/%s/%s", t.Name(), title), title, embedding).Scan(&id))
	return id
}

func seedTopic(t *testing.T, pool *pgxpool.Pool, userID int64, name string, threshold float32, embedding *pgvector.Vector) int64 {
	t.Helper()
	var id int64
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO radar_topics (user_id, name, description, match_threshold, embedding)
		 VALUES ($1, $2, 'seeded topic description', $3, $4) RETURNING id`,
		userID, name, threshold, embedding).Scan(&id))
	return id
}

func TestTopFindingsByVector_ranksBySimilarity(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	pool := testdb.New(t)
	ctx := context.Background()

	feed := seedFeed(t, pool, "https://f.example/rank")
	near := vec(1)
	mid := vec(1, 1)
	far := vec(0, 1)
	nearID := seedFinding(t, pool, feed, "near", &near)
	midID := seedFinding(t, pool, feed, "mid", &mid)
	farID := seedFinding(t, pool, feed, "far", &far)

	rows, err := topFindingsByVector(ctx, pool, vec(1), 10)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	require.Equal(t, []int64{nearID, midID, farID}, []int64{rows[0].ID, rows[1].ID, rows[2].ID})
	require.InDelta(t, 1.0, rows[0].Similarity, 0.001)
	require.InDelta(t, 0.7071, rows[1].Similarity, 0.001)
	require.InDelta(t, 0.0, rows[2].Similarity, 0.001)
	require.Equal(t, "near", *rows[0].Title)
}

func TestTopFindingsByVector_appliesLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	pool := testdb.New(t)
	ctx := context.Background()

	feed := seedFeed(t, pool, "https://f.example/limit")
	near := vec(1)
	far := vec(0, 1)
	nearID := seedFinding(t, pool, feed, "near", &near)
	seedFinding(t, pool, feed, "far", &far)

	rows, err := topFindingsByVector(ctx, pool, vec(1), 1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, nearID, rows[0].ID)
}

func TestTopFindingsByVector_skipsFindingsWithoutEmbedding(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	pool := testdb.New(t)
	ctx := context.Background()

	feed := seedFeed(t, pool, "https://f.example/noembed")
	seedFinding(t, pool, feed, "not embedded yet", nil)

	rows, err := topFindingsByVector(ctx, pool, vec(1), 10)
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestTopFindingsByTopic_usesStoredTopicEmbedding(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	pool := testdb.New(t)
	ctx := context.Background()

	user := seedUser(t, pool)
	embedding := vec(0, 1)
	topicID := seedTopic(t, pool, user, "ai", 0.55, &embedding)

	feed := seedFeed(t, pool, "https://f.example/topic")
	near := vec(0, 1)
	far := vec(1)
	nearID := seedFinding(t, pool, feed, "near", &near)
	farID := seedFinding(t, pool, feed, "far", &far)

	rows, err := topFindingsByTopic(ctx, pool, topicID, 10, false)
	require.NoError(t, err)
	require.Equal(t, []int64{nearID, farID}, []int64{rows[0].ID, rows[1].ID})
	require.InDelta(t, 1.0, rows[0].Similarity, 0.001)
}

func TestTopFindingsByTopic_subscribedOnlyFiltersUnsubscribedFeeds(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	pool := testdb.New(t)
	ctx := context.Background()

	user := seedUser(t, pool)
	embedding := vec(1)
	topicID := seedTopic(t, pool, user, "ai", 0.55, &embedding)

	subscribed := seedFeed(t, pool, "https://f.example/sub")
	other := seedFeed(t, pool, "https://f.example/other")
	_, err := pool.Exec(ctx,
		`INSERT INTO radar_feed_subscriptions (user_id, feed_id) VALUES ($1, $2)`, user, subscribed)
	require.NoError(t, err)

	v := vec(1)
	subID := seedFinding(t, pool, subscribed, "from subscribed feed", &v)
	seedFinding(t, pool, other, "from unsubscribed feed", &v)

	all, err := topFindingsByTopic(ctx, pool, topicID, 10, false)
	require.NoError(t, err)
	require.Len(t, all, 2)

	rows, err := topFindingsByTopic(ctx, pool, topicID, 10, true)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, subID, rows[0].ID)
}

func TestLoadTopic_returnsNameAndThreshold(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	pool := testdb.New(t)
	ctx := context.Background()

	user := seedUser(t, pool)
	embedding := vec(1)
	topicID := seedTopic(t, pool, user, "webauthn", 0.62, &embedding)

	topic, err := loadTopic(ctx, pool, topicID)
	require.NoError(t, err)
	require.Equal(t, "webauthn", topic.Name)
	require.InDelta(t, 0.62, topic.MatchThreshold, 0.0001)
}

func TestLoadTopic_missingTopic(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	pool := testdb.New(t)

	_, err := loadTopic(context.Background(), pool, 424242)
	require.ErrorContains(t, err, "not found")
}

func TestLoadTopic_withoutEmbedding(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	pool := testdb.New(t)
	ctx := context.Background()

	user := seedUser(t, pool)
	topicID := seedTopic(t, pool, user, "unembedded", 0.55, nil)

	_, err := loadTopic(ctx, pool, topicID)
	require.ErrorContains(t, err, "no embedding")
}

func TestListTopics_reportsOwnerThresholdAndEmbeddingState(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}

	pool := testdb.New(t)
	ctx := context.Background()

	user := seedUser(t, pool)
	embedding := vec(1)
	embeddedID := seedTopic(t, pool, user, "embedded", 0.55, &embedding)
	bareID := seedTopic(t, pool, user, "bare", 0.7, nil)

	rows, err := listTopics(ctx, pool)
	require.NoError(t, err)
	require.Len(t, rows, 2)

	byID := map[int64]topicRow{}
	for _, r := range rows {
		byID[r.ID] = r
	}
	require.True(t, byID[embeddedID].HasEmbedding)
	require.False(t, byID[bareID].HasEmbedding)
	require.InDelta(t, 0.7, byID[bareID].MatchThreshold, 0.0001)
	require.Contains(t, byID[embeddedID].OwnerEmail, "@example.com")
}
