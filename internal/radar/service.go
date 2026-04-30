package radar

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/pgvector/pgvector-go"
)

const defaultMatchThreshold = .75

type StoreAPI interface {
	CreateTopic(ctx context.Context, p CreateTopicParams) (*Topic, error)
	UpdateTopicEmbedding(ctx context.Context, topicID int64, vec pgvector.Vector) error
	AddFeed(ctx context.Context, p AddFeedParams) (*Feed, error)
	Subscribe(ctx context.Context, userID, feedID int64) (*Subscription, error)
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

	vec, err := s.embedder.Embed(ctx, desc)
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
