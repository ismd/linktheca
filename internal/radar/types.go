// Package radar implements the news-monitoring module: topics, feeds,
// subscriptions, crawled findings, and topic↔finding matches backed by
// pgvector similarity search.
package radar

import (
	"errors"
	"time"
)

var (
	ErrNotFound            = errors.New("not found")
	ErrDuplicate           = errors.New("duplicate")
	ErrFeedNotFound        = errors.New("feed not found")
	ErrInvalidInput        = errors.New("invalid input")
	ErrEmbedderUnavailable = errors.New("embedder unavailable")
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

// --- Read-API types -------------------------------------------------------

// TopicStats holds aggregated match counts for a topic.
type TopicStats struct {
	NewCount    int        `json:"new_count"`
	TotalCount  int        `json:"total_count"`
	SourceCount int        `json:"source_count"`
	LastMatchAt *time.Time `json:"last_match_at"`
}

// TopicWithStats is a Topic enriched with aggregate match stats.
// Returned by GET /radar/topics and GET /radar/topics/{id}.
type TopicWithStats struct {
	Topic
	Stats TopicStats `json:"stats"`
}

// MatchFinding is the finding portion of a denormalized MatchView.
type MatchFinding struct {
	ID           int64      `json:"id"`
	FeedID       int64      `json:"feed_id"`
	FeedTitle    *string    `json:"feed_title"`
	URL          string     `json:"url"`
	Title        *string    `json:"title"`
	Summary      *string    `json:"summary"`
	PublishedAt  *time.Time `json:"published_at"`
	DiscoveredAt time.Time  `json:"discovered_at"`
}

// MatchView is a Match denormalized with topic name and finding metadata.
// Returned by GET /radar/matches.
type MatchView struct {
	ID         int64        `json:"id"`
	TopicID    int64        `json:"topic_id"`
	TopicName  string       `json:"topic_name"`
	Similarity float32      `json:"similarity"`
	State      string       `json:"state"`
	MatchedAt  time.Time    `json:"matched_at"`
	Finding    MatchFinding `json:"finding"`
}

// ListMatchesParams holds query parameters for GET /radar/matches.
type ListMatchesParams struct {
	UserID  int64
	TopicID *int64  // nil = any topic owned by UserID
	State   *string // nil = any state
	Limit   int
	Offset  int
}

// MatchList holds the paginated response for GET /radar/matches.
type MatchList struct {
	Items []MatchView `json:"items"`
	Total int         `json:"total"`
}

// ListFeedsParams holds query parameters for GET /radar/feeds (admin).
type ListFeedsParams struct {
	Limit  int
	Offset int
}

// FeedListItem is one catalog row: the feed plus per-user subscription state
// and how many findings it has produced.
type FeedListItem struct {
	Feed
	Subscribed   bool `json:"subscribed"`
	FindingCount int  `json:"finding_count"`
}

// FeedList holds the paginated response for GET /radar/feeds.
type FeedList struct {
	Items []FeedListItem `json:"items"`
	Total int            `json:"total"`
}

// UpdateTopicRequest is the payload for PATCH /radar/topics/{id}.
// All fields are optional; only non-nil fields are updated.
type UpdateTopicRequest struct {
	Name           *string  `json:"name,omitempty"`
	Description    *string  `json:"description,omitempty"`
	MatchThreshold *float32 `json:"match_threshold,omitempty"`
	IsActive       *bool    `json:"is_active,omitempty"`
}

// UpdateTopicParams is the store-level analogue of UpdateTopicRequest.
type UpdateTopicParams struct {
	Name           *string
	Description    *string
	MatchThreshold *float32
	IsActive       *bool
}

// PreviewTopicRequest is the payload for POST /radar/topics/preview.
// It carries the draft topic the user is still typing; nothing is persisted.
type PreviewTopicRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// PreviewMatch is a finding scored against a draft topic. It mirrors the
// shape of MatchView (similarity + finding) so clients can render previews
// with the same components they use for real matches.
type PreviewMatch struct {
	Similarity float32      `json:"similarity"`
	Finding    MatchFinding `json:"finding"`
}

// TopicPreview is the response for POST /radar/topics/preview. Threshold is
// the cutoff the topic would be created with, so the client can show which of
// the returned findings would actually have become matches.
type TopicPreview struct {
	Items     []PreviewMatch `json:"items"`
	Threshold float32        `json:"threshold"`
}

// UpdateMatchRequest is the payload for PATCH /radar/matches/{id}.
type UpdateMatchRequest struct {
	State string `json:"state"`
}

// RadarStatus is the response for GET /radar/status.
type RadarStatus struct {
	LastSweepAt *time.Time `json:"last_sweep_at"`
}
