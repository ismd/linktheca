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
	// MatchCutoff is the threshold after learning from the user's marks.
	MatchCutoff  float32
	IsActive     bool
	HasEmbedding bool
}

// topFindingsByVector scores every finding against a probe, best first: by
// the match score against matchVec (the probe's topic match vector), with the
// plain cosine against vec alongside. Unlike the real matcher it ignores feed
// subscriptions — a debug probe has no owner to scope by.
func topFindingsByVector(ctx context.Context, pool *pgxpool.Pool, vec, matchVec pgvector.Vector, limit int) ([]scoredFinding, error) {
	rows, err := pool.Query(ctx, `
		SELECT f.id, f.title, f.url,
		       -(f.match_embedding <#> $2) AS score,
		       1 - (f.embedding <=> $1) AS cos,
		       f.content_id IS NOT NULL
		FROM radar_findings f
		WHERE f.match_embedding IS NOT NULL
		ORDER BY f.match_embedding <#> $2
		LIMIT $3
	`, vec, matchVec, limit)
	if err != nil {
		return nil, fmt.Errorf("query findings: %w", err)
	}

	return collectScored(rows)
}

// topFindingsByTopic scores findings against the topic's stored model — the
// same score MatchFindingToTopics uses, without its cutoff filter and with the
// findings the user marked irrelevant still in.
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
		SELECT f.id, f.title, f.url,
		       -(rt.match_embedding <#> f.match_embedding) - rt.match_offset AS score,
		       1 - (rt.embedding <=> f.embedding) AS cos,
		       f.content_id IS NOT NULL
		FROM radar_topics rt
		JOIN radar_findings f ON f.match_embedding IS NOT NULL
		WHERE rt.id = $1
		  AND rt.match_embedding IS NOT NULL`+subscriptionFilter+`
		ORDER BY rt.match_embedding <#> f.match_embedding
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
		if err := rows.Scan(&f.ID, &f.Title, &f.URL, &f.Score, &f.Cos, &f.HasPage); err != nil {
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
		t                 topicRow
		hasEmbedding      bool
		hasMatchEmbedding bool
	)
	err := pool.QueryRow(ctx, `
		SELECT t.id, u.email, t.name, t.match_threshold, t.match_cutoff, t.is_active,
		       t.embedding IS NOT NULL, t.match_embedding IS NOT NULL
		FROM radar_topics t
		JOIN users u ON u.id = t.user_id
		WHERE t.id = $1
	`, topicID).Scan(&t.ID, &t.OwnerEmail, &t.Name, &t.MatchThreshold, &t.MatchCutoff, &t.IsActive, &hasEmbedding, &hasMatchEmbedding)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("topic %d not found", topicID)
	}
	if err != nil {
		return nil, fmt.Errorf("load topic %d: %w", topicID, err)
	}

	if !hasEmbedding {
		return nil, fmt.Errorf("topic %d has no embedding yet — run reembed-topics", topicID)
	}
	if !hasMatchEmbedding {
		return nil, fmt.Errorf("topic %d has no match vector yet — a running server fills it in, or run reembed-topics", topicID)
	}
	t.HasEmbedding = true

	return &t, nil
}

func listTopics(ctx context.Context, pool *pgxpool.Pool) ([]topicRow, error) {
	rows, err := pool.Query(ctx, `
		SELECT t.id, u.email, t.name, t.match_threshold, t.match_cutoff, t.is_active,
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
		if err := rows.Scan(&t.ID, &t.OwnerEmail, &t.Name, &t.MatchThreshold, &t.MatchCutoff, &t.IsActive, &t.HasEmbedding); err != nil {
			return nil, fmt.Errorf("scan topic: %w", err)
		}
		out = append(out, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate topics: %w", err)
	}

	return out, nil
}
