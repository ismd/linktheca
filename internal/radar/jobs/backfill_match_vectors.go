package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/ismd/linktheca/internal/radar"
	"github.com/pgvector/pgvector-go"
	"github.com/riverqueue/river"
)

// backfillBatch caps the title embeddings one backfill run queues. On CPU a
// title takes TEI about a tenth of a second, so a batch drains well within
// backfillInterval while new findings go first.
const backfillBatch = 1000

// BackfillMatchVectorsWorker completes topics and findings embedded before
// match vectors existed. A topic's match vector follows from its stored
// embedding and the reference centroid, so topics are filled in place. A
// finding's needs its title embedded, so findings get an EmbedFindingTitle
// job each, newest first. Until then they take no part in matching or
// previews.
type BackfillMatchVectorsWorker struct {
	river.WorkerDefaults[BackfillMatchVectorsArgs]
	store     *radar.Store
	reference *radar.Reference
	inserter  Inserter
}

func NewBackfillMatchVectorsWorker(store *radar.Store, reference *radar.Reference) *BackfillMatchVectorsWorker {
	return &BackfillMatchVectorsWorker{store: store, reference: reference}
}

func (w *BackfillMatchVectorsWorker) SetInserter(i Inserter) { w.inserter = i }

func (w *BackfillMatchVectorsWorker) Work(ctx context.Context, _ *river.Job[BackfillMatchVectorsArgs]) error {
	centroid, err := w.reference.Centroid(ctx)
	if err != nil {
		return fmt.Errorf("reference centroid: %w", err)
	}

	n, err := w.store.FillTopicMatchEmbeddings(ctx, pgvector.NewVector(centroid))
	if err != nil {
		return err
	}
	if n > 0 {
		slog.InfoContext(ctx, "radar: filled topic match vectors", "count", n)
	}

	if w.inserter == nil {
		return nil
	}

	ids, err := w.store.ListFindingsWithoutMatchEmbedding(ctx, backfillBatch)
	if err != nil {
		return err
	}

	for _, id := range ids {
		if _, err := w.inserter.Insert(ctx, EmbedFindingTitleArgs{FindingID: id}, nil); err != nil {
			return fmt.Errorf("enqueue title embedding for finding %d: %w", id, err)
		}
	}
	if len(ids) > 0 {
		slog.InfoContext(ctx, "radar: queued findings for match vectors", "count", len(ids))
	}

	return nil
}

// EmbedFindingTitleWorker embeds a finding's title and stores the match
// vector it makes with the article embedding already stored.
type EmbedFindingTitleWorker struct {
	river.WorkerDefaults[EmbedFindingTitleArgs]
	store    *radar.Store
	embedder embeddings.Client
}

func NewEmbedFindingTitleWorker(store *radar.Store, embedder embeddings.Client) *EmbedFindingTitleWorker {
	return &EmbedFindingTitleWorker{store: store, embedder: embedder}
}

func (w *EmbedFindingTitleWorker) Work(ctx context.Context, job *river.Job[EmbedFindingTitleArgs]) error {
	id := job.Args.FindingID

	f, err := w.store.GetFindingForTitleEmbed(ctx, id)
	if errors.Is(err, radar.ErrNotFound) {
		return nil // deleted with its feed since it was queued
	}
	if err != nil {
		return fmt.Errorf("get finding %d: %w", id, err)
	}
	if f.HasMatchEmbedding || f.Embedding == nil {
		return nil
	}

	// A finding embedded from its title alone gets the same vector back,
	// which is what EmbedFindingWorker would have used.
	article := f.Embedding.Slice()
	titleVec := article
	if title := titleText(f.Title); title != "" {
		if titleVec, err = w.embedder.Embed(ctx, title); err != nil {
			return fmt.Errorf("embed title of finding %d: %w", id, err)
		}
	}

	matchVec := radar.FindingMatchVector(article, titleVec)
	return w.store.FillFindingMatchEmbedding(ctx, id, pgvector.NewVector(matchVec))
}
