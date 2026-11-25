package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ismd/linktheca/internal/core/content"
	"github.com/ismd/linktheca/internal/core/netsafe"
	"github.com/ismd/linktheca/internal/radar"
	"github.com/riverqueue/river"
)

const (
	// fetchTimeout bounds one page download, robots.txt included.
	fetchTimeout = 15 * time.Second

	// minContentRunes is the least text a page must yield to be used. Less
	// is usually a paywall, a cookie wall or a page that renders in
	// JavaScript, and the feed's own summary says more than that.
	minContentRunes = 300
)

// RobotsChecker answers whether a page may be fetched; see crawler.Robots.
type RobotsChecker interface {
	Allowed(ctx context.Context, rawURL string) (bool, error)
}

// Pacer spaces requests to one host; see crawler.HostPacer.
type Pacer interface {
	Wait(host string, jobID int64) time.Duration
}

// FetchFindingContentWorker downloads the page a finding links to, so the
// finding is embedded from the article itself rather than from the few words
// the feed carries. Whatever happens, the finding moves on to embedding: when
// the page cannot be had, it is embedded from its title and summary as before.
type FetchFindingContentWorker struct {
	river.WorkerDefaults[FetchFindingContentArgs]
	store     *radar.Store
	contents  *content.Store
	extractor content.Extractor
	robots    RobotsChecker
	pacer     Pacer
	inserter  Inserter
}

func NewFetchFindingContentWorker(
	store *radar.Store,
	contents *content.Store,
	extractor content.Extractor,
	robots RobotsChecker,
	pacer Pacer,
) *FetchFindingContentWorker {
	return &FetchFindingContentWorker{
		store:     store,
		contents:  contents,
		extractor: extractor,
		robots:    robots,
		pacer:     pacer,
	}
}

func (w *FetchFindingContentWorker) SetInserter(i Inserter) { w.inserter = i }

func (w *FetchFindingContentWorker) Work(ctx context.Context, job *river.Job[FetchFindingContentArgs]) error {
	args := job.Args

	f, err := w.store.GetFindingForFetch(ctx, args.FindingID)
	if errors.Is(err, radar.ErrNotFound) {
		return nil // deleted together with its feed
	}
	if err != nil {
		return fmt.Errorf("get finding %d: %w", args.FindingID, err)
	}

	if f.ContentID == nil {
		contentID, err := w.fetch(ctx, job, f)

		var snooze *river.JobSnoozeError
		switch {
		case errors.As(err, &snooze):
			return err
		case err != nil && job.Attempt < job.MaxAttempts:
			return err
		case err != nil:
			slog.InfoContext(ctx, "radar: embedding finding without its page",
				"finding_id", f.ID, "url", f.URL, "err", err)
		case contentID != 0:
			if err := w.store.SetFindingContent(ctx, f.ID, contentID); err != nil {
				return fmt.Errorf("link content to finding %d: %w", f.ID, err)
			}
		}
	}

	if w.inserter == nil {
		return nil
	}

	embed := EmbedFindingArgs{FindingID: f.ID, Reembed: args.Reembed, SkipMatch: args.SkipMatch}
	if _, err := w.inserter.Insert(ctx, embed, nil); err != nil {
		return fmt.Errorf("enqueue embed for finding %d: %w", f.ID, err)
	}

	return nil
}

// fetch gets the page text for f and returns its article_contents ID, or 0
// when the page is off limits or has no usable text. An error asks River to
// try again later: a snooze while the host is being paced, or a failure that
// may go away, such as a timeout or a 503.
func (w *FetchFindingContentWorker) fetch(
	ctx context.Context,
	job *river.Job[FetchFindingContentArgs],
	f *radar.FindingForFetch,
) (int64, error) {
	log := slog.With("finding_id", f.ID, "url", f.URL)

	// Another feed, or someone's Library, may have fetched the page already.
	rec, err := w.contents.GetByURL(ctx, f.URL)
	if err == nil && hasUsableText(rec.Text) {
		return rec.ID, nil
	}
	if err != nil && !errors.Is(err, content.ErrNotFound) {
		return 0, fmt.Errorf("look up content: %w", err)
	}

	u, err := url.Parse(f.URL)
	if err != nil || u.Hostname() == "" {
		log.DebugContext(ctx, "radar: finding url is not fetchable")
		return 0, nil
	}

	if wait := w.pacer.Wait(u.Hostname(), job.ID); wait > 0 {
		return 0, river.JobSnooze(wait)
	}

	fetchCtx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	allowed, err := w.robots.Allowed(fetchCtx, f.URL)
	if errors.Is(err, netsafe.ErrBlockedAddress) {
		log.DebugContext(ctx, "radar: finding url is not public")
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !allowed {
		log.DebugContext(ctx, "radar: robots.txt disallows the page")
		return 0, nil
	}

	article, err := w.extractor.Extract(fetchCtx, f.URL)
	if content.IsTransient(err) {
		return 0, err
	}
	if err != nil {
		log.DebugContext(ctx, "radar: page fetch failed", "err", err)
		return 0, nil
	}
	if !hasUsableText(&article.Text) {
		log.DebugContext(ctx, "radar: page has too little text")
		return 0, nil
	}

	rec, err = w.contents.Upsert(ctx, content.ParamsFromArticle(article))
	if err != nil {
		return 0, fmt.Errorf("store content: %w", err)
	}

	return rec.ID, nil
}

func hasUsableText(text *string) bool {
	return text != nil && utf8.RuneCountInString(strings.TrimSpace(*text)) >= minContentRunes
}
