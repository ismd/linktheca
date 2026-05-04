package jobs

import (
	"context"
	"fmt"

	"github.com/ismd/linktheca/internal/radar"
	"github.com/riverqueue/river"
)

const scheduleCrawlsBatchSize = 100

type ScheduleCrawlsWorker struct {
	river.WorkerDefaults[ScheduleCrawlsArgs]
	store    *radar.Store
	inserter Inserter
}

func NewScheduleCrawlsWorker(store *radar.Store) *ScheduleCrawlsWorker {
	return &ScheduleCrawlsWorker{store: store}
}

func (w *ScheduleCrawlsWorker) SetInserter(i Inserter) { w.inserter = i }

func (w *ScheduleCrawlsWorker) Work(ctx context.Context, _ *river.Job[ScheduleCrawlsArgs]) error {
	ids, err := w.store.ListDueFeeds(ctx, scheduleCrawlsBatchSize)
	if err != nil {
		return fmt.Errorf("list due feeds: %w", err)
	}

	if w.inserter == nil {
		return nil
	}

	for _, id := range ids {
		if _, err := w.inserter.Insert(ctx, CrawlFeedArgs{FeedID: id}, nil); err != nil {
			return fmt.Errorf("enqueue crawl_feed for feed %d: %w", id, err)
		}
	}

	return nil
}
