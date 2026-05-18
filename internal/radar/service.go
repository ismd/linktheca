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
