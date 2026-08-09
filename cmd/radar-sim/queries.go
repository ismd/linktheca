package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

// topicRow is a radar_topics row as printed by -topics.
type topicRow struct {
	ID             int64
	OwnerEmail     string
	Name           string
	MatchThreshold float32
	IsActive       bool
	HasEmbedding   bool
}

// topFindingsByVector scores every embedded finding against vec, best first.
// Unlike the real matcher it ignores feed subscriptions — a debug probe has no
// owner to scope by.
func topFindingsByVector(ctx context.Context, pool *pgxpool.Pool, vec pgvector.Vector, limit int) ([]scoredFinding, error) {
	rows, err := pool.Query(ctx, `
		SELECT f.id, f.title, f.url, 1 - (f.embedding <=> $1) AS similarity
		FROM radar_findings f
		WHERE f.embedding IS NOT NULL
		ORDER BY f.embedding <=> $1
		LIMIT $2
	`, vec, limit)
	if err != nil {
		return nil, fmt.Errorf("query findings: %w", err)
	}

	return collectScored(rows)
}

// topFindingsByTopic scores findings against the topic's stored embedding —
// the same expression MatchFindingToTopics uses, minus the threshold filter.
// With subscribedOnly it also applies the subscription join of the real
// matcher, which is what tells "sim too low" apart from "feed not subscribed".
func topFindingsByTopic(ctx context.Context, pool *pgxpool.Pool, topicID int64, limit int, subscribedOnly bool) ([]scoredFinding, error) {
	subscriptionFilter := ""
	if subscribedOnly {
		subscriptionFilter = `
		  AND EXISTS (
		    SELECT 1 FROM radar_feed_subscriptions rfs
		    WHERE rfs.user_id = rt.user_id AND rfs.feed_id = f.feed_id
		  )`
	}

	rows, err := pool.Query(ctx, `
		SELECT f.id, f.title, f.url, 1 - (rt.embedding <=> f.embedding) AS similarity
		FROM radar_topics rt
		JOIN radar_findings f ON f.embedding IS NOT NULL
		WHERE rt.id = $1
		  AND rt.embedding IS NOT NULL`+subscriptionFilter+`
		ORDER BY rt.embedding <=> f.embedding
		LIMIT $2
	`, topicID, limit)
	if err != nil {
		return nil, fmt.Errorf("query findings for topic %d: %w", topicID, err)
	}

	return collectScored(rows)
}

func collectScored(rows pgx.Rows) ([]scoredFinding, error) {
	defer rows.Close()

	var out []scoredFinding
	for rows.Next() {
		var f scoredFinding
		if err := rows.Scan(&f.ID, &f.Title, &f.URL, &f.Similarity); err != nil {
			return nil, fmt.Errorf("scan finding: %w", err)
		}
		out = append(out, f)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate findings: %w", err)
	}

	return out, nil
}

// loadTopic reads the metadata radar-sim needs to label a -topic run and
// fails loudly when the topic cannot be probed at all.
func loadTopic(ctx context.Context, pool *pgxpool.Pool, topicID int64) (*topicRow, error) {
	var (
		t            topicRow
		hasEmbedding bool
	)
	err := pool.QueryRow(ctx, `
		SELECT t.id, u.email, t.name, t.match_threshold, t.is_active,
		       t.embedding IS NOT NULL
		FROM radar_topics t
		JOIN users u ON u.id = t.user_id
		WHERE t.id = $1
	`, topicID).Scan(&t.ID, &t.OwnerEmail, &t.Name, &t.MatchThreshold, &t.IsActive, &hasEmbedding)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("topic %d not found", topicID)
	}
	if err != nil {
		return nil, fmt.Errorf("load topic %d: %w", topicID, err)
	}

	if !hasEmbedding {
		return nil, fmt.Errorf("topic %d has no embedding yet — run reembed-topics", topicID)
	}
	t.HasEmbedding = true

	return &t, nil
}

func listTopics(ctx context.Context, pool *pgxpool.Pool) ([]topicRow, error) {
	rows, err := pool.Query(ctx, `
		SELECT t.id, u.email, t.name, t.match_threshold, t.is_active,
		       t.embedding IS NOT NULL
		FROM radar_topics t
		JOIN users u ON u.id = t.user_id
		ORDER BY t.id
	`)
	if err != nil {
		return nil, fmt.Errorf("list topics: %w", err)
	}
	defer rows.Close()

	var out []topicRow
	for rows.Next() {
		var t topicRow
		if err := rows.Scan(&t.ID, &t.OwnerEmail, &t.Name, &t.MatchThreshold, &t.IsActive, &t.HasEmbedding); err != nil {
			return nil, fmt.Errorf("scan topic: %w", err)
		}
		out = append(out, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate topics: %w", err)
	}

	return out, nil
}
