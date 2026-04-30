package radar_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/ismd/linktheca/internal/radar"
	"github.com/pgvector/pgvector-go"
	"github.com/stretchr/testify/require"
)

// --- mock store ---

type mockStore struct {
	topics        map[int64]*radar.Topic
	topicEmb      map[int64]pgvector.Vector
	feeds         map[int64]*radar.Feed
	feedsByURL    map[string]*radar.Feed
	subs          map[string]*radar.Subscription
	nextTopicID   int64
	nextFeedID    int64
	createTopicErr error
	addFeedErr     error
	subscribeErr   error
	updateEmbErr   error
}

func newMockStore() *mockStore {
	return &mockStore{
		topics:     make(map[int64]*radar.Topic),
		topicEmb:   make(map[int64]pgvector.Vector),
		feeds:      make(map[int64]*radar.Feed),
		feedsByURL: make(map[string]*radar.Feed),
		subs:       make(map[string]*radar.Subscription),
	}
}

func (m *mockStore) CreateTopic(_ context.Context, p radar.CreateTopicParams) (*radar.Topic, error) {
	if m.createTopicErr != nil {
		return nil, m.createTopicErr
	}
	m.nextTopicID++
	t := &radar.Topic{
		ID: m.nextTopicID, UserID: p.UserID, Name: p.Name,
		Description: p.Description, MatchThreshold: p.MatchThreshold,
		IsActive: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	m.topics[t.ID] = t
	return t, nil
}

func (m *mockStore) UpdateTopicEmbedding(_ context.Context, topicID int64, vec pgvector.Vector) error {
	if m.updateEmbErr != nil {
		return m.updateEmbErr
	}
	t, ok := m.topics[topicID]
	if !ok {
		return radar.ErrNotFound
	}
	m.topicEmb[topicID] = vec
	t.HasEmbedding = true
	return nil
}

func (m *mockStore) AddFeed(_ context.Context, p radar.AddFeedParams) (*radar.Feed, error) {
	if m.addFeedErr != nil {
		return nil, m.addFeedErr
	}
	if _, ok := m.feedsByURL[p.URL]; ok {
		return nil, radar.ErrDuplicate
	}
	m.nextFeedID++
	f := &radar.Feed{
		ID: m.nextFeedID, URL: p.URL, Kind: p.Kind,
		FetchIntervalSeconds: p.FetchIntervalSeconds, IsActive: true,
		CreatedAt: time.Now(),
	}
	m.feeds[f.ID] = f
	m.feedsByURL[p.URL] = f
	return f, nil
}

func (m *mockStore) Subscribe(_ context.Context, userID, feedID int64) (*radar.Subscription, error) {
	if m.subscribeErr != nil {
		return nil, m.subscribeErr
	}
	if _, ok := m.feeds[feedID]; !ok {
		return nil, radar.ErrFeedNotFound
	}
	key := keyOf(userID, feedID)
	if existing, ok := m.subs[key]; ok {
		return existing, nil
	}
	sub := &radar.Subscription{UserID: userID, FeedID: feedID, CreatedAt: time.Now()}
	m.subs[key] = sub
	return sub, nil
}

func keyOf(u, f int64) string { return string(rune(u)) + ":" + string(rune(f)) }

// Compile-time check.
var _ radar.StoreAPI = (*mockStore)(nil)

// --- tests ---

func TestService_CreateTopic_Success(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	thr := float32(0.8)
	topic, err := svc.CreateTopic(context.Background(), 7, radar.CreateTopicRequest{
		Name: "AI", Description: "machine learning research", MatchThreshold: &thr,
	})
	require.NoError(t, err)
	require.Equal(t, int64(7), topic.UserID)
	require.Equal(t, "AI", topic.Name)
	require.Equal(t, float32(0.8), topic.MatchThreshold)
	require.True(t, topic.HasEmbedding, "embedding must be set after CreateTopic")
}

func TestService_CreateTopic_DefaultThreshold(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	topic, err := svc.CreateTopic(context.Background(), 1, radar.CreateTopicRequest{
		Name: "x", Description: "ten chars long",
	})
	require.NoError(t, err)
	require.Equal(t, float32(0.75), topic.MatchThreshold)
}

func TestService_CreateTopic_Validation(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	// empty name
	_, err := svc.CreateTopic(context.Background(), 1, radar.CreateTopicRequest{
		Name: "", Description: "ten chars long",
	})
	require.ErrorIs(t, err, radar.ErrInvalidInput)

	// short description
	_, err = svc.CreateTopic(context.Background(), 1, radar.CreateTopicRequest{
		Name: "x", Description: "short",
	})
	require.ErrorIs(t, err, radar.ErrInvalidInput)

	// out-of-range threshold
	bad := float32(1.5)
	_, err = svc.CreateTopic(context.Background(), 1, radar.CreateTopicRequest{
		Name: "x", Description: "ten chars long", MatchThreshold: &bad,
	})
	require.ErrorIs(t, err, radar.ErrInvalidInput)
}

func TestService_CreateTopic_EmbedderError(t *testing.T) {
	store := newMockStore()
	emb := &errEmbedder{err: errors.New("boom")}
	svc := radar.NewService(store, emb)

	_, err := svc.CreateTopic(context.Background(), 1, radar.CreateTopicRequest{
		Name: "x", Description: "ten chars long",
	})
	require.ErrorIs(t, err, radar.ErrEmbedderUnavailable)
}

type errEmbedder struct{ err error }

func (e *errEmbedder) Embed(_ context.Context, _ string) ([]float32, error) {
	return nil, e.err
}
