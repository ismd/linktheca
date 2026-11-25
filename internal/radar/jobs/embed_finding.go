package jobs

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

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
	if f.HasEmbedding && !job.Args.Reembed {
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

	if w.inserter != nil && !job.Args.SkipMatch {
		if _, err := w.inserter.Insert(ctx, MatchFindingArgs{FindingID: id}, nil); err != nil {
			return fmt.Errorf("enqueue match for finding %d: %w", id, err)
		}
	}

	return nil
}

// maxEmbedTextRunes caps the article text a finding is embedded from: about
// 450 tokens, the opening of the article, which is where it says what it is
// about. The cost is what decides it. On CPU, bge-m3 takes ~1.7 s for 2000
// characters and ~4.5 s for the 1024 tokens TEI would truncate to, and every
// new finding goes through it.
const maxEmbedTextRunes = 2000

// embedText is what a finding is embedded from: its title followed by the
// article text when the page was downloaded, or by the feed's summary when it
// was not.
func embedText(f *radar.FindingForEmbed) string {
	parts := []string{}

	if f.Title != nil {
		parts = append(parts, strings.TrimSpace(*f.Title))
	}

	switch {
	case f.Text != nil && strings.TrimSpace(*f.Text) != "":
		parts = append(parts, truncateRunes(strings.TrimSpace(*f.Text), maxEmbedTextRunes))
	case f.Summary != nil:
		parts = append(parts, strings.TrimSpace(*f.Summary))
	}

	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}

	return string([]rune(s)[:n])
}
