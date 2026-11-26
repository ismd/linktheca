package jobs_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ismd/linktheca/internal/core/content"
	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/ismd/linktheca/internal/core/netsafe"
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
	// Fake embeddings are unrelated to each other, so the topic is given the
	// finding's own match vector: the score is then its squared length, > 0.
	tvec, _ := emb.Embed(context.Background(), "machine learning research")
	require.NoError(t, store.UpdateTopicEmbedding(context.Background(), topic.ID,
		pgvector.NewVector(tvec), findingMatchVector(t, emb, "OpenAI ships\nAbout models", "OpenAI ships")))

	rss := []byte(`<?xml version="1.0"?>
<rss version="2.0"><channel>
<item><title>OpenAI ships</title><link>https://news.example/post/1</link>
<description>About models</description><guid>g1</guid></item>
</channel></rss>`)

	scheduler := jobs.NewScheduleCrawlsWorker(store)
	crawl := jobs.NewCrawlFeedWorker(store, &fakeFetcher{body: rss}, false)
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

// With page downloads on, a finding is embedded from its title and the
// article text rather than from the feed's summary.
func TestJobs_FullPipelineWithPageFetch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.Background()
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	client := netsafe.NewClient(netsafe.Options{Timeout: 5 * time.Second, AllowPrivate: true})
	extractor := content.NewExtractor(client)

	s := newSite(t, map[string]page{"/post": htmlPage(articlePage("Rust 2.0", articleText))})
	article, err := extractor.Extract(ctx, s.URL+"/post")
	require.NoError(t, err)

	userID := seedUser(t, pool)
	feed, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: "https://x.example/feed.xml", Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)
	_, err = store.Subscribe(ctx, userID, feed.ID)
	require.NoError(t, err)

	// The topic is given the match vector the finding should end up with,
	// from the page text and the title. The score is then the vector's squared
	// length, ~0.5 for unrelated fake embeddings; a finding embedded from the
	// feed's summary instead shares only the title half and scores ~0.25.
	text := "Rust 2.0 released\n" + strings.TrimSpace(article.Text)
	want := findingMatchVector(t, emb, text, "Rust 2.0 released")
	topic, err := store.CreateTopic(ctx, radar.CreateTopicParams{
		UserID: userID, Name: "Rust", Description: "Rust releases", MatchThreshold: dot(want, want) - 0.01,
	})
	require.NoError(t, err)
	tvec, _ := emb.Embed(ctx, "Rust releases")
	require.NoError(t, store.UpdateTopicEmbedding(ctx, topic.ID, pgvector.NewVector(tvec), want))

	rss := []byte(`<?xml version="1.0"?>
<rss version="2.0"><channel>
<item><title>Rust 2.0 released</title><link>` + s.URL + `/post</link>
<description>Short feed summary</description><guid>g1</guid></item>
</channel></rss>`)

	crawl := jobs.NewCrawlFeedWorker(store, &fakeFetcher{body: rss}, true)
	fetch := jobs.NewFetchFindingContentWorker(store, content.NewStore(pool), extractor,
		crawler.NewRobots(client), crawler.NewHostPacer(0))
	embed := jobs.NewEmbedFindingWorker(store, emb)
	match := jobs.NewMatchFindingWorker(store)

	inserter := newFakeInserter()
	crawl.SetInserter(inserter)
	fetch.SetInserter(inserter)
	embed.SetInserter(inserter)

	inserter.dispatch[jobs.FetchFindingContentArgs{}.Kind()] = func(ctx context.Context, a river.JobArgs) {
		job := &river.Job[jobs.FetchFindingContentArgs]{
			JobRow: &rivertype.JobRow{ID: 1, Attempt: 1, MaxAttempts: 3},
			Args:   a.(jobs.FetchFindingContentArgs),
		}
		require.NoError(t, fetch.Work(ctx, job))
	}
	inserter.dispatch[jobs.EmbedFindingArgs{}.Kind()] = func(ctx context.Context, a river.JobArgs) {
		require.NoError(t, embed.Work(ctx, &river.Job[jobs.EmbedFindingArgs]{Args: a.(jobs.EmbedFindingArgs)}))
	}
	inserter.dispatch[jobs.MatchFindingArgs{}.Kind()] = func(ctx context.Context, a river.JobArgs) {
		require.NoError(t, match.Work(ctx, &river.Job[jobs.MatchFindingArgs]{Args: a.(jobs.MatchFindingArgs)}))
	}

	require.NoError(t, crawl.Work(ctx, &river.Job[jobs.CrawlFeedArgs]{Args: jobs.CrawlFeedArgs{FeedID: feed.ID}}))

	var n int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM radar_topic_matches WHERE topic_id=$1`, topic.ID).Scan(&n))
	require.Equal(t, 1, n, "the finding must be embedded from the page text")

	var got pgvector.Vector
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT match_embedding FROM radar_findings WHERE feed_id = $1`, feed.ID).Scan(&got))
	require.InDeltaSlice(t, want.Slice(), got.Slice(), 1e-6, "text and title make the match vector")
}

// findingMatchVector is the match vector EmbedFindingWorker stores for a
// finding embedded from text whose title is title.
func findingMatchVector(t *testing.T, emb embeddings.Client, text, title string) pgvector.Vector {
	t.Helper()
	d, err := emb.Embed(context.Background(), text)
	require.NoError(t, err)
	tv, err := emb.Embed(context.Background(), title)
	require.NoError(t, err)
	return pgvector.NewVector(radar.FindingMatchVector(d, tv))
}

func dot(a, b pgvector.Vector) float32 {
	var s float32
	for i, x := range a.Slice() {
		s += x * b.Slice()[i]
	}
	return s
}

func TestEmbedFinding_ReembedAndSkipMatch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.Background()
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	emb := &embeddings.FakeEmbedder{Dim: 1024}

	feed, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: "https://x.example/feed.xml", Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)
	title, ext := "Rust 2.0 released", "g1"
	f, _, err := store.UpsertFinding(ctx, radar.FindingUpsert{
		FeedID: feed.ID, ExternalID: &ext, URL: "https://x.example/post", Title: &title,
	})
	require.NoError(t, err)

	old, _ := emb.Embed(ctx, "something else entirely")
	require.NoError(t, store.UpdateFindingEmbedding(ctx, f.ID, pgvector.NewVector(old), pgvector.NewVector(old)))

	embedding := func() []float32 {
		var v pgvector.Vector
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT embedding FROM radar_findings WHERE id=$1`, f.ID).Scan(&v))
		return v.Slice()
	}
	matchEmbedding := func() []float32 {
		var v pgvector.Vector
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT match_embedding FROM radar_findings WHERE id=$1`, f.ID).Scan(&v))
		return v.Slice()
	}

	inserter := newFakeInserter()
	embed := jobs.NewEmbedFindingWorker(store, emb)
	embed.SetInserter(inserter)

	require.NoError(t, embed.Work(ctx, &river.Job[jobs.EmbedFindingArgs]{
		Args: jobs.EmbedFindingArgs{FindingID: f.ID},
	}))
	require.Equal(t, old, embedding(), "an existing embedding is kept by default")

	require.NoError(t, embed.Work(ctx, &river.Job[jobs.EmbedFindingArgs]{
		Args: jobs.EmbedFindingArgs{FindingID: f.ID, Reembed: true, SkipMatch: true},
	}))
	want, _ := emb.Embed(ctx, title)
	require.Equal(t, want, embedding())
	require.Equal(t, want, matchEmbedding(), "embedded from the title alone, the title adds nothing")
	require.Empty(t, inserter.calls, "SkipMatch queues no match")
}

// Rows embedded before match vectors existed get them: topics in place from
// their stored embedding, findings through a title embedding job each.
func TestBackfillMatchVectors(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.Background()
	pool := testdb.New(t)
	store := radar.NewStore(pool)
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	reference := radar.NewReference(emb)

	userID := seedUser(t, pool)
	topic, err := store.CreateTopic(ctx, radar.CreateTopicParams{
		UserID: userID, Name: "Rust", Description: "Rust releases", MatchThreshold: 0.18,
	})
	require.NoError(t, err)
	tvec, _ := emb.Embed(ctx, "Rust releases")
	_, err = pool.Exec(ctx, `UPDATE radar_topics SET embedding = $1 WHERE id = $2`, pgvector.NewVector(tvec), topic.ID)
	require.NoError(t, err)

	feed, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: "https://x.example/feed.xml", Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)
	_, err = store.Subscribe(ctx, userID, feed.ID)
	require.NoError(t, err)

	title, ext := "Rust 2.0 released", "g1"
	f, _, err := store.UpsertFinding(ctx, radar.FindingUpsert{
		FeedID: feed.ID, ExternalID: &ext, URL: "https://x.example/post", Title: &title,
	})
	require.NoError(t, err)
	article, _ := emb.Embed(ctx, "Rust 2.0 released\nThe article")
	_, err = pool.Exec(ctx, `UPDATE radar_findings SET embedding = $1 WHERE id = $2`, pgvector.NewVector(article), f.ID)
	require.NoError(t, err)

	backfill := jobs.NewBackfillMatchVectorsWorker(store, reference)
	titleWorker := jobs.NewEmbedFindingTitleWorker(store, emb)
	inserter := newFakeInserter()
	backfill.SetInserter(inserter)
	inserter.dispatch[jobs.EmbedFindingTitleArgs{}.Kind()] = func(ctx context.Context, a river.JobArgs) {
		require.NoError(t, titleWorker.Work(ctx, &river.Job[jobs.EmbedFindingTitleArgs]{Args: a.(jobs.EmbedFindingTitleArgs)}))
	}

	require.NoError(t, backfill.Work(ctx, &river.Job[jobs.BackfillMatchVectorsArgs]{}))

	var topicMatch, findingMatch pgvector.Vector
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT match_embedding FROM radar_topics WHERE id = $1`, topic.ID).Scan(&topicMatch))
	wantTopic, err := reference.TopicMatchVector(ctx, tvec)
	require.NoError(t, err)
	require.InDeltaSlice(t, wantTopic, topicMatch.Slice(), 1e-6)

	require.NoError(t, pool.QueryRow(ctx,
		`SELECT match_embedding FROM radar_findings WHERE id = $1`, f.ID).Scan(&findingMatch))
	titleVec, _ := emb.Embed(ctx, title)
	require.InDeltaSlice(t, radar.FindingMatchVector(article, titleVec), findingMatch.Slice(), 1e-6)

	require.Equal(t, []river.JobArgs{jobs.EmbedFindingTitleArgs{FindingID: f.ID}}, inserter.calls,
		"one title job, and no match: the finding is old news")

	// Nothing left to do on the next run.
	inserter.calls = nil
	require.NoError(t, backfill.Work(ctx, &river.Job[jobs.BackfillMatchVectorsArgs]{}))
	require.Empty(t, inserter.calls)
}

// River validates queue names, worker registrations and periodic jobs when
// the client is built; no database connection is made yet.
func TestNewClient_AcceptsThePipeline(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://nobody@127.0.0.1:1/none")
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	bundle := jobs.Build(jobs.Deps{}, jobs.Config{
		SchedulerInterval: time.Minute, FetchContent: true, ContentRetention: time.Hour,
	})

	_, err = jobs.NewClient(pool, bundle.Workers, bundle.Periodic, jobs.QueueSizes{Default: 5, Fetch: 3, Embed: 2})
	require.NoError(t, err)

	opts := jobs.FetchFindingContentArgs{}.InsertOpts()
	require.Equal(t, jobs.QueueFetch, opts.Queue)
	require.Equal(t, 3, opts.MaxAttempts)

	require.Equal(t, jobs.QueueEmbed, jobs.EmbedFindingArgs{}.InsertOpts().Queue)

	title := jobs.EmbedFindingTitleArgs{}.InsertOpts()
	require.Equal(t, jobs.QueueEmbed, title.Queue)
	require.Equal(t, 4, title.Priority, "the backfill waits behind new findings")
	require.True(t, title.UniqueOpts.ByArgs)
}
