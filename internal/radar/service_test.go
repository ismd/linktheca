package radar_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/ismd/linktheca/internal/radar"
	"github.com/pgvector/pgvector-go"
	"github.com/stretchr/testify/require"
)

// --- mock store ---

type mockStore struct {
	topics         map[int64]*radar.Topic
	topicEmb       map[int64]pgvector.Vector
	feeds          map[int64]*radar.Feed
	feedsByURL     map[string]*radar.Feed
	subs           map[string]*radar.Subscription
	nextTopicID    int64
	nextFeedID     int64
	createTopicErr error
	addFeedErr     error
	subscribeErr   error
	updateEmbErr   error

	// Read-API recording / overrides:
	listTopicsResult  []radar.TopicWithStats
	listTopicsErr     error
	getTopicResult    *radar.TopicWithStats
	getTopicErr       error
	updateTopicResult *radar.Topic
	updateTopicErr    error
	updateTopicCalled bool
	updateTopicParams radar.UpdateTopicParams
	deleteTopicErr    error
	deleteTopicCalled bool
	listMatchesResult []radar.MatchView
	listMatchesTotal  int
	listMatchesErr    error
	listMatchesCalled bool
	listMatchesParams radar.ListMatchesParams
	updateMatchErr    error
	updateMatchCalled bool
	updateMatchState  string
	lastSweepResult   *time.Time
	lastSweepErr      error
	listFeedsUserID   int64
	listFeedsParams   radar.ListFeedsParams
	listFeedsResult   []radar.FeedListItem
	listFeedsTotal    int
	listFeedsErr      error
	getMatchResult    *radar.MatchView
	getMatchErr       error
	getMatchCalled    bool
	previewResult     []radar.PreviewMatch
	previewErr        error
	previewCalled     bool
	previewUserID     int64
	previewVec        pgvector.Vector
	previewLimit      int
	unsubscribeErr    error
	updateFeedCalled  bool
	updateFeedParams  radar.UpdateFeedParams
	updateFeedErr     error
	deleteFeedCalled  bool
	deleteFeedErr     error
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

func TestService_CreateTopic_EmbedsNameAndDescription(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	topic, err := svc.CreateTopic(context.Background(), 1, radar.CreateTopicRequest{
		Name: "WebAuthn", Description: "webauthn passkeys",
	})
	require.NoError(t, err)

	expected, _ := emb.Embed(context.Background(), "WebAuthn: webauthn passkeys")
	require.Equal(t, pgvector.NewVector(expected), store.topicEmb[topic.ID],
		"topic embedding must be derived from name + description")

	descOnly, _ := emb.Embed(context.Background(), "webauthn passkeys")
	require.NotEqual(t, pgvector.NewVector(descOnly), store.topicEmb[topic.ID],
		"topic embedding must not be description-only")
}

func TestService_CreateTopic_DefaultThreshold(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	topic, err := svc.CreateTopic(context.Background(), 1, radar.CreateTopicRequest{
		Name: "x", Description: "ten chars long",
	})
	require.NoError(t, err)
	require.Equal(t, float32(0.55), topic.MatchThreshold)
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

func TestService_AddFeed_Defaults(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	feed, err := svc.AddFeed(context.Background(), radar.AddFeedRequest{
		URL: "https://example.com/feed.xml",
	})
	require.NoError(t, err)
	require.Equal(t, "rss", feed.Kind)
	require.Equal(t, 3600, feed.FetchIntervalSeconds)
}

func TestService_AddFeed_Validation(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	_, err := svc.AddFeed(context.Background(), radar.AddFeedRequest{URL: ""})
	require.ErrorIs(t, err, radar.ErrInvalidInput)

	_, err = svc.AddFeed(context.Background(), radar.AddFeedRequest{URL: "not-a-url"})
	require.ErrorIs(t, err, radar.ErrInvalidInput)

	bad := "weird"
	_, err = svc.AddFeed(context.Background(), radar.AddFeedRequest{
		URL: "https://x.example/f", Kind: &bad,
	})
	require.ErrorIs(t, err, radar.ErrInvalidInput)

	tooFast := 60
	_, err = svc.AddFeed(context.Background(), radar.AddFeedRequest{
		URL: "https://x.example/f", FetchIntervalSeconds: &tooFast,
	})
	require.ErrorIs(t, err, radar.ErrInvalidInput)
}

func TestService_AddFeed_Duplicate(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	_, err := svc.AddFeed(context.Background(), radar.AddFeedRequest{
		URL: "https://x.example/dup",
	})
	require.NoError(t, err)
	_, err = svc.AddFeed(context.Background(), radar.AddFeedRequest{
		URL: "https://x.example/dup",
	})
	require.ErrorIs(t, err, radar.ErrDuplicate)
}

func TestService_Subscribe_Success(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	feed, err := svc.AddFeed(context.Background(), radar.AddFeedRequest{
		URL: "https://x.example/sub",
	})
	require.NoError(t, err)

	sub, err := svc.Subscribe(context.Background(), 42, radar.SubscribeRequest{FeedID: feed.ID})
	require.NoError(t, err)
	require.Equal(t, int64(42), sub.UserID)
	require.Equal(t, feed.ID, sub.FeedID)
}

func TestService_Subscribe_Idempotent(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	feed, _ := svc.AddFeed(context.Background(), radar.AddFeedRequest{URL: "https://x.example/i"})

	sub1, _ := svc.Subscribe(context.Background(), 1, radar.SubscribeRequest{FeedID: feed.ID})
	sub2, err := svc.Subscribe(context.Background(), 1, radar.SubscribeRequest{FeedID: feed.ID})
	require.NoError(t, err)
	require.Equal(t, sub1.CreatedAt, sub2.CreatedAt)
}

func TestService_Subscribe_FeedMissing(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	_, err := svc.Subscribe(context.Background(), 1, radar.SubscribeRequest{FeedID: 999})
	require.ErrorIs(t, err, radar.ErrFeedNotFound)
}

func TestService_Subscribe_Validation(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	_, err := svc.Subscribe(context.Background(), 1, radar.SubscribeRequest{FeedID: 0})
	require.ErrorIs(t, err, radar.ErrInvalidInput)
}

func (m *mockStore) ListTopicsWithStats(_ context.Context, _ int64) ([]radar.TopicWithStats, error) {
	return m.listTopicsResult, m.listTopicsErr
}

func (m *mockStore) GetTopicWithStats(_ context.Context, _, _ int64) (*radar.TopicWithStats, error) {
	if m.getTopicErr != nil {
		return nil, m.getTopicErr
	}
	if m.getTopicResult == nil {
		return nil, radar.ErrNotFound
	}
	return m.getTopicResult, nil
}

func (m *mockStore) UpdateTopic(_ context.Context, _, _ int64, p radar.UpdateTopicParams) (*radar.Topic, error) {
	m.updateTopicCalled = true
	m.updateTopicParams = p
	if m.updateTopicErr != nil {
		return nil, m.updateTopicErr
	}
	if m.updateTopicResult != nil {
		m.topics[m.updateTopicResult.ID] = m.updateTopicResult
		return m.updateTopicResult, nil
	}
	// Default: synthesize a Topic reflecting params.
	t := radar.Topic{ID: 1, Name: "default"}
	if p.Name != nil {
		t.Name = *p.Name
	}
	if p.Description != nil {
		t.Description = *p.Description
	}
	if p.MatchThreshold != nil {
		t.MatchThreshold = *p.MatchThreshold
	}
	if p.IsActive != nil {
		t.IsActive = *p.IsActive
	}
	m.topics[t.ID] = &t
	return &t, nil
}

func (m *mockStore) DeleteTopic(_ context.Context, _, _ int64) error {
	m.deleteTopicCalled = true
	return m.deleteTopicErr
}

func (m *mockStore) ListMatches(_ context.Context, _ int64, p radar.ListMatchesParams) ([]radar.MatchView, int, error) {
	m.listMatchesCalled = true
	m.listMatchesParams = p
	return m.listMatchesResult, m.listMatchesTotal, m.listMatchesErr
}

func (m *mockStore) GetMatch(_ context.Context, _, _ int64) (*radar.MatchView, error) {
	m.getMatchCalled = true
	return m.getMatchResult, m.getMatchErr
}

func (m *mockStore) UpdateMatchState(_ context.Context, _, _ int64, state string) error {
	m.updateMatchCalled = true
	m.updateMatchState = state
	return m.updateMatchErr
}

func (m *mockStore) LastSweepAt(_ context.Context, _ int64) (*time.Time, error) {
	return m.lastSweepResult, m.lastSweepErr
}

func (m *mockStore) ListFeeds(_ context.Context, userID int64, p radar.ListFeedsParams) ([]radar.FeedListItem, int, error) {
	m.listFeedsUserID = userID
	m.listFeedsParams = p
	return m.listFeedsResult, m.listFeedsTotal, m.listFeedsErr
}

func (m *mockStore) PreviewFindings(
	_ context.Context, userID int64, vec pgvector.Vector, limit int,
) ([]radar.PreviewMatch, error) {
	m.previewCalled = true
	m.previewUserID = userID
	m.previewVec = vec
	m.previewLimit = limit
	return m.previewResult, m.previewErr
}

func TestService_ListTopics_passesThrough(t *testing.T) {
	store := newMockStore()
	store.listTopicsResult = []radar.TopicWithStats{
		{Topic: radar.Topic{ID: 1, Name: "A"}},
		{Topic: radar.Topic{ID: 2, Name: "B"}},
	}
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	got, err := svc.ListTopics(context.Background(), 42)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, int64(1), got[0].ID)
}

func TestService_GetTopic_notFound(t *testing.T) {
	store := newMockStore()
	store.getTopicErr = radar.ErrNotFound
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	_, err := svc.GetTopic(context.Background(), 1, 999)
	require.ErrorIs(t, err, radar.ErrNotFound)
}

func TestService_DeleteTopic_passesThrough(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	require.NoError(t, svc.DeleteTopic(context.Background(), 1, 7))
	require.True(t, store.deleteTopicCalled)

	store.deleteTopicErr = radar.ErrNotFound
	store.deleteTopicCalled = false
	require.ErrorIs(t, svc.DeleteTopic(context.Background(), 1, 7), radar.ErrNotFound)
	require.True(t, store.deleteTopicCalled)
}

func TestService_UpdateTopic_noFields(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	_, err := svc.UpdateTopic(context.Background(), 1, 7, radar.UpdateTopicRequest{})
	require.ErrorIs(t, err, radar.ErrInvalidInput)
	require.False(t, store.updateTopicCalled)
}

func TestService_UpdateTopic_validation(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	emptyName := ""
	_, err := svc.UpdateTopic(context.Background(), 1, 7,
		radar.UpdateTopicRequest{Name: &emptyName})
	require.ErrorIs(t, err, radar.ErrInvalidInput)

	bad := float32(1.5)
	_, err = svc.UpdateTopic(context.Background(), 1, 7,
		radar.UpdateTopicRequest{MatchThreshold: &bad})
	require.ErrorIs(t, err, radar.ErrInvalidInput)

	shortDesc := "tiny"
	_, err = svc.UpdateTopic(context.Background(), 1, 7,
		radar.UpdateTopicRequest{Description: &shortDesc})
	require.ErrorIs(t, err, radar.ErrInvalidInput)

	require.False(t, store.updateTopicCalled)
}

func TestService_UpdateTopic_nameOnly_noEmbed(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	name := "new name"
	got, err := svc.UpdateTopic(context.Background(), 1, 7,
		radar.UpdateTopicRequest{Name: &name})
	require.NoError(t, err)
	require.Equal(t, "new name", got.Name)
	require.True(t, store.updateTopicCalled)
	require.NotNil(t, store.updateTopicParams.Name)
	require.Equal(t, "new name", *store.updateTopicParams.Name)
	require.Nil(t, store.updateTopicParams.Description)
	// embedder was not invoked: UpdateTopicEmbedding stores nothing in mock.
	// Confirm topicEmb is empty.
	require.Empty(t, store.topicEmb)
}

func TestService_UpdateTopic_descriptionTriggersEmbed(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	// updateTopicResult dictates what UpdateTopic returns (used to derive
	// embedder input "name: description").
	store.updateTopicResult = &radar.Topic{
		ID: 7, Name: "name", Description: "new long description here",
	}

	desc := "new long description here"
	got, err := svc.UpdateTopic(context.Background(), 1, 7,
		radar.UpdateTopicRequest{Description: &desc})
	require.NoError(t, err)
	require.Equal(t, "new long description here", got.Description)
	require.True(t, store.updateTopicCalled)
	require.NotEmpty(t, store.topicEmb, "embedder should have written via UpdateTopicEmbedding")
}

func TestService_UpdateTopic_embedderUnavailable(t *testing.T) {
	store := newMockStore()
	store.updateTopicResult = &radar.Topic{
		ID: 7, Name: "name", Description: "new long description here",
	}
	svc := radar.NewService(store, &errEmbedder{err: errors.New("conn refused")})

	desc := "new long description here"
	_, err := svc.UpdateTopic(context.Background(), 1, 7,
		radar.UpdateTopicRequest{Description: &desc})
	require.ErrorIs(t, err, radar.ErrEmbedderUnavailable)
	require.True(t, store.updateTopicCalled, "fields are persisted before embed attempt")
}

func TestService_ListMatches_clampLimit(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	_, err := svc.ListMatches(context.Background(), radar.ListMatchesParams{UserID: 1, Limit: 200})
	require.NoError(t, err)
	require.Equal(t, 100, store.listMatchesParams.Limit)

	store.listMatchesCalled = false
	_, err = svc.ListMatches(context.Background(), radar.ListMatchesParams{UserID: 1, Limit: 0})
	require.NoError(t, err)
	require.Equal(t, 50, store.listMatchesParams.Limit)

	store.listMatchesCalled = false
	_, err = svc.ListMatches(context.Background(), radar.ListMatchesParams{UserID: 1, Limit: 25, Offset: -3})
	require.NoError(t, err)
	require.Equal(t, 25, store.listMatchesParams.Limit)
	require.Equal(t, 0, store.listMatchesParams.Offset)
}

func TestService_ListMatches_returnsResult(t *testing.T) {
	store := newMockStore()
	store.listMatchesResult = []radar.MatchView{{ID: 1}, {ID: 2}}
	store.listMatchesTotal = 2
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	got, err := svc.ListMatches(context.Background(), radar.ListMatchesParams{UserID: 1, Limit: 10})
	require.NoError(t, err)
	require.Len(t, got.Items, 2)
	require.Equal(t, 2, got.Total)
}

func TestService_GetMatch_passesThrough(t *testing.T) {
	store := newMockStore()
	want := &radar.MatchView{ID: 7, TopicID: 3, TopicName: "T"}
	store.getMatchResult = want
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	got, err := svc.GetMatch(context.Background(), 1, 7)
	require.NoError(t, err)
	require.Equal(t, want, got)
	require.True(t, store.getMatchCalled)
}

func TestService_GetMatch_notFound(t *testing.T) {
	store := newMockStore()
	store.getMatchErr = radar.ErrNotFound
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	_, err := svc.GetMatch(context.Background(), 1, 7)
	require.True(t, errors.Is(err, radar.ErrNotFound))
}

func TestService_SetMatchState_validation(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	err := svc.SetMatchState(context.Background(), 1, 9, "foo")
	require.ErrorIs(t, err, radar.ErrInvalidInput)
	require.False(t, store.updateMatchCalled)

	require.NoError(t, svc.SetMatchState(context.Background(), 1, 9, "new"))
	require.NoError(t, svc.SetMatchState(context.Background(), 1, 9, "seen"))
	require.True(t, store.updateMatchCalled)
}

func TestService_SetMatchState_propagatesNotFound(t *testing.T) {
	store := newMockStore()
	store.updateMatchErr = radar.ErrNotFound
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	err := svc.SetMatchState(context.Background(), 1, 9, "seen")
	require.ErrorIs(t, err, radar.ErrNotFound)
}

func TestService_LastSweep_passesThrough(t *testing.T) {
	store := newMockStore()
	when := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	store.lastSweepResult = &when
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	got, err := svc.LastSweep(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, when, *got)
}

func TestService_ListFeeds_clampsPagination(t *testing.T) {
	store := newMockStore()
	store.listFeedsResult = []radar.FeedListItem{{Feed: radar.Feed{ID: 1}}}
	store.listFeedsTotal = 1
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	got, err := svc.ListFeeds(context.Background(), 1, radar.ListFeedsParams{Limit: 0, Offset: -1})
	require.NoError(t, err)
	require.Len(t, got.Items, 1)
	require.Equal(t, 1, got.Total)
}

func TestService_PreviewTopic_ProbesLikeCreateTopic(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	got, err := svc.PreviewTopic(context.Background(), 7, radar.PreviewTopicRequest{
		Name: "WebAuthn", Description: "webauthn passkeys",
	})
	require.NoError(t, err)
	require.Equal(t, float32(0.55), got.Threshold)

	require.True(t, store.previewCalled)
	require.Equal(t, int64(7), store.previewUserID)
	require.Equal(t, 5, store.previewLimit)

	expected, _ := emb.Embed(context.Background(), "WebAuthn: webauthn passkeys")
	require.Equal(t, pgvector.NewVector(expected), store.previewVec,
		"preview must probe with the same text CreateTopic embeds")
}

func TestService_PreviewTopic_WithoutNameProbesDescriptionOnly(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	_, err := svc.PreviewTopic(context.Background(), 1, radar.PreviewTopicRequest{
		Description: "webauthn passkeys",
	})
	require.NoError(t, err)

	expected, _ := emb.Embed(context.Background(), "webauthn passkeys")
	require.Equal(t, pgvector.NewVector(expected), store.previewVec)
}

func TestService_PreviewTopic_ReturnsScoredFindings(t *testing.T) {
	store := newMockStore()
	store.previewResult = []radar.PreviewMatch{
		{Similarity: 0.81, Finding: radar.MatchFinding{ID: 3, URL: "https://x.example/a"}},
		{Similarity: 0.42, Finding: radar.MatchFinding{ID: 4, URL: "https://x.example/b"}},
	}
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	got, err := svc.PreviewTopic(context.Background(), 1, radar.PreviewTopicRequest{
		Description: "ten chars long",
	})
	require.NoError(t, err)
	require.Len(t, got.Items, 2)
	require.Equal(t, float32(0.81), got.Items[0].Similarity)
	require.Equal(t, int64(4), got.Items[1].Finding.ID)
}

func TestService_PreviewTopic_Validation(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	_, err := svc.PreviewTopic(context.Background(), 1, radar.PreviewTopicRequest{
		Description: "short",
	})
	require.ErrorIs(t, err, radar.ErrInvalidInput)

	_, err = svc.PreviewTopic(context.Background(), 1, radar.PreviewTopicRequest{
		Name: strings.Repeat("x", 201), Description: "ten chars long",
	})
	require.ErrorIs(t, err, radar.ErrInvalidInput)

	require.False(t, store.previewCalled, "invalid drafts must not reach the embedder or store")
}

func TestService_PreviewTopic_EmbedderError(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &errEmbedder{err: errors.New("boom")})

	_, err := svc.PreviewTopic(context.Background(), 1, radar.PreviewTopicRequest{
		Description: "ten chars long",
	})
	require.ErrorIs(t, err, radar.ErrEmbedderUnavailable)
}

func (m *mockStore) Unsubscribe(_ context.Context, userID, feedID int64) error {
	if m.unsubscribeErr != nil {
		return m.unsubscribeErr
	}

	delete(m.subs, keyOf(userID, feedID))
	return nil
}

func (m *mockStore) UpdateFeed(_ context.Context, feedID int64, p radar.UpdateFeedParams) (*radar.Feed, error) {
	m.updateFeedCalled = true
	m.updateFeedParams = p

	if m.updateFeedErr != nil {
		return nil, m.updateFeedErr
	}

	f, ok := m.feeds[feedID]
	if !ok {
		return &radar.Feed{ID: feedID}, nil
	}

	if p.Title != nil {
		f.Title = p.Title
	}
	if p.FetchIntervalSeconds != nil {
		f.FetchIntervalSeconds = *p.FetchIntervalSeconds
	}
	if p.IsActive != nil {
		f.IsActive = *p.IsActive
	}

	return f, nil
}

func (m *mockStore) DeleteFeed(_ context.Context, feedID int64) error {
	m.deleteFeedCalled = true

	if m.deleteFeedErr != nil {
		return m.deleteFeedErr
	}

	f, ok := m.feeds[feedID]
	if !ok {
		return radar.ErrNotFound
	}

	delete(m.feedsByURL, f.URL)
	delete(m.feeds, feedID)
	return nil
}

func TestService_UpdateFeed_Validation(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	ctx := context.Background()

	_, err := svc.UpdateFeed(ctx, 1, radar.UpdateFeedRequest{})
	require.ErrorIs(t, err, radar.ErrInvalidInput)

	tooFast := 60
	_, err = svc.UpdateFeed(ctx, 1, radar.UpdateFeedRequest{FetchIntervalSeconds: &tooFast})
	require.ErrorIs(t, err, radar.ErrInvalidInput)

	tooSlow := 999999
	_, err = svc.UpdateFeed(ctx, 1, radar.UpdateFeedRequest{FetchIntervalSeconds: &tooSlow})
	require.ErrorIs(t, err, radar.ErrInvalidInput)

	ok := 1800
	paused := false
	_, err = svc.UpdateFeed(ctx, 1, radar.UpdateFeedRequest{
		FetchIntervalSeconds: &ok, IsActive: &paused,
	})
	require.NoError(t, err)
	require.Equal(t, 1800, *store.updateFeedParams.FetchIntervalSeconds)
	require.False(t, *store.updateFeedParams.IsActive)
}
