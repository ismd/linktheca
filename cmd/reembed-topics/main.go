// Command reembed-topics recomputes embeddings for every row in radar_topics
// and relearns each topic from the user's marks.
//
// Useful when the topic embedding strategy changes (e.g. switched from
// name+description to description-only, see radar.TopicEmbedText) or the
// reference topics do (radar.Reference) — existing rows are stale and need a
// one-shot re-embed against TEI.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ismd/linktheca/internal/core/config"
	"github.com/ismd/linktheca/internal/core/db"
	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/ismd/linktheca/internal/core/logging"
	"github.com/ismd/linktheca/internal/radar"
)

func main() {
	if err := run(); err != nil {
		slog.Error("reembed-topics failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
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

	teiClient := embeddings.NewTEIClient(cfg.TEIURL, cfg.TEITimeout)
	svc := radar.NewService(radar.NewStore(pool), teiClient)

	rows, err := pool.Query(ctx, `SELECT id, name FROM radar_topics ORDER BY id`)
	if err != nil {
		return fmt.Errorf("list topics: %w", err)
	}

	type topicRow struct {
		ID   int64
		Name string
	}
	var topics []topicRow
	for rows.Next() {
		var t topicRow
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			rows.Close()
			return fmt.Errorf("scan topic: %w", err)
		}
		topics = append(topics, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate topics: %w", err)
	}

	logger.Info("reembedding topics", "count", len(topics))

	for _, t := range topics {
		if err := svc.ReembedTopic(ctx, t.ID); err != nil {
			return fmt.Errorf("reembed topic %d: %w", t.ID, err)
		}
		logger.Info("reembedded topic", "id", t.ID, "name", t.Name)
	}

	logger.Info("done")
	return nil
}
