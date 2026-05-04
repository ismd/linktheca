// Package jobs hosts the four River workers that drive the Radar pipeline:
// ScheduleCrawls (periodic) → CrawlFeed → EmbedFinding → MatchFinding.
package jobs

import (
	"context"

	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/ismd/linktheca/internal/radar"
	"github.com/ismd/linktheca/internal/radar/crawler"
	"github.com/riverqueue/river"
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
