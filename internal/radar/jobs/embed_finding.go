package jobs

import (
	"context"
	"fmt"
	"strings"

	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/ismd/linktheca/internal/radar"
	"github.com/pgvector/pgvector-go"
	"github.com/riverqueue/river"
)

type EmbedFindingWorker struct {
	river.WorkerDefaults[EmbedFindingArgs]
	store    *radar.Store
	embedder embeddings.Client
	inserter Inserter
}

func NewEmbedFindingWorker(store *radar.Store, embedder embeddings.Client) *EmbedFindingWorker {
	return &EmbedFindingWorker{store: store, embedder: embedder}
}

func (w *EmbedFindingWorker) SetInserter(i Inserter) { w.inserter = i }

func (w *EmbedFindingWorker) Work(ctx context.Context, job *river.Job[EmbedFindingArgs]) error {
	id := job.Args.FindingID

	f, err := w.store.GetFindingForEmbed(ctx, id)
	if err != nil {
		return fmt.Errorf("get finding %d: %w", id, err)
	}
	if f.HasEmbedding {
		return nil
	}

	text := embedText(f)
	if text == "" {
		return nil
	}

	vec, err := w.embedder.Embed(ctx, text)
	if err != nil {
		return fmt.Errorf("embed finding %d: %w", id, err)
	}

	if err := w.store.UpdateFindingEmbedding(ctx, id, pgvector.NewVector(vec)); err != nil {
		return fmt.Errorf("save embedding %d: %w", id, err)
	}

	if w.inserter != nil {
		if _, err := w.inserter.Insert(ctx, MatchFindingArgs{FindingID: id}, nil); err != nil {
			return fmt.Errorf("enqueue match for finding %d: %w", id, err)
		}
	}

	return nil
}

func embedText(f *radar.FindingForEmbed) string {
	parts := []string{}

	if f.Title != nil {
		parts = append(parts, strings.TrimSpace(*f.Title))
	}

	if f.Summary != nil {
		parts = append(parts, strings.TrimSpace(*f.Summary))
	}

	return strings.TrimSpace(strings.Join(parts, "\n"))
}
