package jobs_test

import (
	"context"
	"testing"

	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/ismd/linktheca/internal/radar"
	"github.com/ismd/linktheca/internal/radar/crawler"
	"github.com/ismd/linktheca/internal/radar/jobs"
	"github.com/ismd/linktheca/internal/testing/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/require"
)

// fakeInserter records args and dispatches each one synchronously to the
// matching worker — so a single scheduler.Work call cascades through
// crawl → embed → match in-process.
type fakeInserter struct {
	dispatch map[string]func(ctx context.Context, args river.JobArgs)
	calls    []river.JobArgs
}

func newFakeInserter() *fakeInserter {
	return &fakeInserter{dispatch: map[string]func(context.Context, river.JobArgs){}}
}

func (f *fakeInserter) Insert(ctx context.Context, args river.JobArgs, _ *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	f.calls = append(f.calls, args)
	if fn, ok := f.dispatch[args.Kind()]; ok {
		fn(ctx, args)
	}

	return &rivertype.JobInsertResult{}, nil
}

type fakeFetcher struct{ body []byte }

func (f *fakeFetcher) Fetch(_ context.Context, _, _, _ string) (*crawler.FetchResult, error) {
	return &crawler.FetchResult{StatusCode: 200, Body: f.body, Etag: `"v1"`}, nil
}

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

func TestJobs_FullPipeline(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := radar.NewStore(pool)
	emb := &embeddings.FakeEmbedder{Dim: 1024}

	userID := seedUser(t, pool)
	feed, err := store.AddFeed(context.Background(), radar.AddFeedParams{
		URL: "https://x.example/feed.xml", Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)
	_, err = store.Subscribe(context.Background(), userID, feed.ID)
	require.NoError(t, err)

	topic, err := store.CreateTopic(context.Background(), radar.CreateTopicParams{
		UserID: userID, Name: "ML", Description: "machine learning research", MatchThreshold: 0.0,
	})
	require.NoError(t, err)
	tvec, _ := emb.Embed(context.Background(), "machine learning research")
	require.NoError(t, store.UpdateTopicEmbedding(context.Background(), topic.ID, pgvector.NewVector(tvec)))

	rss := []byte(`<?xml version="1.0"?>
<rss version="2.0"><channel>
<item><title>OpenAI ships</title><link>https://news.example/post/1</link>
<description>About models</description><guid>g1</guid></item>
</channel></rss>`)

	scheduler := jobs.NewScheduleCrawlsWorker(store)
	crawl := jobs.NewCrawlFeedWorker(store, &fakeFetcher{body: rss})
	embed := jobs.NewEmbedFindingWorker(store, emb)
	match := jobs.NewMatchFindingWorker(store)

	inserter := newFakeInserter()
	scheduler.SetInserter(inserter)
	crawl.SetInserter(inserter)
	embed.SetInserter(inserter)

	inserter.dispatch[jobs.CrawlFeedArgs{}.Kind()] = func(ctx context.Context, a river.JobArgs) {
		require.NoError(t, crawl.Work(ctx, &river.Job[jobs.CrawlFeedArgs]{Args: a.(jobs.CrawlFeedArgs)}))
	}
	inserter.dispatch[jobs.EmbedFindingArgs{}.Kind()] = func(ctx context.Context, a river.JobArgs) {
		require.NoError(t, embed.Work(ctx, &river.Job[jobs.EmbedFindingArgs]{Args: a.(jobs.EmbedFindingArgs)}))
	}
	inserter.dispatch[jobs.MatchFindingArgs{}.Kind()] = func(ctx context.Context, a river.JobArgs) {
		require.NoError(t, match.Work(ctx, &river.Job[jobs.MatchFindingArgs]{Args: a.(jobs.MatchFindingArgs)}))
	}

	// Cascade.
	require.NoError(t, scheduler.Work(context.Background(), &river.Job[jobs.ScheduleCrawlsArgs]{}))

	var n int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM radar_topic_matches WHERE topic_id=$1`, topic.ID).Scan(&n))
	require.Equal(t, 1, n, "expected exactly one match")

	// Idempotent: second cascade does not duplicate.
	require.NoError(t, scheduler.Work(context.Background(), &river.Job[jobs.ScheduleCrawlsArgs]{}))
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM radar_topic_matches WHERE topic_id=$1`, topic.ID).Scan(&n))
	require.Equal(t, 1, n, "second cascade must be idempotent")
}
