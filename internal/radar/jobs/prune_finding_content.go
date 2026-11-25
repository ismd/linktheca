package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ismd/linktheca/internal/radar"
	"github.com/riverqueue/river"
)

// PruneFindingContentWorker deletes downloaded pages of findings that matched
// no topic once they are older than the retention period. Radar downloads
// every link of every subscribed feed, and almost all of them match nothing.
type PruneFindingContentWorker struct {
	river.WorkerDefaults[PruneFindingContentArgs]
	store     *radar.Store
	retention time.Duration
	now       func() time.Time
}

func NewPruneFindingContentWorker(store *radar.Store, retention time.Duration) *PruneFindingContentWorker {
	return &PruneFindingContentWorker{store: store, retention: retention, now: time.Now}
}

func (w *PruneFindingContentWorker) Work(ctx context.Context, _ *river.Job[PruneFindingContentArgs]) error {
	n, err := w.store.PruneFindingContent(ctx, w.now().Add(-w.retention))
	if err != nil {
		return fmt.Errorf("prune finding content: %w", err)
	}

	if n > 0 {
		slog.InfoContext(ctx, "radar: pruned downloaded pages", "count", n)
	}

	return nil
}
