package radar_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

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

func TestStore_UpdateTopic_partial(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)
	topicID := seedTopic(t, pool, userID, "orig", "orig description", 0.55, true)

	newName := "renamed"
	updated, err := store.UpdateTopic(ctx, userID, topicID, radar.UpdateTopicParams{
		Name: &newName,
	})
	require.NoError(t, err)
	require.Equal(t, "renamed", updated.Name)
	require.Equal(t, "orig description", updated.Description) // unchanged
	require.Equal(t, float32(0.55), updated.MatchThreshold)
	require.True(t, updated.IsActive)
}

func TestStore_UpdateTopic_allFields(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)
	topicID := seedTopic(t, pool, userID, "orig", "orig description", 0.55, true)

	name := "new-name"
	desc := "new description"
	threshold := float32(0.7)
	active := false
	updated, err := store.UpdateTopic(ctx, userID, topicID, radar.UpdateTopicParams{
		Name: &name, Description: &desc, MatchThreshold: &threshold, IsActive: &active,
	})
	require.NoError(t, err)
	require.Equal(t, name, updated.Name)
	require.Equal(t, desc, updated.Description)
	require.Equal(t, threshold, updated.MatchThreshold)
	require.False(t, updated.IsActive)
}

func TestStore_UpdateTopic_otherUser(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userA := seedUser(t, pool)
	userB := seedUser(t, pool)
	topicID := seedTopic(t, pool, userB, "B's topic", "B's description", 0.55, true)

	name := "stolen"
	_, err := store.UpdateTopic(ctx, userA, topicID, radar.UpdateTopicParams{Name: &name})
	require.ErrorIs(t, err, radar.ErrNotFound)
}

func TestStore_DeleteTopic_cascades(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)
	topicID := seedTopic(t, pool, userID, "doomed", "to be deleted", 0.55, true)
	feedID := seedFeed(t, pool, "https://x.example/rss", "X")
	findingID := seedFinding(t, pool, feedID, "https://x.example/a", "a")
	matchID := seedMatch(t, pool, topicID, findingID, "new", 0.7)

	require.NoError(t, store.DeleteTopic(ctx, userID, topicID))

	// match is gone via CASCADE
	var count int
	err := pool.QueryRow(ctx,
		`SELECT count(*) FROM radar_topic_matches WHERE id=$1`, matchID).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 0, count)
}

func TestStore_DeleteTopic_otherUser(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userA := seedUser(t, pool)
	userB := seedUser(t, pool)
	topicID := seedTopic(t, pool, userB, "B's", "B's desc", 0.55, true)

	err := store.DeleteTopic(ctx, userA, topicID)
	require.ErrorIs(t, err, radar.ErrNotFound)
}

func TestStore_ListMatches_filters(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)
	topicA := seedTopic(t, pool, userID, "A", "desc-a", 0.55, true)
	topicB := seedTopic(t, pool, userID, "B", "desc-b", 0.55, true)
	feedID := seedFeed(t, pool, "https://x.example/rss", "X-feed")
	f1 := seedFinding(t, pool, feedID, "https://x.example/1", "title-1")
	f2 := seedFinding(t, pool, feedID, "https://x.example/2", "title-2")
	f3 := seedFinding(t, pool, feedID, "https://x.example/3", "title-3")
	seedMatch(t, pool, topicA, f1, "new", 0.7)
	seedMatch(t, pool, topicA, f2, "seen", 0.71)
	seedMatch(t, pool, topicB, f3, "new", 0.72)

	// No filters → all 3
	items, total, err := store.ListMatches(ctx, userID, radar.ListMatchesParams{Limit: 50})
	require.NoError(t, err)
	require.Len(t, items, 3)
	require.Equal(t, 3, total)

	// Filter by topic A → 2
	items, total, err = store.ListMatches(ctx, userID,
		radar.ListMatchesParams{TopicID: &topicA, Limit: 50})
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, 2, total)

	// Filter by state=new → 2 (topicA-f1 + topicB-f3)
	stateNew := "new"
	items, total, err = store.ListMatches(ctx, userID,
		radar.ListMatchesParams{State: &stateNew, Limit: 50})
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, 2, total)

	// Combined: topic A + state=seen → 1
	stateSeen := "seen"
	items, total, err = store.ListMatches(ctx, userID,
		radar.ListMatchesParams{TopicID: &topicA, State: &stateSeen, Limit: 50})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, 1, total)
}

func TestStore_ListMatches_denormalization(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)
	topicID := seedTopic(t, pool, userID, "Local-first", "local-first software", 0.55, true)
	feedID := seedFeed(t, pool, "https://hn.example/rss", "Hacker News")
	findingID := seedFinding(t, pool, feedID, "https://hn.example/a", "article-title")
	seedMatch(t, pool, topicID, findingID, "new", 0.73)

	items, _, err := store.ListMatches(ctx, userID, radar.ListMatchesParams{Limit: 50})
	require.NoError(t, err)
	require.Len(t, items, 1)
	m := items[0]
	require.Equal(t, "Local-first", m.TopicName)
	require.Equal(t, findingID, m.Finding.ID)
	require.Equal(t, feedID, m.Finding.FeedID)
	require.NotNil(t, m.Finding.FeedTitle)
	require.Equal(t, "Hacker News", *m.Finding.FeedTitle)
	require.NotNil(t, m.Finding.Title)
	require.Equal(t, "article-title", *m.Finding.Title)
}

func TestStore_ListMatches_isolation(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userA := seedUser(t, pool)
	userB := seedUser(t, pool)
	topicB := seedTopic(t, pool, userB, "B's topic", "B's desc", 0.55, true)
	feedID := seedFeed(t, pool, "https://x.example/rss", "X")
	findingID := seedFinding(t, pool, feedID, "https://x.example/a", "a")
	seedMatch(t, pool, topicB, findingID, "new", 0.7)

	// userA sees nothing globally
	items, total, err := store.ListMatches(ctx, userA, radar.ListMatchesParams{Limit: 50})
	require.NoError(t, err)
	require.Empty(t, items)
	require.Equal(t, 0, total)

	// userA cannot peek into B's topic by passing topic_id
	items, total, err = store.ListMatches(ctx, userA,
		radar.ListMatchesParams{TopicID: &topicB, Limit: 50})
	require.NoError(t, err)
	require.Empty(t, items)
	require.Equal(t, 0, total)
}

func TestStore_ListMatches_pagination(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)
	topicID := seedTopic(t, pool, userID, "T", "desc-t", 0.55, true)
	feedID := seedFeed(t, pool, "https://x.example/rss", "X")
	for i := 0; i < 5; i++ {
		fid := seedFinding(t, pool, feedID,
			fmt.Sprintf("https://x.example/a%d", i),
			fmt.Sprintf("title-%d", i))
		seedMatch(t, pool, topicID, fid, "new", 0.7)
	}

	items, total, err := store.ListMatches(ctx, userID,
		radar.ListMatchesParams{Limit: 2, Offset: 0})
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, 5, total)

	items, _, err = store.ListMatches(ctx, userID,
		radar.ListMatchesParams{Limit: 2, Offset: 4})
	require.NoError(t, err)
	require.Len(t, items, 1)
}

func TestStore_UpdateMatchState_ownership(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userA := seedUser(t, pool)
	userB := seedUser(t, pool)
	topicB := seedTopic(t, pool, userB, "B", "B's desc", 0.55, true)
	feedID := seedFeed(t, pool, "https://x.example/rss", "X")
	findingID := seedFinding(t, pool, feedID, "https://x.example/a", "a")
	matchID := seedMatch(t, pool, topicB, findingID, "new", 0.7)

	err := store.UpdateMatchState(ctx, userA, matchID, "seen")
	require.ErrorIs(t, err, radar.ErrNotFound)

	// B can update
	err = store.UpdateMatchState(ctx, userB, matchID, "seen")
	require.NoError(t, err)
}

func TestStore_UpdateMatchState_idempotent(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)
	topicID := seedTopic(t, pool, userID, "T", "desc", 0.55, true)
	feedID := seedFeed(t, pool, "https://x.example/rss", "X")
	findingID := seedFinding(t, pool, feedID, "https://x.example/a", "a")
	matchID := seedMatch(t, pool, topicID, findingID, "seen", 0.7)

	require.NoError(t, store.UpdateMatchState(ctx, userID, matchID, "seen"))
	require.NoError(t, store.UpdateMatchState(ctx, userID, matchID, "seen"))
}

func TestStore_LastSweepAt_noSubs(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)

	last, err := store.LastSweepAt(ctx, userID)
	require.NoError(t, err)
	require.Nil(t, last)
}

func TestStore_LastSweepAt_picksMax(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)
	feed1 := seedFeed(t, pool, "https://f1.example/rss", "F1")
	feed2 := seedFeed(t, pool, "https://f2.example/rss", "F2")
	seedSubscription(t, pool, userID, feed1)
	seedSubscription(t, pool, userID, feed2)

	// Set distinct fetch timestamps; feed2 is most recent.
	_, err := pool.Exec(ctx,
		`UPDATE radar_feeds SET last_fetched_at = $1 WHERE id = $2`,
		time.Date(2026, 5, 14, 10, 0, 0, 0, time.UTC), feed1)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`UPDATE radar_feeds SET last_fetched_at = $1 WHERE id = $2`,
		time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC), feed2)
	require.NoError(t, err)

	last, err := store.LastSweepAt(ctx, userID)
	require.NoError(t, err)
	require.NotNil(t, last)
	require.Equal(t, 12, last.UTC().Hour())
}

func TestStore_ListFeeds_pagination(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)
	for i := 0; i < 4; i++ {
		seedFeed(t, pool,
			fmt.Sprintf("https://feed-%d.example/rss", i),
			fmt.Sprintf("Feed %d", i))
	}

	items, total, err := store.ListFeeds(ctx, userID, radar.ListFeedsParams{Limit: 2, Offset: 0})
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, 4, total)

	items, _, err = store.ListFeeds(ctx, userID, radar.ListFeedsParams{Limit: 2, Offset: 3})
	require.NoError(t, err)
	require.Len(t, items, 1)
}

func TestStore_GetMatch_ok(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)
	topicID := seedTopic(t, pool, userID, "A", "desc", 0.55, true)
	feedID := seedFeed(t, pool, "https://f.example/rss", "F1")
	findingID := seedFinding(t, pool, feedID, "https://x.example/1", "t1")
	matchID := seedMatch(t, pool, topicID, findingID, "new", 0.7)

	mv, err := store.GetMatch(ctx, userID, matchID)
	require.NoError(t, err)
	require.Equal(t, matchID, mv.ID)
	require.Equal(t, topicID, mv.TopicID)
	require.Equal(t, "A", mv.TopicName)
	require.Equal(t, float32(0.7), mv.Similarity)
	require.Equal(t, "new", mv.State)
	require.Equal(t, findingID, mv.Finding.ID)
	require.Equal(t, "https://x.example/1", mv.Finding.URL)
	require.NotNil(t, mv.Finding.FeedTitle)
	require.Equal(t, "F1", *mv.Finding.FeedTitle)
}

func TestStore_GetMatch_notFound(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)

	_, err := store.GetMatch(ctx, userID, 99999)
	require.ErrorIs(t, err, radar.ErrNotFound)
}

func TestStore_GetMatch_otherUser(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userA := seedUser(t, pool)
	userB := seedUser(t, pool)
	topicB := seedTopic(t, pool, userB, "B", "desc-b", 0.55, true)
	feedID := seedFeed(t, pool, "https://f.example/rss2", "F2")
	findingID := seedFinding(t, pool, feedID, "https://x.example/2", "t2")
	matchID := seedMatch(t, pool, topicB, findingID, "new", 0.7)

	_, err := store.GetMatch(ctx, userA, matchID)
	require.ErrorIs(t, err, radar.ErrNotFound)
}

// seedFindingEmbedding sets a finding's embedding to the unit vector pointing
// along axis, so cosine similarity against another axis is exactly 0 or 1.
func seedFindingEmbedding(t *testing.T, pool *pgxpool.Pool, findingID int64, axis int) {
	t.Helper()
	vec := make([]float32, 1024)
	vec[axis] = 1
	_, err := pool.Exec(context.Background(),
		`UPDATE radar_findings SET embedding = $1 WHERE id = $2`,
		pgvector.NewVector(vec), findingID)
	require.NoError(t, err)
}

func TestStore_PreviewFindings_ranksSubscribedFindings(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)
	subscribed := seedFeed(t, pool, "https://f.example/sub", "Subscribed Feed")
	other := seedFeed(t, pool, "https://f.example/other", "Other Feed")
	seedSubscription(t, pool, userID, subscribed)

	hit := seedFinding(t, pool, subscribed, "https://p.example/hit", "Hit")
	miss := seedFinding(t, pool, subscribed, "https://p.example/miss", "Miss")
	unsubscribed := seedFinding(t, pool, other, "https://p.example/nope", "Unsubscribed")
	unembedded := seedFinding(t, pool, subscribed, "https://p.example/raw", "No embedding")

	seedFindingEmbedding(t, pool, hit, 0)
	seedFindingEmbedding(t, pool, miss, 1)
	seedFindingEmbedding(t, pool, unsubscribed, 0)

	probe := make([]float32, 1024)
	probe[0] = 1

	items, err := store.PreviewFindings(ctx, userID, pgvector.NewVector(probe), 5)
	require.NoError(t, err)
	require.Len(t, items, 2, "unsubscribed and unembedded findings must not appear")

	require.Equal(t, hit, items[0].Finding.ID)
	require.InDelta(t, 1.0, items[0].Similarity, 0.001)
	require.Equal(t, miss, items[1].Finding.ID)
	require.InDelta(t, 0.0, items[1].Similarity, 0.001)

	require.NotNil(t, items[0].Finding.FeedTitle)
	require.Equal(t, "Subscribed Feed", *items[0].Finding.FeedTitle)
	require.NotNil(t, items[0].Finding.Title)
	require.Equal(t, "Hit", *items[0].Finding.Title)

	for _, it := range items {
		require.NotEqual(t, unsubscribed, it.Finding.ID)
		require.NotEqual(t, unembedded, it.Finding.ID)
	}
}

func TestStore_PreviewFindings_respectsLimit(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)
	feed := seedFeed(t, pool, "https://f.example/lim", "Feed")
	seedSubscription(t, pool, userID, feed)

	for i := 0; i < 4; i++ {
		id := seedFinding(t, pool, feed, fmt.Sprintf("https://p.example/%d", i), "T")
		seedFindingEmbedding(t, pool, id, i)
	}

	probe := make([]float32, 1024)
	probe[0] = 1

	items, err := store.PreviewFindings(ctx, userID, pgvector.NewVector(probe), 2)
	require.NoError(t, err)
	require.Len(t, items, 2)
}

func TestStore_PreviewFindings_emptyForUserWithoutSubscriptions(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)
	feed := seedFeed(t, pool, "https://f.example/none", "Feed")
	id := seedFinding(t, pool, feed, "https://p.example/none", "T")
	seedFindingEmbedding(t, pool, id, 0)

	probe := make([]float32, 1024)
	probe[0] = 1

	items, err := store.PreviewFindings(ctx, userID, pgvector.NewVector(probe), 5)
	require.NoError(t, err)
	require.Empty(t, items)
}

func TestStore_ListFeeds_SubscribedAndCounts(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)

	subscribed, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: fmt.Sprintf("https://a.example/%d.xml", userID), Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)
	other, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: fmt.Sprintf("https://b.example/%d.xml", userID), Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)

	_, err = store.Subscribe(ctx, userID, subscribed.ID)
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		ext := fmt.Sprintf("ext-%d", i)
		_, _, err = store.UpsertFinding(ctx, radar.FindingUpsert{
			FeedID: subscribed.ID, ExternalID: &ext,
			URL: fmt.Sprintf("https://a.example/post/%d", i),
		})
		require.NoError(t, err)
	}

	items, total, err := store.ListFeeds(ctx, userID, radar.ListFeedsParams{Limit: 100})
	require.NoError(t, err)
	require.GreaterOrEqual(t, total, 2)

	byID := map[int64]radar.FeedListItem{}
	for _, it := range items {
		byID[it.ID] = it
	}

	require.True(t, byID[subscribed.ID].Subscribed)
	require.Equal(t, 3, byID[subscribed.ID].FindingCount)
	require.False(t, byID[other.ID].Subscribed)
	require.Equal(t, 0, byID[other.ID].FindingCount)
}

func TestStore_Unsubscribe(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	userID := seedUser(t, pool)
	feed, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: fmt.Sprintf("https://unsub.example/%d.xml", userID), Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)
	_, err = store.Subscribe(ctx, userID, feed.ID)
	require.NoError(t, err)

	require.NoError(t, store.Unsubscribe(ctx, userID, feed.ID))

	items, _, err := store.ListFeeds(ctx, userID, radar.ListFeedsParams{Limit: 100})
	require.NoError(t, err)
	for _, it := range items {
		if it.ID == feed.ID {
			require.False(t, it.Subscribed)
		}
	}

	// Idempotent: a second call is not an error.
	require.NoError(t, store.Unsubscribe(ctx, userID, feed.ID))
}

func TestStore_UpdateFeed_PartialAndClearTitle(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	feed, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL:  fmt.Sprintf("https://patch.example/%d.xml", time.Now().UnixNano()),
		Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)

	title := "The Verge"
	updated, err := store.UpdateFeed(ctx, feed.ID, radar.UpdateFeedParams{Title: &title})
	require.NoError(t, err)
	require.Equal(t, "The Verge", *updated.Title)
	require.Equal(t, 3600, updated.FetchIntervalSeconds, "untouched field keeps its value")

	empty := ""
	cleared, err := store.UpdateFeed(ctx, feed.ID, radar.UpdateFeedParams{Title: &empty})
	require.NoError(t, err)
	require.Nil(t, cleared.Title)

	_, err = store.UpdateFeed(ctx, 999999, radar.UpdateFeedParams{Title: &title})
	require.ErrorIs(t, err, radar.ErrNotFound)
}

func TestStore_SeedSubscriptions_ActiveOnlyAndIdempotent(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	stamp := time.Now().UnixNano()
	active, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: fmt.Sprintf("https://seed-a.example/%d.xml", stamp), Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)
	paused, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: fmt.Sprintf("https://seed-b.example/%d.xml", stamp), Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)
	off := false
	_, err = store.UpdateFeed(ctx, paused.ID, radar.UpdateFeedParams{IsActive: &off})
	require.NoError(t, err)

	userID := seedUser(t, pool)
	n, err := store.SeedSubscriptions(ctx, userID)
	require.NoError(t, err)
	require.GreaterOrEqual(t, n, 1)

	items, _, err := store.ListFeeds(ctx, userID, radar.ListFeedsParams{Limit: 100})
	require.NoError(t, err)
	byID := map[int64]radar.FeedListItem{}
	for _, it := range items {
		byID[it.ID] = it
	}
	require.True(t, byID[active.ID].Subscribed)
	require.False(t, byID[paused.ID].Subscribed)

	again, err := store.SeedSubscriptions(ctx, userID)
	require.NoError(t, err)
	require.Equal(t, 0, again, "second seeding inserts nothing")
}

func TestStore_MarkFeedFetched_TitleFillsOnlyWhenEmpty(t *testing.T) {
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	ctx := context.Background()

	feed, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL:  fmt.Sprintf("https://title.example/%d.xml", time.Now().UnixNano()),
		Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)

	auto := "Auto Title"
	require.NoError(t, store.MarkFeedFetched(ctx, feed.ID, nil, nil, &auto))

	items, _, err := store.ListFeeds(ctx, 0, radar.ListFeedsParams{Limit: 100})
	require.NoError(t, err)
	require.Equal(t, "Auto Title", *findFeed(t, items, feed.ID).Title)

	manual := "Manual Title"
	_, err = store.UpdateFeed(ctx, feed.ID, radar.UpdateFeedParams{Title: &manual})
	require.NoError(t, err)

	other := "Auto Again"
	require.NoError(t, store.MarkFeedFetched(ctx, feed.ID, nil, nil, &other))

	items, _, err = store.ListFeeds(ctx, 0, radar.ListFeedsParams{Limit: 100})
	require.NoError(t, err)
	require.Equal(t, "Manual Title", *findFeed(t, items, feed.ID).Title)
}

func findFeed(t *testing.T, items []radar.FeedListItem, id int64) radar.FeedListItem {
	t.Helper()
	for _, it := range items {
		if it.ID == id {
			return it
		}
	}
	t.Fatalf("feed %d not in catalog", id)
	return radar.FeedListItem{}
}
