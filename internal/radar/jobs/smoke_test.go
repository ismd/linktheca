//go:build smoke

package jobs_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/ismd/linktheca/internal/radar"
	"github.com/ismd/linktheca/internal/radar/crawler"
	"github.com/ismd/linktheca/internal/radar/jobs"
	"github.com/ismd/linktheca/internal/testing/testdb"
	"github.com/pgvector/pgvector-go"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestSmoke_PipelineWithRealTEI runs a TEI container, serves a synthetic RSS
// feed, and walks scheduler → crawl → embed → match using the real embedder.
// Verifies that bge-m3 produces 1024-dim vectors and that a topic similar to
// the finding gets matched.
func TestSmoke_PipelineWithRealTEI(t *testing.T) {
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "ghcr.io/huggingface/text-embeddings-inference:cpu-1.9",
		Cmd:          []string{"--model-id", "BAAI/bge-m3", "--port", "8080"},
		ExposedPorts: []string{"8080/tcp"},
		WaitingFor:   wait.ForHTTP("/health").WithPort("8080/tcp").WithStartupTimeout(10 * time.Minute),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req, Started: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(context.Background()) })

	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "8080/tcp")
	require.NoError(t, err)

	teiClient := embeddings.NewTEIClient("http://"+host+":"+port.Port(), 60*time.Second)

	// Synthetic RSS server.
	rss := `<?xml version="1.0"?>
<rss version="2.0"><channel>
<item><title>OpenAI ships GPT</title>
<link>https://news.example/post/1</link>
<description>Recent advances in transformer architectures and inference.</description>
<guid>g1</guid></item></channel></rss>`
	rssSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(rss))
	}))
	defer rssSrv.Close()

	pool := testdb.New(t)
	store := radar.NewStore(pool)

	userID := seedUser(t, pool)
	feed, err := store.AddFeed(ctx, radar.AddFeedParams{
		URL: rssSrv.URL, Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)
	_, err = store.Subscribe(ctx, userID, feed.ID)
	require.NoError(t, err)

	topic, err := store.CreateTopic(ctx, radar.CreateTopicParams{
		UserID: userID, Name: "AI", Description: "transformers and large language models", MatchThreshold: 0.0,
	})
	require.NoError(t, err)
	tvec, err := teiClient.Embed(ctx, "transformers and large language models")
	require.NoError(t, err)
	require.Len(t, tvec, 1024)
	require.NoError(t, store.UpdateTopicEmbedding(ctx, topic.ID, pgvector.NewVector(tvec)))

	scheduler := jobs.NewScheduleCrawlsWorker(store)
	crawl := jobs.NewCrawlFeedWorker(store, crawler.NewHTTPFetcher(30*time.Second))
	embed := jobs.NewEmbedFindingWorker(store, teiClient)
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

	require.NoError(t, scheduler.Work(ctx, &river.Job[jobs.ScheduleCrawlsArgs]{}))

	var n int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM radar_topic_matches WHERE topic_id=$1`, topic.ID).Scan(&n))
	require.GreaterOrEqual(t, n, 1, "expected at least one match")
}
