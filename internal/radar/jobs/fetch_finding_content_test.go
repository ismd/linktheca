package jobs_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ismd/linktheca/internal/core/content"
	"github.com/ismd/linktheca/internal/core/netsafe"
	"github.com/ismd/linktheca/internal/radar"
	"github.com/ismd/linktheca/internal/radar/crawler"
	"github.com/ismd/linktheca/internal/radar/jobs"
	"github.com/ismd/linktheca/internal/testing/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/require"
)

// articleText is long enough for readability to treat the page as an article
// and for the worker to accept it.
var articleText = strings.Repeat(
	"Rust 2.0 ships a new borrow checker that accepts more programs while keeping the same guarantees. ", 8)

func articlePage(title, text string) string {
	return `<!DOCTYPE html><html><head><title>` + title + `</title></head><body><article>` +
		`<h1>` + title + `</h1><p>` + text + `</p><p>` + text + `</p></article></body></html>`
}

type page struct {
	status      int
	contentType string
	body        string
}

// site is a fake website: a handful of pages and a robots.txt, counting
// the requests it gets.
type site struct {
	*httptest.Server
	mu    sync.Mutex
	pages map[string]page
	hits  map[string]int
}

func newSite(t *testing.T, pages map[string]page) *site {
	t.Helper()

	s := &site{pages: pages, hits: map[string]int{}}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.hits[r.URL.Path]++
		p, ok := s.pages[r.URL.Path]
		s.mu.Unlock()

		if !ok {
			http.NotFound(w, r)
			return
		}
		if p.contentType != "" {
			w.Header().Set("Content-Type", p.contentType)
		}
		w.WriteHeader(p.status)
		_, _ = w.Write([]byte(p.body))
	}))
	t.Cleanup(s.Close)

	return s
}

func (s *site) hitCount(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.hits[path]
}

func htmlPage(body string) page {
	return page{status: http.StatusOK, contentType: "text/html; charset=utf-8", body: body}
}

type fetchFixture struct {
	pool     *pgxpool.Pool
	store    *radar.Store
	contents *content.Store
	feedID   int64
	inserter *fakeInserter
}

func newFetchFixture(t *testing.T) *fetchFixture {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := radar.NewStore(pool)

	feed, err := store.AddFeed(context.Background(), radar.AddFeedParams{
		URL: "https://feeds.example/rss", Kind: "rss", FetchIntervalSeconds: 3600,
	})
	require.NoError(t, err)

	return &fetchFixture{
		pool:     pool,
		store:    store,
		contents: content.NewStore(pool),
		feedID:   feed.ID,
		inserter: newFakeInserter(),
	}
}

func (fx *fetchFixture) finding(t *testing.T, url string) int64 {
	t.Helper()

	title, ext := "Rust 2.0 released", url
	f, created, err := fx.store.UpsertFinding(context.Background(), radar.FindingUpsert{
		FeedID: fx.feedID, ExternalID: &ext, URL: url, Title: &title,
	})
	require.NoError(t, err)
	require.True(t, created)

	return f.ID
}

// worker builds the worker under test. allowPrivate is what lets it reach the
// httptest sites at all.
func (fx *fetchFixture) worker(allowPrivate bool, pacer jobs.Pacer) *jobs.FetchFindingContentWorker {
	client := netsafe.NewClient(netsafe.Options{Timeout: 5 * time.Second, AllowPrivate: allowPrivate})
	if pacer == nil {
		pacer = crawler.NewHostPacer(0)
	}

	w := jobs.NewFetchFindingContentWorker(fx.store, fx.contents,
		content.NewExtractor(client), crawler.NewRobots(client), pacer)
	w.SetInserter(fx.inserter)

	return w
}

func fetchJob(id int64, attempt int, args jobs.FetchFindingContentArgs) *river.Job[jobs.FetchFindingContentArgs] {
	return &river.Job[jobs.FetchFindingContentArgs]{
		JobRow: &rivertype.JobRow{ID: id, Attempt: attempt, MaxAttempts: 3},
		Args:   args,
	}
}

func (fx *fetchFixture) contentID(t *testing.T, findingID int64) *int64 {
	t.Helper()

	var id *int64
	require.NoError(t, fx.pool.QueryRow(context.Background(),
		`SELECT content_id FROM radar_findings WHERE id = $1`, findingID).Scan(&id))

	return id
}

func (fx *fetchFixture) contentRows(t *testing.T) int {
	t.Helper()

	var n int
	require.NoError(t, fx.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM article_contents`).Scan(&n))

	return n
}

func (fx *fetchFixture) requireEmbedQueued(t *testing.T, want jobs.EmbedFindingArgs) {
	t.Helper()

	require.Len(t, fx.inserter.calls, 1)
	require.Equal(t, want, fx.inserter.calls[0])
}

func TestFetchFindingContent_StoresPageAndQueuesEmbed(t *testing.T) {
	fx := newFetchFixture(t)
	s := newSite(t, map[string]page{"/post": htmlPage(articlePage("Rust 2.0", articleText))})
	id := fx.finding(t, s.URL+"/post")

	err := fx.worker(true, nil).Work(context.Background(), fetchJob(1, 1, jobs.FetchFindingContentArgs{FindingID: id}))
	require.NoError(t, err)

	cid := fx.contentID(t, id)
	require.NotNil(t, cid)

	rec, err := fx.contents.GetByURL(context.Background(), s.URL+"/post")
	require.NoError(t, err)
	require.Equal(t, *cid, rec.ID)
	require.Contains(t, *rec.Text, "new borrow checker")
	require.NotNil(t, rec.HTML)
	require.Nil(t, rec.Image, "Radar does not download images")

	require.Equal(t, 1, s.hitCount("/robots.txt"))
	fx.requireEmbedQueued(t, jobs.EmbedFindingArgs{FindingID: id})
}

func TestFetchFindingContent_PermanentFailureFallsBackAtOnce(t *testing.T) {
	tests := map[string]page{
		"not found":    {status: http.StatusNotFound},
		"pdf":          {status: http.StatusOK, contentType: "application/pdf", body: "%PDF-1.7"},
		"short page":   htmlPage(articlePage("Paywall", "Subscribe to read this article.")),
		"forbidden":    {status: http.StatusForbidden},
		"empty answer": htmlPage(""),
	}

	for name, p := range tests {
		t.Run(name, func(t *testing.T) {
			fx := newFetchFixture(t)
			s := newSite(t, map[string]page{"/post": p})
			id := fx.finding(t, s.URL+"/post")

			err := fx.worker(true, nil).Work(context.Background(), fetchJob(1, 1, jobs.FetchFindingContentArgs{FindingID: id}))
			require.NoError(t, err)

			require.Nil(t, fx.contentID(t, id))
			require.Zero(t, fx.contentRows(t), "failed fetches are not stored")
			fx.requireEmbedQueued(t, jobs.EmbedFindingArgs{FindingID: id})
		})
	}
}

func TestFetchFindingContent_TransientFailureRetriesThenFallsBack(t *testing.T) {
	fx := newFetchFixture(t)
	s := newSite(t, map[string]page{"/post": {status: http.StatusServiceUnavailable}})
	id := fx.finding(t, s.URL+"/post")
	w := fx.worker(true, nil)
	args := jobs.FetchFindingContentArgs{FindingID: id}

	require.Error(t, w.Work(context.Background(), fetchJob(1, 1, args)))
	require.Error(t, w.Work(context.Background(), fetchJob(1, 2, args)))
	require.Empty(t, fx.inserter.calls, "no embed while retries remain")

	require.NoError(t, w.Work(context.Background(), fetchJob(1, 3, args)))
	require.Nil(t, fx.contentID(t, id))
	fx.requireEmbedQueued(t, jobs.EmbedFindingArgs{FindingID: id})
	require.Equal(t, 3, s.hitCount("/post"))
}

func TestFetchFindingContent_RobotsDisallowSkipsThePage(t *testing.T) {
	fx := newFetchFixture(t)
	s := newSite(t, map[string]page{
		"/robots.txt": {status: http.StatusOK, body: "User-agent: Linktheca\nDisallow: /\n"},
		"/post":       htmlPage(articlePage("Rust 2.0", articleText)),
	})
	id := fx.finding(t, s.URL+"/post")

	err := fx.worker(true, nil).Work(context.Background(), fetchJob(1, 1, jobs.FetchFindingContentArgs{FindingID: id}))
	require.NoError(t, err)

	require.Zero(t, s.hitCount("/post"))
	require.Nil(t, fx.contentID(t, id))
	fx.requireEmbedQueued(t, jobs.EmbedFindingArgs{FindingID: id})
}

func TestFetchFindingContent_RobotsServerErrorIsRetried(t *testing.T) {
	fx := newFetchFixture(t)
	s := newSite(t, map[string]page{
		"/robots.txt": {status: http.StatusInternalServerError},
		"/post":       htmlPage(articlePage("Rust 2.0", articleText)),
	})
	id := fx.finding(t, s.URL+"/post")

	err := fx.worker(true, nil).Work(context.Background(), fetchJob(1, 1, jobs.FetchFindingContentArgs{FindingID: id}))

	require.Error(t, err)
	require.Zero(t, s.hitCount("/post"))
	require.Empty(t, fx.inserter.calls)
}

// The production client refuses loopback. That is not worth retrying.
func TestFetchFindingContent_PrivateAddressFallsBackAtOnce(t *testing.T) {
	fx := newFetchFixture(t)
	s := newSite(t, map[string]page{"/post": htmlPage(articlePage("Rust 2.0", articleText))})
	id := fx.finding(t, s.URL+"/post")

	err := fx.worker(false, nil).Work(context.Background(), fetchJob(1, 1, jobs.FetchFindingContentArgs{FindingID: id}))
	require.NoError(t, err)

	require.Zero(t, s.hitCount("/robots.txt"))
	require.Zero(t, s.hitCount("/post"))
	fx.requireEmbedQueued(t, jobs.EmbedFindingArgs{FindingID: id})
}

// A page another feed or the Library already fetched is linked, not fetched
// again.
func TestFetchFindingContent_ReusesAStoredPage(t *testing.T) {
	fx := newFetchFixture(t)
	s := newSite(t, map[string]page{})
	url := s.URL + "/post"
	id := fx.finding(t, url)

	rec, err := fx.contents.Upsert(context.Background(), content.UpsertParams{URL: url, Text: &articleText})
	require.NoError(t, err)

	err = fx.worker(true, nil).Work(context.Background(), fetchJob(1, 1, jobs.FetchFindingContentArgs{FindingID: id}))
	require.NoError(t, err)

	require.Equal(t, &rec.ID, fx.contentID(t, id))
	require.Zero(t, s.hitCount("/robots.txt"))
	require.Zero(t, s.hitCount("/post"))
	fx.requireEmbedQueued(t, jobs.EmbedFindingArgs{FindingID: id})
}

func TestFetchFindingContent_SnoozesWhileTheHostIsBusy(t *testing.T) {
	fx := newFetchFixture(t)
	s := newSite(t, map[string]page{
		"/a": htmlPage(articlePage("A", articleText)),
		"/b": htmlPage(articlePage("B", articleText)),
	})
	first, second := fx.finding(t, s.URL+"/a"), fx.finding(t, s.URL+"/b")
	w := fx.worker(true, crawler.NewHostPacer(time.Hour))

	require.NoError(t, w.Work(context.Background(), fetchJob(1, 1, jobs.FetchFindingContentArgs{FindingID: first})))

	err := w.Work(context.Background(), fetchJob(2, 1, jobs.FetchFindingContentArgs{FindingID: second}))

	var snooze *river.JobSnoozeError
	require.ErrorAs(t, err, &snooze)
	require.InDelta(t, time.Hour, snooze.Duration, float64(time.Second))
	require.Zero(t, s.hitCount("/b"))
	require.Len(t, fx.inserter.calls, 1, "only the first finding moved on")
}

func TestFetchFindingContent_PassesBackfillFlagsOn(t *testing.T) {
	fx := newFetchFixture(t)
	s := newSite(t, map[string]page{"/post": htmlPage(articlePage("Rust 2.0", articleText))})
	id := fx.finding(t, s.URL+"/post")
	args := jobs.FetchFindingContentArgs{FindingID: id, Reembed: true, SkipMatch: true}

	require.NoError(t, fx.worker(true, nil).Work(context.Background(), fetchJob(1, 1, args)))

	fx.requireEmbedQueued(t, jobs.EmbedFindingArgs{FindingID: id, Reembed: true, SkipMatch: true})
}

func TestFetchFindingContent_FindingAlreadyHasContent(t *testing.T) {
	fx := newFetchFixture(t)
	s := newSite(t, map[string]page{"/post": htmlPage(articlePage("Rust 2.0", articleText))})
	id := fx.finding(t, s.URL+"/post")
	w := fx.worker(true, nil)

	require.NoError(t, w.Work(context.Background(), fetchJob(1, 1, jobs.FetchFindingContentArgs{FindingID: id})))
	require.NoError(t, w.Work(context.Background(), fetchJob(2, 1, jobs.FetchFindingContentArgs{FindingID: id})))

	require.Equal(t, 1, s.hitCount("/post"))
	require.Len(t, fx.inserter.calls, 2)
}

func TestFetchFindingContent_DeletedFindingIsDone(t *testing.T) {
	fx := newFetchFixture(t)

	err := fx.worker(true, nil).Work(context.Background(), fetchJob(1, 1, jobs.FetchFindingContentArgs{FindingID: 424242}))

	require.NoError(t, err)
	require.Empty(t, fx.inserter.calls)
}
