package jobs

import (
	"context"
	"fmt"

	"github.com/ismd/linktheca/internal/radar"
	"github.com/riverqueue/river"
)

type MatchFindingWorker struct {
	river.WorkerDefaults[MatchFindingArgs]
	store *radar.Store
}

func NewMatchFindingWorker(store *radar.Store) *MatchFindingWorker {
	return &MatchFindingWorker{store: store}
}

func (w *MatchFindingWorker) Work(ctx context.Context, job *river.Job[MatchFindingArgs]) error {
	if _, err := w.store.MatchFindingToTopics(ctx, job.Args.FindingID); err != nil {
		return fmt.Errorf("match finding %d: %w", job.Args.FindingID, err)
	}

	return nil
}

// Compile-time check that radar.Store satisfies what MatchFindingWorker needs.
var _ matchStore = (*radar.Store)(nil)

type matchStore interface {
	MatchFindingToTopics(ctx context.Context, findingID int64) (int64, error)
}
