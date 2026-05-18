package radar

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/pgvector/pgvector-go"
)

// defaultMatchThreshold sits between BGE-M3's noise floor (~0.4 for unrelated
// pairs) and weak cross-lingual semantic matches (~0.55–0.6). Per-topic
// match_threshold lets users tighten for high-precision topics.
const defaultMatchThreshold = .55

type StoreAPI interface {
	CreateTopic(ctx context.Context, p CreateTopicParams) (*Topic, error)
	UpdateTopicEmbedding(ctx context.Context, topicID int64, vec pgvector.Vector) error
	AddFeed(ctx context.Context, p AddFeedParams) (*Feed, error)
	Subscribe(ctx context.Context, userID, feedID int64) (*Subscription, error)

	// Read-API extensions:
	ListTopicsWithStats(ctx context.Context, userID int64) ([]TopicWithStats, error)
	GetTopicWithStats(ctx context.Context, userID, topicID int64) (*TopicWithStats, error)
	UpdateTopic(ctx context.Context, userID, topicID int64, p UpdateTopicParams) (*Topic, error)
	DeleteTopic(ctx context.Context, userID, topicID int64) error
	ListMatches(ctx context.Context, userID int64, p ListMatchesParams) ([]MatchView, int, error)
	UpdateMatchState(ctx context.Context, userID, matchID int64, state string) error
	LastSweepAt(ctx context.Context, userID int64) (*time.Time, error)
	ListFeeds(ctx context.Context, p ListFeedsParams) ([]Feed, int, error)
}

type Service struct {
	store    StoreAPI
	embedder embeddings.Client
}

func NewService(store StoreAPI, embedder embeddings.Client) *Service {
	return &Service{store: store, embedder: embedder}
}

// CreateTopic validates the request, persists the topic, and synchronously
// computes its embedding. If the embedder is unavailable the topic stays in
// the database without an embedding and is silently skipped by MatchFindingJob.
func (s *Service) CreateTopic(ctx context.Context, userID int64, req CreateTopicRequest) (*Topic, error) {
	name := strings.TrimSpace(req.Name)
	desc := strings.TrimSpace(req.Description)

	if name == "" || len(name) > 200 {
		return nil, fmt.Errorf("%w: name must be 1..200 chars", ErrInvalidInput)
	}

	if len(desc) < 10 || len(desc) > 2000 {
		return nil, fmt.Errorf("%w: description must be 10..2000 chars", ErrInvalidInput)
	}

	threshold := float32(defaultMatchThreshold)
	if req.MatchThreshold != nil {
		threshold = *req.MatchThreshold
		if threshold < 0 || threshold > 1 {
			return nil, fmt.Errorf("%w: match_threshold must be in [0,1]", ErrInvalidInput)
		}
	}

	topic, err := s.store.CreateTopic(ctx, CreateTopicParams{
		UserID: userID, Name: name, Description: desc, MatchThreshold: threshold,
	})
	if err != nil {
		return nil, fmt.Errorf("create topic: %w", err)
	}

	vec, err := s.embedder.Embed(ctx, name+": "+desc)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEmbedderUnavailable, err)
	}

	if err := s.store.UpdateTopicEmbedding(ctx, topic.ID, pgvector.NewVector(vec)); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("save embedding: %w", err)
	}

	topic.HasEmbedding = true
	return topic, nil
}

const (
	defaultFetchIntervalSeconds = 3600
	minFetchIntervalSeconds     = 300
	maxFetchIntervalSeconds     = 86400
)

func (s *Service) AddFeed(ctx context.Context, req AddFeedRequest) (*Feed, error) {
	url := strings.TrimSpace(req.URL)

	if url == "" || len(url) > 2000 {
		return nil, fmt.Errorf("%w: url must be 1..2000 chars", ErrInvalidInput)
	}

	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return nil, fmt.Errorf("%w: url must be http(s)", ErrInvalidInput)
	}

	kind := "rss"
	if req.Kind != nil {
		kind = *req.Kind
		if kind != "rss" && kind != "atom" {
			return nil, fmt.Errorf("%w: kind must be rss|atom", ErrInvalidInput)
		}
	}

	interval := defaultFetchIntervalSeconds
	if req.FetchIntervalSeconds != nil {
		interval = *req.FetchIntervalSeconds
		if interval < minFetchIntervalSeconds || interval > maxFetchIntervalSeconds {
			return nil, fmt.Errorf("%w: fetch_interval_seconds must be %d..%d",
				ErrInvalidInput, minFetchIntervalSeconds, maxFetchIntervalSeconds)
		}
	}

	feed, err := s.store.AddFeed(ctx, AddFeedParams{
		URL: url, Kind: kind, FetchIntervalSeconds: interval,
	})
	if err != nil {
		return nil, err
	}

	return feed, nil
}

func (s *Service) Subscribe(ctx context.Context, userID int64, req SubscribeRequest) (*Subscription, error) {
	if req.FeedID <= 0 {
		return nil, fmt.Errorf("%w: feed_id must be positive", ErrInvalidInput)
	}

	return s.store.Subscribe(ctx, userID, req.FeedID)
}

// ListTopics returns all topics of a user with aggregate match stats.
func (s *Service) ListTopics(ctx context.Context, userID int64) ([]TopicWithStats, error) {
	return s.store.ListTopicsWithStats(ctx, userID)
}

// GetTopic returns a single topic with aggregate match stats, or ErrNotFound.
func (s *Service) GetTopic(ctx context.Context, userID, topicID int64) (*TopicWithStats, error) {
	return s.store.GetTopicWithStats(ctx, userID, topicID)
}

// DeleteTopic removes a topic and CASCADEs its matches.
func (s *Service) DeleteTopic(ctx context.Context, userID, topicID int64) error {
	return s.store.DeleteTopic(ctx, userID, topicID)
}

// UpdateTopic validates the patch, persists changed fields, and — if
// `description` was in the patch — re-embeds the topic. Mirrors CreateTopic:
// embedder failure leaves the topic's fields updated and embedding stale, and
// returns ErrEmbedderUnavailable. The caller can retry with the same payload.
func (s *Service) UpdateTopic(ctx context.Context, userID, topicID int64, req UpdateTopicRequest) (*Topic, error) {
	p := UpdateTopicParams{
		Name:           req.Name,
		Description:    req.Description,
		MatchThreshold: req.MatchThreshold,
		IsActive:       req.IsActive,
	}

	if p.Name == nil && p.Description == nil && p.MatchThreshold == nil && p.IsActive == nil {
		return nil, fmt.Errorf("%w: no fields to update", ErrInvalidInput)
	}

	if p.Name != nil {
		n := strings.TrimSpace(*p.Name)
		if n == "" || len(n) > 200 {
			return nil, fmt.Errorf("%w: name must be 1..200 chars", ErrInvalidInput)
		}
		p.Name = &n
	}
	if p.Description != nil {
		d := strings.TrimSpace(*p.Description)
		if len(d) < 10 || len(d) > 2000 {
			return nil, fmt.Errorf("%w: description must be 10..2000 chars", ErrInvalidInput)
		}
		p.Description = &d
	}
	if p.MatchThreshold != nil {
		if *p.MatchThreshold < 0 || *p.MatchThreshold > 1 {
			return nil, fmt.Errorf("%w: match_threshold must be in [0,1]", ErrInvalidInput)
		}
	}

	topic, err := s.store.UpdateTopic(ctx, userID, topicID, p)
	if err != nil {
		return nil, err
	}

	if p.Description != nil {
		vec, err := s.embedder.Embed(ctx, topic.Name+": "+topic.Description)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrEmbedderUnavailable, err)
		}
		if err := s.store.UpdateTopicEmbedding(ctx, topic.ID, pgvector.NewVector(vec)); err != nil {
			if errors.Is(err, ErrNotFound) {
				return nil, err
			}
			return nil, fmt.Errorf("save embedding: %w", err)
		}
		topic.HasEmbedding = true
	}

	return topic, nil
}

// ListMatches returns paginated matches for a user, optionally filtered by
// topic and/or state. Clamps Limit to [1,100] (default 50) and Offset to >=0.
func (s *Service) ListMatches(ctx context.Context, p ListMatchesParams) (*MatchList, error) {
	if p.Limit <= 0 || p.Limit > 100 {
		if p.Limit > 100 {
			p.Limit = 100
		} else {
			p.Limit = 50
		}
	}
	if p.Offset < 0 {
		p.Offset = 0
	}

	items, total, err := s.store.ListMatches(ctx, p.UserID, p)
	if err != nil {
		return nil, err
	}
	return &MatchList{Items: items, Total: total}, nil
}

// SetMatchState updates a match's state. Valid states: "new", "seen".
func (s *Service) SetMatchState(ctx context.Context, userID, matchID int64, state string) error {
	if state != "new" && state != "seen" {
		return fmt.Errorf("%w: state must be new|seen", ErrInvalidInput)
	}
	return s.store.UpdateMatchState(ctx, userID, matchID, state)
}

// LastSweep returns the latest fetch timestamp across the user's active
// subscribed feeds, or nil if there are no subscriptions.
func (s *Service) LastSweep(ctx context.Context, userID int64) (*time.Time, error) {
	return s.store.LastSweepAt(ctx, userID)
}

// ListFeeds returns paginated feeds (admin scope; middleware enforces).
func (s *Service) ListFeeds(ctx context.Context, p ListFeedsParams) (*FeedList, error) {
	if p.Limit <= 0 || p.Limit > 100 {
		if p.Limit > 100 {
			p.Limit = 100
		} else {
			p.Limit = 50
		}
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	items, total, err := s.store.ListFeeds(ctx, p)
	if err != nil {
		return nil, err
	}
	return &FeedList{Items: items, Total: total}, nil
}
