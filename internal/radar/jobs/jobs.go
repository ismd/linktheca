// Package jobs hosts the River workers that drive the Radar pipeline:
//
//	ScheduleCrawls (periodic) → CrawlFeed → FetchFindingContent → EmbedFinding → MatchFinding
//
// plus PruneFindingContent (periodic), which drops downloaded pages nobody
// needs any more, and BackfillMatchVectors (periodic) → EmbedFindingTitle,
// which completes rows embedded before match vectors existed.
// FetchFindingContent is skipped when page downloads are off.
package jobs

import (
	"context"
	"time"

	"github.com/ismd/linktheca/internal/core/content"
	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/ismd/linktheca/internal/radar"
	"github.com/ismd/linktheca/internal/radar/crawler"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
)

// --- Args (one type per worker; River uses Kind() for routing) ---

type ScheduleCrawlsArgs struct{}

func (ScheduleCrawlsArgs) Kind() string { return "radar.schedule_crawls" }

type CrawlFeedArgs struct {
	FeedID int64 `json:"feed_id"`
}

func (CrawlFeedArgs) Kind() string { return "radar.crawl_feed" }

const (
	// QueueFetch is the queue page downloads run in. It has its own
	// workers, so slow sites never hold up crawling, embedding and matching.
	QueueFetch = "radar_fetch"

	// QueueEmbed is the queue embeddings are computed in. On CPU a finding
	// takes TEI a second or two and TEI works through one or two at a time,
	// so more parallel requests only wait in TEI's queue until the client
	// times out. A small queue of its own keeps them flowing and keeps
	// crawling and matching from waiting behind them.
	QueueEmbed = "radar_embed"
)

// FetchFindingContentArgs asks for the page behind a finding to be
// downloaded. Reembed and SkipMatch are passed on to EmbedFindingArgs.
type FetchFindingContentArgs struct {
	FindingID int64 `json:"finding_id"`
	Reembed   bool  `json:"reembed,omitempty"`
	SkipMatch bool  `json:"skip_match,omitempty"`
}

func (FetchFindingContentArgs) Kind() string { return "radar.fetch_finding_content" }

// InsertOpts gives a failing download two more tries, after which the finding
// is embedded without its page.
func (FetchFindingContentArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueFetch, MaxAttempts: 3}
}

type EmbedFindingArgs struct {
	FindingID int64 `json:"finding_id"`
	// Reembed replaces an existing embedding instead of keeping it, for
	// findings whose page was downloaded after they were embedded.
	Reembed bool `json:"reembed,omitempty"`
	// SkipMatch embeds without matching, so re-embedding old findings does
	// not flood inboxes with matches from weeks ago.
	SkipMatch bool `json:"skip_match,omitempty"`
}

func (EmbedFindingArgs) Kind() string { return "radar.embed_finding" }

func (EmbedFindingArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: QueueEmbed}
}

type MatchFindingArgs struct {
	FindingID int64 `json:"finding_id"`
}

func (MatchFindingArgs) Kind() string { return "radar.match_finding" }

type PruneFindingContentArgs struct{}

func (PruneFindingContentArgs) Kind() string { return "radar.prune_finding_content" }

// BackfillMatchVectorsArgs fills in the match vectors of topics and findings
// embedded before match vectors existed.
type BackfillMatchVectorsArgs struct{}

func (BackfillMatchVectorsArgs) Kind() string { return "radar.backfill_match_vectors" }

// EmbedFindingTitleArgs completes the match vector of an already embedded
// finding by embedding its title. Backfill only: the finding is not matched
// again, its topics have seen it.
type EmbedFindingTitleArgs struct {
	FindingID int64 `json:"finding_id"`
}

func (EmbedFindingTitleArgs) Kind() string { return "radar.embed_finding_title" }

// InsertOpts puts the backfill behind new findings in the embed queue, and
// keeps a finding from being queued twice while its job is pending.
func (EmbedFindingTitleArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:      QueueEmbed,
		Priority:   4,
		UniqueOpts: river.UniqueOpts{ByArgs: true},
	}
}

// Inserter is the slice of river.Client that workers actually need.
// Defining an interface lets jobs_test.go run workers without a real client.
type Inserter interface {
	Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// Deps groups everything passed into Build so callers don't drift.
type Deps struct {
	Store    *radar.Store
	Embedder embeddings.Client
	Fetcher  crawler.Fetcher
	// Reference is shared with the radar service, so the reference centroid
	// is embedded once per process.
	Reference *radar.Reference

	// Page downloads.
	Contents  *content.Store
	Extractor content.Extractor
	Robots    RobotsChecker
	Pacer     Pacer
}

// Config tunes the pipeline.
type Config struct {
	SchedulerInterval time.Duration
	// FetchContent downloads the page behind each finding before embedding
	// it. Off, findings are embedded from the feed's title and summary.
	FetchContent bool
	// ContentRetention is how long pages of findings that matched nothing
	// are kept.
	ContentRetention time.Duration
}

// pruneInterval is how often downloaded pages past retention are deleted.
const pruneInterval = 24 * time.Hour

// backfillInterval is how often rows without match vectors are looked for.
// Each run queues at most backfillBatch title embeddings.
const backfillInterval = 10 * time.Minute

// Bundle is what Build returns: ready-to-mount workers, the periodic-job
// spec, and a setter that the caller invokes after constructing the River
// client so workers can enqueue downstream jobs.
type Bundle struct {
	Workers      *river.Workers
	Periodic     []*river.PeriodicJob
	WireInserter func(Inserter)
}

func Build(d Deps, cfg Config) Bundle {
	scheduler := NewScheduleCrawlsWorker(d.Store)
	crawl := NewCrawlFeedWorker(d.Store, d.Fetcher, cfg.FetchContent)
	fetch := NewFetchFindingContentWorker(d.Store, d.Contents, d.Extractor, d.Robots, d.Pacer)
	embed := NewEmbedFindingWorker(d.Store, d.Embedder)
	match := NewMatchFindingWorker(d.Store)
	prune := NewPruneFindingContentWorker(d.Store, cfg.ContentRetention)
	backfill := NewBackfillMatchVectorsWorker(d.Store, d.Reference)
	title := NewEmbedFindingTitleWorker(d.Store, d.Embedder)

	workers := river.NewWorkers()
	river.AddWorker(workers, scheduler)
	river.AddWorker(workers, crawl)
	// Registered even with downloads off, so jobs queued before they were
	// turned off still finish.
	river.AddWorker(workers, fetch)
	river.AddWorker(workers, embed)
	river.AddWorker(workers, match)
	river.AddWorker(workers, prune)
	river.AddWorker(workers, backfill)
	river.AddWorker(workers, title)

	periodic := []*river.PeriodicJob{
		river.NewPeriodicJob(
			river.PeriodicInterval(cfg.SchedulerInterval),
			func() (river.JobArgs, *river.InsertOpts) {
				return ScheduleCrawlsArgs{}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		),
		river.NewPeriodicJob(
			river.PeriodicInterval(pruneInterval),
			func() (river.JobArgs, *river.InsertOpts) {
				return PruneFindingContentArgs{}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		),
		river.NewPeriodicJob(
			river.PeriodicInterval(backfillInterval),
			func() (river.JobArgs, *river.InsertOpts) {
				return BackfillMatchVectorsArgs{}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: true},
		),
	}

	return Bundle{
		Workers:  workers,
		Periodic: periodic,
		WireInserter: func(i Inserter) {
			scheduler.SetInserter(i)
			crawl.SetInserter(i)
			fetch.SetInserter(i)
			embed.SetInserter(i)
			backfill.SetInserter(i)
		},
	}
}

// QueueSizes is how many jobs of each queue run at once.
type QueueSizes struct {
	Default int // crawling, matching, housekeeping
	Fetch   int // page downloads
	Embed   int // TEI requests
}

// NewClient is a thin convenience over river.NewClient for callers that just
// want sensible defaults.
func NewClient(
	pool *pgxpool.Pool,
	workers *river.Workers,
	periodic []*river.PeriodicJob,
	sizes QueueSizes,
) (*river.Client[pgx.Tx], error) {
	return river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: sizes.Default},
			QueueFetch:         {MaxWorkers: sizes.Fetch},
			QueueEmbed:         {MaxWorkers: sizes.Embed},
		},
		Workers:      workers,
		PeriodicJobs: periodic,
	})
}
