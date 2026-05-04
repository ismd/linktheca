// Package jobs hosts the four River workers that drive the Radar pipeline:
// ScheduleCrawls (periodic) → CrawlFeed → EmbedFinding → MatchFinding.
package jobs

import (
	"context"
	"time"

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

type EmbedFindingArgs struct {
	FindingID int64 `json:"finding_id"`
}

func (EmbedFindingArgs) Kind() string { return "radar.embed_finding" }

type MatchFindingArgs struct {
	FindingID int64 `json:"finding_id"`
}

func (MatchFindingArgs) Kind() string { return "radar.match_finding" }

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
}

// Bundle is what Build returns: ready-to-mount workers, the periodic-job
// spec, and a setter that the caller invokes after constructing the River
// client so workers can enqueue downstream jobs.
type Bundle struct {
	Workers      *river.Workers
	Periodic     []*river.PeriodicJob
	WireInserter func(Inserter)
}

func Build(d Deps, schedulerInterval time.Duration) Bundle {
	scheduler := NewScheduleCrawlsWorker(d.Store)
	crawl := NewCrawlFeedWorker(d.Store, d.Fetcher)
	embed := NewEmbedFindingWorker(d.Store, d.Embedder)
	match := NewMatchFindingWorker(d.Store)

	workers := river.NewWorkers()
	river.AddWorker(workers, scheduler)
	river.AddWorker(workers, crawl)
	river.AddWorker(workers, embed)
	river.AddWorker(workers, match)

	periodic := []*river.PeriodicJob{
		river.NewPeriodicJob(
			river.PeriodicInterval(schedulerInterval),
			func() (river.JobArgs, *river.InsertOpts) {
				return ScheduleCrawlsArgs{}, nil
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
			embed.SetInserter(i)
		},
	}
}

// NewClient is a thin convenience over river.NewClient for callers that just
// want sensible defaults.
func NewClient(pool *pgxpool.Pool, workers *river.Workers, periodic []*river.PeriodicJob, maxWorkers int) (*river.Client[pgx.Tx], error) {
	return river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:       map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: maxWorkers}},
		Workers:      workers,
		PeriodicJobs: periodic,
	})
}
