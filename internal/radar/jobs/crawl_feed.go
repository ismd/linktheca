package jobs

import (
	"context"
	"fmt"

	"github.com/ismd/linktheca/internal/radar"
	"github.com/ismd/linktheca/internal/radar/crawler"
	"github.com/riverqueue/river"
)

type CrawlFeedWorker struct {
	river.WorkerDefaults[CrawlFeedArgs]
	store    *radar.Store
	fetcher  crawler.Fetcher
	inserter Inserter // set after River client is built; see SetInserter
}

func NewCrawlFeedWorker(store *radar.Store, fetcher crawler.Fetcher) *CrawlFeedWorker {
	return &CrawlFeedWorker{store: store, fetcher: fetcher}
}

func (w *CrawlFeedWorker) SetInserter(i Inserter) { w.inserter = i }

func (w *CrawlFeedWorker) Work(ctx context.Context, job *river.Job[CrawlFeedArgs]) error {
	feedID := job.Args.FeedID

	state, err := w.store.GetFeedForFetch(ctx, feedID)
	if err != nil {
		return fmt.Errorf("get feed %d: %w", feedID, err)
	}

	etag, lastMod := "", ""
	if state.Etag != nil {
		etag = *state.Etag
	}
	if state.LastModified != nil {
		lastMod = *state.LastModified
	}

	res, err := w.fetcher.Fetch(ctx, state.URL, etag, lastMod)
	if err != nil {
		_ = w.store.MarkFeedError(ctx, feedID, err.Error())
		return fmt.Errorf("fetch feed %d: %w", feedID, err)
	}
	if res.NotModified {
		return w.store.MarkFeedFetched(ctx, feedID, ptrOrNil(res.Etag), ptrOrNil(res.LastModified), nil)
	}

	parsed, err := crawler.Parse(res.Body)
	if err != nil {
		_ = w.store.MarkFeedError(ctx, feedID, err.Error())
		return fmt.Errorf("parse feed %d: %w", feedID, err)
	}

	for _, up := range crawler.ToUpserts(feedID, parsed.Items) {
		f, created, err := w.store.UpsertFinding(ctx, up)

		if err != nil {
			return fmt.Errorf("upsert finding for feed %d: %w", feedID, err)
		}

		if !created {
			continue
		}

		if w.inserter != nil {
			if _, err := w.inserter.Insert(ctx, EmbedFindingArgs{FindingID: f.ID}, nil); err != nil {
				return fmt.Errorf("enqueue embed for finding %d: %w", f.ID, err)
			}
		}
	}

	return w.store.MarkFeedFetched(ctx, feedID,
		ptrOrNil(res.Etag), ptrOrNil(res.LastModified), ptrOrNil(parsed.Title))
}

func ptrOrNil(s string) *string {
	if s == "" {
		return nil
	}

	return &s
}
