// Command radar-backfill-content downloads the pages of findings that Radar
// found before it downloaded pages, and re-embeds those findings from them.
//
// It only queues jobs. A running linktheca-server does the work, paced per
// site and following robots.txt like for any new finding, so it is safe to run
// against a live instance. Findings whose page cannot be had keep being matched
// on their title and summary.
//
// Re-embedded findings are not matched again by default: their topics have
// already seen them, and matching weeks-old news would flood inboxes. Pass
// -match to match them too, e.g. after changing the match threshold.
//
// In the bundled compose stack the binary ships in the backend image:
//
//	docker compose -f compose.prod.yaml exec backend radar-backfill-content -days 7
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ismd/linktheca/internal/core/config"
	"github.com/ismd/linktheca/internal/core/db"
	"github.com/ismd/linktheca/internal/core/logging"
	"github.com/ismd/linktheca/internal/radar"
	"github.com/ismd/linktheca/internal/radar/jobs"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

// insertBatch is how many jobs go into one InsertMany call.
const insertBatch = 500

type options struct {
	days   int
	match  bool
	dryRun bool
}

func main() {
	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "radar-backfill-content:", err)
		os.Exit(2)
	}

	if err := run(opts); err != nil {
		slog.Error("radar-backfill-content failed", "err", err)
		os.Exit(1)
	}
}

func parseArgs(args []string) (*options, error) {
	var opts options

	fs := flag.NewFlagSet("radar-backfill-content", flag.ContinueOnError)
	fs.IntVar(&opts.days, "days", 7, "backfill findings discovered in the last N days")
	fs.BoolVar(&opts.match, "match", false, "match the re-embedded findings against topics again")
	fs.BoolVar(&opts.dryRun, "dry-run", false, "only report how many findings would be queued")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	if opts.days < 1 {
		return nil, fmt.Errorf("-days must be positive, got %d", opts.days)
	}

	return &opts, nil
}

func jobArgs(ids []int64, match bool) []river.InsertManyParams {
	params := make([]river.InsertManyParams, 0, len(ids))
	for _, id := range ids {
		params = append(params, river.InsertManyParams{
			Args: jobs.FetchFindingContentArgs{FindingID: id, Reembed: true, SkipMatch: !match},
		})
	}

	return params
}

func run(opts *options) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := logging.New(os.Stdout, cfg.LogFormat, cfg.LogLevel)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.NewPool(ctx, cfg.DBDSN)
	if err != nil {
		return err
	}
	defer pool.Close()

	since := time.Now().AddDate(0, 0, -opts.days)
	ids, err := radar.NewStore(pool).ListFindingsWithoutPage(ctx, since)
	if err != nil {
		return err
	}

	logger.Info("findings without a downloaded page", "count", len(ids), "since", since.Format(time.DateOnly))
	if opts.dryRun || len(ids) == 0 {
		return nil
	}

	// No queues configured: this client only inserts jobs.
	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{})
	if err != nil {
		return fmt.Errorf("river client: %w", err)
	}

	params := jobArgs(ids, opts.match)
	for start := 0; start < len(params); start += insertBatch {
		end := min(start+insertBatch, len(params))
		if _, err := client.InsertMany(ctx, params[start:end]); err != nil {
			return fmt.Errorf("queue jobs: %w", err)
		}
	}

	logger.Info("queued page downloads; a running linktheca-server works through them",
		"count", len(params), "match", opts.match)

	return nil
}
