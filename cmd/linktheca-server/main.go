package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	linktheca "github.com/ismd/linktheca"
	"github.com/ismd/linktheca/internal/core/config"
	"github.com/ismd/linktheca/internal/core/db"
	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/ismd/linktheca/internal/core/logging"
	"github.com/ismd/linktheca/internal/radar"
	"github.com/ismd/linktheca/internal/radar/crawler"
	"github.com/ismd/linktheca/internal/radar/jobs"
	"github.com/ismd/linktheca/internal/server"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

func main() {
	if err := run(); err != nil {
		slog.Error("startup failed", "err", err)
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

	if err := db.Migrate(ctx, pool, linktheca.MigrationsFS, "migrations"); err != nil {
		return err
	}
	logger.Info("goose migrations applied")

	var radarDeps *server.RadarDeps
	var riverClient *river.Client[pgx.Tx]
	if cfg.RadarEnabled {
		// River migrations.
		mig, err := rivermigrate.New(riverpgxv5.New(pool), nil)
		if err != nil {
			return fmt.Errorf("river migrate new: %w", err)
		}
		if _, err := mig.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
			return fmt.Errorf("river migrate: %w", err)
		}
		logger.Info("river migrations applied")

		teiClient := embeddings.NewTEIClient(cfg.TEIURL, cfg.TEITimeout)
		// Best-effort self-check; warn but don't fail-fast.
		if vec, err := teiClient.Embed(ctx, "ping"); err != nil {
			logger.Warn("TEI self-check failed", "err", err)
		} else if len(vec) != cfg.EmbeddingDim {
			logger.Warn("TEI embedding dim mismatch",
				"want", cfg.EmbeddingDim, "got", len(vec))
		}

		store := radar.NewStore(pool)
		bundle := jobs.Build(jobs.Deps{
			Store:    store,
			Embedder: teiClient,
			Fetcher:  crawler.NewHTTPFetcher(30 * time.Second),
		}, cfg.RadarSchedulerInterval)

		client, err := jobs.NewClient(pool, bundle.Workers, bundle.Periodic, cfg.RadarMaxWorkers)
		if err != nil {
			return fmt.Errorf("river new client: %w", err)
		}
		bundle.WireInserter(client)
		if err := client.Start(ctx); err != nil {
			return fmt.Errorf("river start: %w", err)
		}
		riverClient = client

		radarDeps = &server.RadarDeps{
			Embedder: teiClient,
			River:    client,
			Workers:  bundle.Workers,
		}
	}

	srv := server.New(server.Deps{
		Config: cfg,
		Logger: logger,
		DB:     pool,
		Radar:  radarDeps,
	})

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server starting", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	logger.Info("http server stopped")

	if riverClient != nil {
		if err := riverClient.Stop(shutdownCtx); err != nil {
			logger.Warn("river stop", "err", err)
		}
	}

	return nil
}
