// Package radar implements the news-monitoring module: topics, feeds,
// subscriptions, crawled findings, and topic↔finding matches backed by
// pgvector similarity search.
package radar

import (
	"errors"
	"time"
)

var (
	ErrNotFound   = errors.New("not found")
	ErrDuplicate  = errors.New("duplicate")
	ErrFeedNotFound = errors.New("feed not found")
)

type Topic struct {
	ID             int64     `json:"id"`
	UserID         int64     `json:"user_id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	MatchThreshold float32   `json:"match_threshold"`
	IsActive       bool      `json:"is_active"`
	HasEmbedding   bool      `json:"has_embedding"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Feed struct {
	ID                   int64      `json:"id"`
	URL                  string     `json:"url"`
	Kind                 string     `json:"kind"`
	Title                *string    `json:"title,omitempty"`
	FetchIntervalSeconds int        `json:"fetch_interval_seconds"`
	IsActive             bool       `json:"is_active"`
	LastFetchedAt        *time.Time `json:"last_fetched_at,omitempty"`
	LastError            *string    `json:"last_error,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
}

type Subscription struct {
	UserID    int64     `json:"user_id"`
	FeedID    int64     `json:"feed_id"`
	CreatedAt time.Time `json:"created_at"`
}

type Finding struct {
	ID           int64      `json:"id"`
	FeedID       int64      `json:"feed_id"`
	ContentID    *int64     `json:"content_id,omitempty"`
	ExternalID   *string    `json:"external_id,omitempty"`
	URL          string     `json:"url"`
	Title        *string    `json:"title,omitempty"`
	Summary      *string    `json:"summary,omitempty"`
	PublishedAt  *time.Time `json:"published_at,omitempty"`
	DiscoveredAt time.Time  `json:"discovered_at"`
	HasEmbedding bool       `json:"has_embedding"`
}

type Match struct {
	ID         int64     `json:"id"`
	TopicID    int64     `json:"topic_id"`
	FindingID  int64     `json:"finding_id"`
	Similarity float32   `json:"similarity"`
	State      string    `json:"state"`
	MatchedAt  time.Time `json:"matched_at"`
}

// DTOs ---------------------------------------------------------------------

type CreateTopicRequest struct {
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	MatchThreshold *float32 `json:"match_threshold,omitempty"`
}

type AddFeedRequest struct {
	URL                  string  `json:"url"`
	Kind                 *string `json:"kind,omitempty"`
	FetchIntervalSeconds *int    `json:"fetch_interval_seconds,omitempty"`
}

type SubscribeRequest struct {
	FeedID int64 `json:"feed_id"`
}

// Internal params ---------------------------------------------------------

type CreateTopicParams struct {
	UserID         int64
	Name           string
	Description    string
	MatchThreshold float32
}

type AddFeedParams struct {
	URL                  string
	Kind                 string
	FetchIntervalSeconds int
}

type FindingUpsert struct {
	FeedID      int64
	ExternalID  *string
	URL         string
	Title       *string
	Summary     *string
	PublishedAt *time.Time
}
