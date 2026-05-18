package radar_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/ismd/linktheca/internal/radar"
	"github.com/ismd/linktheca/internal/testing/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	"github.com/stretchr/testify/require"
)

var seedUserCounter atomic.Uint64

func seedUser(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	email := fmt.Sprintf("u+%s+%d@example.com", t.Name(), seedUserCounter.Add(1))
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email, password_hash, display_name)
		 VALUES ($1, $2, $3) RETURNING id`,
		email, "x", "Tester").Scan(&id)
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

func seedTopic(t *testing.T, pool *pgxpool.Pool, userID int64, name, desc string, threshold float32, active bool) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO radar_topics (user_id, name, description, match_threshold, is_active)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		userID, name, desc, threshold, active).Scan(&id)
	require.NoError(t, err)
	return id
}

func seedFeed(t *testing.T, pool *pgxpool.Pool, url, title string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO radar_feeds (url, kind, title) VALUES ($1, 'rss', $2) RETURNING id`,
		url, title).Scan(&id)
	require.NoError(t, err)
	return id
}

func seedFinding(t *testing.T, pool *pgxpool.Pool, feedID int64, url, title string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO radar_findings (feed_id, url, title) VALUES ($1, $2, $3) RETURNING id`,
		feedID, url, title).Scan(&id)
	require.NoError(t, err)
	return id
}

func seedMatch(t *testing.T, pool *pgxpool.Pool, topicID, findingID int64, state string, similarity float32) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO radar_topic_matches (topic_id, finding_id, similarity, state)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		topicID, findingID, similarity, state).Scan(&id)
	require.NoError(t, err)
	return id
}

func seedSubscription(t *testing.T, pool *pgxpool.Pool, userID, feedID int64) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO radar_feed_subscriptions (user_id, feed_id) VALUES ($1, $2)`,
		userID, feedID)
	require.NoError(t, err)
}

func TestStore_ListTopicsWithStats_empty(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)

	items, err := store.ListTopicsWithStats(ctx, userID)
	require.NoError(t, err)
	require.Empty(t, items)
}

func TestStore_ListTopicsWithStats_aggregates(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)
	topicA := seedTopic(t, pool, userID, "A", "desc-a", 0.55, true)
	topicB := seedTopic(t, pool, userID, "B", "desc-b", 0.6, true)

	feed1 := seedFeed(t, pool, "https://f1.example/rss", "Feed1")
	feed2 := seedFeed(t, pool, "https://f2.example/rss", "Feed2")
	f1a := seedFinding(t, pool, feed1, "https://x.example/1", "t1")
	f1b := seedFinding(t, pool, feed1, "https://x.example/2", "t2")
	f2a := seedFinding(t, pool, feed2, "https://x.example/3", "t3")

	// topicA: 2 new + 1 seen across 2 feeds
	seedMatch(t, pool, topicA, f1a, "new", 0.7)
	seedMatch(t, pool, topicA, f1b, "new", 0.71)
	seedMatch(t, pool, topicA, f2a, "seen", 0.65)
	// topicB: no matches

	items, err := store.ListTopicsWithStats(ctx, userID)
	require.NoError(t, err)
	require.Len(t, items, 2)

	byID := make(map[int64]radar.TopicWithStats, len(items))
	for _, it := range items {
		byID[it.ID] = it
	}

	a := byID[topicA]
	require.Equal(t, 2, a.Stats.NewCount)
	require.Equal(t, 3, a.Stats.TotalCount)
	require.Equal(t, 2, a.Stats.SourceCount)
	require.NotNil(t, a.Stats.LastMatchAt)

	b := byID[topicB]
	require.Equal(t, 0, b.Stats.NewCount)
	require.Equal(t, 0, b.Stats.TotalCount)
	require.Equal(t, 0, b.Stats.SourceCount)
	require.Nil(t, b.Stats.LastMatchAt)
}

func TestStore_ListTopicsWithStats_isolation(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userA := seedUser(t, pool)
	userB := seedUser(t, pool)
	seedTopic(t, pool, userB, "OtherB", "other-desc", 0.55, true)

	items, err := store.ListTopicsWithStats(ctx, userA)
	require.NoError(t, err)
	require.Empty(t, items)
}

func TestStore_GetTopicWithStats_notFound(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userA := seedUser(t, pool)
	userB := seedUser(t, pool)
	otherTopic := seedTopic(t, pool, userB, "OtherB", "other-desc", 0.55, true)

	_, err := store.GetTopicWithStats(ctx, userA, otherTopic)
	require.ErrorIs(t, err, radar.ErrNotFound)
}
