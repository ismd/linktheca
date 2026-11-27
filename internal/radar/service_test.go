package radar_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
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
	topicMatchEmb  map[int64]pgvector.Vector
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
	getFindingResult  *radar.MatchFinding
	getFindingErr     error
	getFindingUserID  int64
	getFindingID      int64
	previewResult     []radar.PreviewMatch
	previewErr        error
	previewCalled     bool
	previewUserID     int64
	previewModel      radar.TopicModel
	previewExclude    []int64
	previewLimit      int

	// Learning from marks: topic -> finding -> verdict, and the match vectors
	// of findings the user can see.
	topicModels       map[int64]radar.TopicModel
	feedback          map[int64]map[int64]string
	findingVecs       map[int64][]float32
	replaceFeedback   []radar.TopicFeedback
	replaceCalled     bool
	examplesUserID    int64
	listFeedbackItems []radar.TopicFeedbackView
	streamMean        []float32
	streamMeanCalls   int
	unsubscribeErr    error
	updateFeedCalled  bool
	updateFeedParams  radar.UpdateFeedParams
	updateFeedOwner   *int64
	updateFeedErr     error
	deleteFeedCalled  bool
	deleteFeedOwner   *int64
	deleteFeedErr     error
	seedErr           error
	feedOwners        map[int64]*int64
}

func newMockStore() *mockStore {
	return &mockStore{
		topics:        make(map[int64]*radar.Topic),
		topicEmb:      make(map[int64]pgvector.Vector),
		topicMatchEmb: make(map[int64]pgvector.Vector),
		topicModels:   make(map[int64]radar.TopicModel),
		feedback:      make(map[int64]map[int64]string),
		findingVecs:   make(map[int64][]float32),
		feeds:         make(map[int64]*radar.Feed),
		feedsByURL:    make(map[string]*radar.Feed),
		feedOwners:    make(map[int64]*int64),
		subs:          make(map[string]*radar.Subscription),
	}
}

func (m *mockStore) CreateTopic(_ context.Context, p radar.CreateTopicParams) (*radar.Topic, error) {
	if m.createTopicErr != nil {
		return nil, m.createTopicErr
	}
	m.nextTopicID++
	t := &radar.Topic{
		ID: m.nextTopicID, UserID: p.UserID, Name: p.Name,
		Description: p.Description, MatchThreshold: p.MatchThreshold, MatchCutoff: p.MatchThreshold,
		IsActive: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	m.topics[t.ID] = t
	return t, nil
}

func (m *mockStore) UpdateTopicEmbedding(_ context.Context, topicID int64, vec pgvector.Vector, model radar.TopicModel) error {
	if m.updateEmbErr != nil {
		return m.updateEmbErr
	}
	t, ok := m.topics[topicID]
	if !ok {
		return radar.ErrNotFound
	}
	m.topicEmb[topicID] = vec
	m.topicMatchEmb[topicID] = pgvector.NewVector(model.MatchVec)
	m.topicModels[topicID] = model
	t.HasEmbedding = true
	t.MatchCutoff = model.Cutoff
	return nil
}

func (m *mockStore) GetTopicLearnInput(_ context.Context, topicID int64) (*radar.TopicLearnInput, error) {
	t, ok := m.topics[topicID]
	if !ok {
		return nil, radar.ErrNotFound
	}
	in := &radar.TopicLearnInput{ID: t.ID, Description: t.Description, MatchThreshold: t.MatchThreshold}
	if vec, ok := m.topicEmb[topicID]; ok {
		in.Embedding = &vec
	}
	return in, nil
}

func (m *mockStore) ListTopicExamples(_ context.Context, topicID int64) ([]radar.Example, error) {
	var out []radar.Example
	for id, verdict := range m.feedback[topicID] {
		if vec, ok := m.findingVecs[id]; ok {
			out = append(out, radar.Example{FindingID: id, Vec: vec, Relevant: verdict == radar.VerdictRelevant})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FindingID < out[j].FindingID })
	return out, nil
}

func (m *mockStore) FindingExamples(_ context.Context, userID int64, fb []radar.TopicFeedback) ([]radar.Example, error) {
	m.examplesUserID = userID
	var out []radar.Example
	for _, f := range fb {
		if vec, ok := m.findingVecs[f.FindingID]; ok {
			out = append(out, radar.Example{FindingID: f.FindingID, Vec: vec, Relevant: f.Verdict == radar.VerdictRelevant})
		}
	}
	return out, nil
}

func (m *mockStore) SetTopicFeedback(_ context.Context, _, topicID, findingID int64, verdict string) error {
	if _, ok := m.topics[topicID]; !ok {
		return radar.ErrNotFound
	}
	if _, ok := m.findingVecs[findingID]; !ok {
		return radar.ErrNotFound
	}
	if m.feedback[topicID] == nil {
		m.feedback[topicID] = map[int64]string{}
	}
	m.feedback[topicID][findingID] = verdict
	return nil
}

func (m *mockStore) ReplaceTopicFeedback(_ context.Context, _, topicID int64, fb []radar.TopicFeedback) error {
	m.replaceCalled = true
	m.replaceFeedback = fb
	m.feedback[topicID] = map[int64]string{}
	for _, f := range fb {
		m.feedback[topicID][f.FindingID] = f.Verdict
	}
	return nil
}

func (m *mockStore) DeleteTopicFeedback(_ context.Context, _, topicID, findingID int64) error {
	if _, ok := m.feedback[topicID][findingID]; !ok {
		return radar.ErrNotFound
	}
	delete(m.feedback[topicID], findingID)
	return nil
}

func (m *mockStore) ListTopicFeedback(_ context.Context, _, _ int64) ([]radar.TopicFeedbackView, error) {
	return m.listFeedbackItems, nil
}

func (m *mockStore) StreamMean(_ context.Context, _ int) ([]float32, error) {
	m.streamMeanCalls++
	return m.streamMean, nil
}

func (m *mockStore) AddFeed(_ context.Context, p radar.AddFeedParams) (*radar.Feed, error) {
	if m.addFeedErr != nil {
		return nil, m.addFeedErr
	}
	key := feedKey(p.URL, p.OwnerUserID)
	if _, ok := m.feedsByURL[key]; ok {
		return nil, radar.ErrDuplicate
	}
	m.nextFeedID++
	f := &radar.Feed{
		ID: m.nextFeedID, URL: p.URL, Kind: p.Kind,
		FetchIntervalSeconds: p.FetchIntervalSeconds, IsActive: true,
		CreatedAt: time.Now(),
	}
	m.feeds[f.ID] = f
	m.feedsByURL[key] = f
	m.feedOwners[f.ID] = p.OwnerUserID
	return f, nil
}

func (m *mockStore) GetGlobalFeedByURL(_ context.Context, url string) (*radar.Feed, error) {
	if f, ok := m.feedsByURL[feedKey(url, nil)]; ok {
		return f, nil
	}

	return nil, radar.ErrNotFound
}

func (m *mockStore) CountUserFeeds(_ context.Context, userID int64) (int, error) {
	n := 0

	for _, owner := range m.feedOwners {
		if owner != nil && *owner == userID {
			n++
		}
	}

	return n, nil
}

// feedKey mirrors the partial unique indexes: one global row per URL, one
// personal row per (URL, owner).
func feedKey(url string, owner *int64) string {
	if owner == nil {
		return "global:" + url
	}
	return fmt.Sprintf("user:%d:%s", *owner, url)
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

func (m *mockStore) hasSubscription(userID, feedID int64) bool {
	_, ok := m.subs[keyOf(userID, feedID)]
	return ok
}

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

func TestService_CreateTopic_EmbedsDescriptionOnly(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	topic, err := svc.CreateTopic(context.Background(), 1, radar.CreateTopicRequest{
		Name: "WebAuthn", Description: "webauthn passkeys",
	})
	require.NoError(t, err)

	expected, _ := emb.Embed(context.Background(), "webauthn passkeys")
	require.Equal(t, pgvector.NewVector(expected), store.topicEmb[topic.ID],
		"the name is a label; only the description is embedded")
}

// A topic is matched with its embedding minus the reference centroid.
func TestService_CreateTopic_StoresMatchVector(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)
	ctx := context.Background()

	topic, err := svc.CreateTopic(ctx, 1, radar.CreateTopicRequest{Description: "webauthn passkeys"})
	require.NoError(t, err)

	raw, _ := emb.Embed(ctx, "webauthn passkeys")
	centroid, err := radar.NewReference(emb).Centroid(ctx)
	require.NoError(t, err)

	got := store.topicMatchEmb[topic.ID].Slice()
	require.Len(t, got, len(raw))
	for i := range raw {
		require.InDelta(t, raw[i]-centroid[i], got[i], 1e-6)
	}
}

// The reference centroid is embedded once and shared through WithReference.
func TestService_WithReference_sharesTheCentroid(t *testing.T) {
	counter := &countingEmbedder{inner: &embeddings.FakeEmbedder{Dim: 1024}}
	ref := radar.NewReference(counter)
	ctx := context.Background()

	_, err := ref.Centroid(ctx)
	require.NoError(t, err)
	afterWarmup := counter.calls

	svc := radar.NewService(newMockStore(), counter, radar.WithReference(ref))
	_, err = svc.PreviewTopic(ctx, 1, radar.PreviewTopicRequest{Description: "webauthn passkeys"})
	require.NoError(t, err)
	require.Equal(t, afterWarmup+1, counter.calls, "only the description is embedded")
}

type countingEmbedder struct {
	inner embeddings.Client
	calls int
}

func (c *countingEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	c.calls++
	return c.inner.Embed(ctx, text)
}

func TestService_CreateTopic_DerivesNameFromDescription(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	topic, err := svc.CreateTopic(context.Background(), 1, radar.CreateTopicRequest{
		Description: "  Open-weight LLM releases: Llama, Qwen, Mistral  ",
	})
	require.NoError(t, err)
	require.Equal(t, "Open-weight LLM releases", topic.Name)
	require.Equal(t, "Open-weight LLM releases: Llama, Qwen, Mistral", topic.Description)
}

func TestService_CreateTopic_KeepsExplicitName(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	topic, err := svc.CreateTopic(context.Background(), 1, radar.CreateTopicRequest{
		Name: "  LLMs ", Description: "Open-weight LLM releases",
	})
	require.NoError(t, err)
	require.Equal(t, "LLMs", topic.Name)
}

func TestService_CreateTopic_DefaultThreshold(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	topic, err := svc.CreateTopic(context.Background(), 1, radar.CreateTopicRequest{
		Name: "x", Description: "ten chars long",
	})
	require.NoError(t, err)
	require.Equal(t, float32(0.18), topic.MatchThreshold)
}

func TestService_CreateTopic_Validation(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	// name too long
	_, err := svc.CreateTopic(context.Background(), 1, radar.CreateTopicRequest{
		Name: strings.Repeat("x", 201), Description: "ten chars long",
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

func TestService_AddGlobalFeed_Defaults(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	feed, err := svc.AddGlobalFeed(context.Background(), radar.AddFeedRequest{
		URL: "https://example.com/feed.xml",
	})
	require.NoError(t, err)
	require.Equal(t, "rss", feed.Kind)
	require.Equal(t, 3600, feed.FetchIntervalSeconds)
}

func TestService_AddGlobalFeed_Validation(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	_, err := svc.AddGlobalFeed(context.Background(), radar.AddFeedRequest{URL: ""})
	require.ErrorIs(t, err, radar.ErrInvalidInput)

	_, err = svc.AddGlobalFeed(context.Background(), radar.AddFeedRequest{URL: "not-a-url"})
	require.ErrorIs(t, err, radar.ErrInvalidInput)

	bad := "weird"
	_, err = svc.AddGlobalFeed(context.Background(), radar.AddFeedRequest{
		URL: "https://x.example/f", Kind: &bad,
	})
	require.ErrorIs(t, err, radar.ErrInvalidInput)

	tooFast := 60
	_, err = svc.AddGlobalFeed(context.Background(), radar.AddFeedRequest{
		URL: "https://x.example/f", FetchIntervalSeconds: &tooFast,
	})
	require.ErrorIs(t, err, radar.ErrInvalidInput)
}

func TestService_AddGlobalFeed_Duplicate(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	_, err := svc.AddGlobalFeed(context.Background(), radar.AddFeedRequest{
		URL: "https://x.example/dup",
	})
	require.NoError(t, err)
	_, err = svc.AddGlobalFeed(context.Background(), radar.AddFeedRequest{
		URL: "https://x.example/dup",
	})
	require.ErrorIs(t, err, radar.ErrDuplicate)
}

func TestService_AddUserFeed_ReusesCatalogFeed(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	ctx := context.Background()

	// A catalog feed already covers this URL.
	catalog, err := svc.AddGlobalFeed(ctx, radar.AddFeedRequest{URL: "https://shared.example/rss"})
	require.NoError(t, err)

	res, err := svc.AddUserFeed(ctx, 42, radar.AddFeedRequest{URL: "https://shared.example/rss"})
	require.NoError(t, err)
	require.False(t, res.Created, "no personal row is created for a catalog URL")
	require.Equal(t, catalog.ID, res.Feed.ID)
	require.True(t, store.hasSubscription(42, catalog.ID))
}

func TestService_AddUserFeed_CreatesAndSubscribes(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	res, err := svc.AddUserFeed(context.Background(), 42,
		radar.AddFeedRequest{URL: "https://mine.example/rss"})
	require.NoError(t, err)
	require.True(t, res.Created)
	require.True(t, store.hasSubscription(42, res.Feed.ID))
}

func TestService_AddUserFeed_QuotaExceeded(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024},
		radar.WithMaxUserFeeds(1))
	ctx := context.Background()

	_, err := svc.AddUserFeed(ctx, 42, radar.AddFeedRequest{URL: "https://a.example/rss"})
	require.NoError(t, err)

	_, err = svc.AddUserFeed(ctx, 42, radar.AddFeedRequest{URL: "https://b.example/rss"})
	require.ErrorIs(t, err, radar.ErrQuotaExceeded)
}

func TestService_UserFeedWritesCarryOwner(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	ctx := context.Background()

	title := "Renamed"
	_, err := svc.UpdateUserFeed(ctx, 42, 1, radar.UpdateFeedRequest{Title: &title})
	require.NoError(t, err)
	require.NotNil(t, store.updateFeedOwner)
	require.Equal(t, int64(42), *store.updateFeedOwner)

	_, err = svc.UpdateGlobalFeed(ctx, 1, radar.UpdateFeedRequest{Title: &title})
	require.NoError(t, err)
	require.Nil(t, store.updateFeedOwner, "the admin scope addresses catalog rows")
}

func TestService_Subscribe_Success(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)

	feed, err := svc.AddGlobalFeed(context.Background(), radar.AddFeedRequest{
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

	feed, _ := svc.AddGlobalFeed(context.Background(), radar.AddFeedRequest{URL: "https://x.example/i"})

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

func (m *mockStore) GetFinding(_ context.Context, userID, findingID int64) (*radar.MatchFinding, error) {
	m.getFindingUserID = userID
	m.getFindingID = findingID
	return m.getFindingResult, m.getFindingErr
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
	_ context.Context, userID int64, model radar.TopicModel, exclude []int64, limit int,
) ([]radar.PreviewMatch, error) {
	m.previewCalled = true
	m.previewUserID = userID
	m.previewModel = model
	m.previewExclude = exclude
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

	// updateTopicResult dictates what UpdateTopic returns, and so the
	// description the embedder is fed.
	store.updateTopicResult = &radar.Topic{
		ID: 7, Name: "name", Description: "new long description here",
	}

	desc := "new long description here"
	got, err := svc.UpdateTopic(context.Background(), 1, 7,
		radar.UpdateTopicRequest{Description: &desc})
	require.NoError(t, err)
	require.Equal(t, "new long description here", got.Description)
	require.True(t, store.updateTopicCalled)

	expected, _ := emb.Embed(context.Background(), "new long description here")
	require.Equal(t, pgvector.NewVector(expected), store.topicEmb[7])

	match, err := radar.NewReference(emb).TopicMatchVector(context.Background(), expected)
	require.NoError(t, err)
	require.Equal(t, pgvector.NewVector(match), store.topicMatchEmb[7],
		"an edited description gets a new match vector too")
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

func TestService_GetMatch_sanitizesArticleHTML(t *testing.T) {
	store := newMockStore()
	html := `<p>Article</p><script>alert(1)</script><img src="https://x.example/a.png" onerror="alert(2)">`
	text := "Article"
	store.getMatchResult = &radar.MatchView{ID: 7, Finding: radar.MatchFinding{
		Content: &radar.FindingContent{HTML: &html, Text: &text},
	}}
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	got, err := svc.GetMatch(context.Background(), 1, 7)
	require.NoError(t, err)

	require.Equal(t, `<p>Article</p><img src="https://x.example/a.png">`, *got.Finding.Content.HTML)
	require.Equal(t, "Article", *got.Finding.Content.Text)
}

func TestService_GetMatch_dropsHTMLThatSanitizesToNothing(t *testing.T) {
	store := newMockStore()
	html := `<script>alert(1)</script>`
	store.getMatchResult = &radar.MatchView{ID: 7, Finding: radar.MatchFinding{
		Content: &radar.FindingContent{HTML: &html},
	}}
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	got, err := svc.GetMatch(context.Background(), 1, 7)
	require.NoError(t, err)
	require.Nil(t, got.Finding.Content.HTML)
}

func TestService_GetMatch_notFound(t *testing.T) {
	store := newMockStore()
	store.getMatchErr = radar.ErrNotFound
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	_, err := svc.GetMatch(context.Background(), 1, 7)
	require.True(t, errors.Is(err, radar.ErrNotFound))
}

func TestService_GetFinding_scopesToUser(t *testing.T) {
	store := newMockStore()
	want := &radar.MatchFinding{ID: 100, URL: "https://x.example/a"}
	store.getFindingResult = want
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	got, err := svc.GetFinding(context.Background(), 3, 100)
	require.NoError(t, err)
	require.Equal(t, want, got)
	require.Equal(t, int64(3), store.getFindingUserID)
	require.Equal(t, int64(100), store.getFindingID)
}

func TestService_GetFinding_sanitizesArticleHTML(t *testing.T) {
	store := newMockStore()
	html := `<p>Article</p><script>alert(1)</script>`
	store.getFindingResult = &radar.MatchFinding{
		Content: &radar.FindingContent{HTML: &html},
	}
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	got, err := svc.GetFinding(context.Background(), 1, 100)
	require.NoError(t, err)
	require.Equal(t, `<p>Article</p>`, *got.Content.HTML)
}

func TestService_GetFinding_notFound(t *testing.T) {
	store := newMockStore()
	store.getFindingErr = radar.ErrNotFound
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})

	_, err := svc.GetFinding(context.Background(), 1, 100)
	require.ErrorIs(t, err, radar.ErrNotFound)
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
		Description: "  webauthn passkeys ",
	})
	require.NoError(t, err)
	require.Equal(t, float32(0.18), got.Threshold)

	require.True(t, store.previewCalled)
	require.Equal(t, int64(7), store.previewUserID)
	require.Equal(t, 10, store.previewLimit)
	require.Empty(t, store.previewExclude)

	raw, _ := emb.Embed(context.Background(), radar.TopicEmbedText("webauthn passkeys"))
	expected, err := radar.NewReference(emb).TopicMatchVector(context.Background(), raw)
	require.NoError(t, err)
	require.Equal(t, expected, store.previewModel.MatchVec,
		"preview must probe with the match vector CreateTopic stores")
	require.Zero(t, store.previewModel.Offset)
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

func (m *mockStore) UpdateFeed(_ context.Context, feedID int64, owner *int64, p radar.UpdateFeedParams) (*radar.Feed, error) {
	m.updateFeedCalled = true
	m.updateFeedParams = p
	m.updateFeedOwner = owner

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

func (m *mockStore) DeleteFeed(_ context.Context, feedID int64, owner *int64) error {
	m.deleteFeedCalled = true
	m.deleteFeedOwner = owner

	if m.deleteFeedErr != nil {
		return m.deleteFeedErr
	}

	f, ok := m.feeds[feedID]
	if !ok {
		return radar.ErrNotFound
	}

	delete(m.feedsByURL, feedKey(f.URL, m.feedOwners[feedID]))
	delete(m.feedOwners, feedID)
	delete(m.feeds, feedID)

	return nil
}

func (m *mockStore) SeedSubscriptions(_ context.Context, userID int64) (int, error) {
	if m.seedErr != nil {
		return 0, m.seedErr
	}

	n := 0
	for id, f := range m.feeds {
		if !f.IsActive {
			continue
		}
		key := keyOf(userID, id)
		if _, ok := m.subs[key]; ok {
			continue
		}
		m.subs[key] = &radar.Subscription{UserID: userID, FeedID: id, CreatedAt: time.Now()}
		n++
	}

	return n, nil
}

func TestService_UpdateGlobalFeed_Validation(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	ctx := context.Background()

	_, err := svc.UpdateGlobalFeed(ctx, 1, radar.UpdateFeedRequest{})
	require.ErrorIs(t, err, radar.ErrInvalidInput)

	tooFast := 60
	_, err = svc.UpdateGlobalFeed(ctx, 1, radar.UpdateFeedRequest{FetchIntervalSeconds: &tooFast})
	require.ErrorIs(t, err, radar.ErrInvalidInput)

	tooSlow := 999999
	_, err = svc.UpdateGlobalFeed(ctx, 1, radar.UpdateFeedRequest{FetchIntervalSeconds: &tooSlow})
	require.ErrorIs(t, err, radar.ErrInvalidInput)

	ok := 1800
	paused := false
	_, err = svc.UpdateGlobalFeed(ctx, 1, radar.UpdateFeedRequest{
		FetchIntervalSeconds: &ok, IsActive: &paused,
	})
	require.NoError(t, err)
	require.Equal(t, 1800, *store.updateFeedParams.FetchIntervalSeconds)
	require.False(t, *store.updateFeedParams.IsActive)
}

// --- learning from marks ---

// markedFindings gives the mock two findings the user can see: one the fake
// embedder makes close to "webauthn passkeys", one unrelated.
func markedFindings(t *testing.T, store *mockStore, emb embeddings.Client) (near, far int64) {
	t.Helper()
	ctx := context.Background()

	q, _ := emb.Embed(ctx, "webauthn passkeys")
	other, _ := emb.Embed(ctx, "chess openings")
	store.findingVecs[1] = q
	store.findingVecs[2] = other
	return 1, 2
}

func TestService_CreateTopic_LearnsFromMarks(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)
	near, _ := markedFindings(t, store, emb)

	topic, err := svc.CreateTopic(context.Background(), 1, radar.CreateTopicRequest{
		Description: "webauthn passkeys",
		Feedback: []radar.TopicFeedback{
			{FindingID: near, Verdict: radar.VerdictRelevant},
			{FindingID: near, Verdict: radar.VerdictIrrelevant},
		},
	})
	require.NoError(t, err)

	require.Equal(t, []radar.TopicFeedback{{FindingID: near, Verdict: radar.VerdictIrrelevant}},
		store.replaceFeedback, "a finding marked twice keeps the last verdict")

	model := store.topicModels[topic.ID]
	require.Greater(t, model.Cutoff, float32(0.18), "the rejected finding lifted the cutoff")
	require.Less(t, model.Score(store.findingVecs[near]), model.Cutoff)
	require.Equal(t, model.Cutoff, topic.MatchCutoff)
	require.Equal(t, float32(0.18), topic.MatchThreshold, "the user's threshold is kept apart")
}

func TestService_CreateTopic_RejectsBadMarks(t *testing.T) {
	svc := radar.NewService(newMockStore(), &embeddings.FakeEmbedder{Dim: 1024})
	ctx := context.Background()

	for _, fb := range [][]radar.TopicFeedback{
		{{FindingID: 1, Verdict: "maybe"}},
		{{FindingID: 0, Verdict: radar.VerdictRelevant}},
		make([]radar.TopicFeedback, 201),
	} {
		_, err := svc.CreateTopic(ctx, 1, radar.CreateTopicRequest{Description: "webauthn passkeys", Feedback: fb})
		require.ErrorIs(t, err, radar.ErrInvalidInput)
	}
}

func TestService_PreviewTopic_LearnsFromMarksAndHidesRejected(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)
	near, far := markedFindings(t, store, emb)

	got, err := svc.PreviewTopic(context.Background(), 7, radar.PreviewTopicRequest{
		Description: "webauthn passkeys",
		Feedback: []radar.TopicFeedback{
			{FindingID: near, Verdict: radar.VerdictIrrelevant},
			{FindingID: far, Verdict: radar.VerdictRelevant},
		},
	})
	require.NoError(t, err)

	require.Equal(t, int64(7), store.examplesUserID, "only the user's own findings teach the draft")
	require.Equal(t, []int64{near}, store.previewExclude)
	require.Equal(t, store.previewModel.Cutoff, got.Threshold)
	require.Greater(t, got.Threshold, float32(0.18))
}

func TestService_SetTopicFeedback_RelearnsTheTopic(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)
	ctx := context.Background()
	near, _ := markedFindings(t, store, emb)

	topic, err := svc.CreateTopic(ctx, 1, radar.CreateTopicRequest{Description: "webauthn passkeys"})
	require.NoError(t, err)
	require.Equal(t, float32(0.18), store.topicModels[topic.ID].Cutoff)

	require.NoError(t, svc.SetTopicFeedback(ctx, 1, topic.ID, near, radar.VerdictIrrelevant))
	raised := store.topicModels[topic.ID].Cutoff
	require.Greater(t, raised, float32(0.18))

	require.NoError(t, svc.DeleteTopicFeedback(ctx, 1, topic.ID, near))
	require.Equal(t, float32(0.18), store.topicModels[topic.ID].Cutoff, "taking the mark back undoes it")

	require.ErrorIs(t, svc.SetTopicFeedback(ctx, 1, topic.ID, near, "maybe"), radar.ErrInvalidInput)
	require.ErrorIs(t, svc.SetTopicFeedback(ctx, 1, topic.ID, 99, radar.VerdictRelevant), radar.ErrNotFound)
	require.ErrorIs(t, svc.DeleteTopicFeedback(ctx, 1, topic.ID, near), radar.ErrNotFound)
}

func TestService_UpdateTopic_FeedbackOnlyRelearnsWithoutEmbedding(t *testing.T) {
	store := newMockStore()
	inner := &embeddings.FakeEmbedder{Dim: 1024}
	counter := &countingEmbedder{inner: inner}
	svc := radar.NewService(store, counter)
	ctx := context.Background()
	near, _ := markedFindings(t, store, inner)

	topic, err := svc.CreateTopic(ctx, 1, radar.CreateTopicRequest{Description: "webauthn passkeys"})
	require.NoError(t, err)
	store.getTopicResult = &radar.TopicWithStats{Topic: *topic}
	embedded := counter.calls

	fb := []radar.TopicFeedback{{FindingID: near, Verdict: radar.VerdictIrrelevant}}
	got, err := svc.UpdateTopic(ctx, 1, topic.ID, radar.UpdateTopicRequest{Feedback: &fb})
	require.NoError(t, err)

	require.False(t, store.updateTopicCalled, "no topic field changed")
	require.True(t, store.replaceCalled)
	require.Equal(t, embedded, counter.calls, "the stored embedding is reused")
	require.Greater(t, got.MatchCutoff, float32(0.18))

	// An empty list forgets the marks.
	empty := []radar.TopicFeedback{}
	got, err = svc.UpdateTopic(ctx, 1, topic.ID, radar.UpdateTopicRequest{Feedback: &empty})
	require.NoError(t, err)
	require.Equal(t, float32(0.18), got.MatchCutoff)
}

func TestService_UpdateTopic_ThresholdMovesTheCutoff(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)
	ctx := context.Background()

	topic, err := svc.CreateTopic(ctx, 1, radar.CreateTopicRequest{Description: "webauthn passkeys"})
	require.NoError(t, err)

	thr := float32(0.3)
	store.updateTopicResult = &radar.Topic{ID: topic.ID, Description: topic.Description, MatchThreshold: thr}
	got, err := svc.UpdateTopic(ctx, 1, topic.ID, radar.UpdateTopicRequest{MatchThreshold: &thr})
	require.NoError(t, err)
	require.Equal(t, float32(0.3), got.MatchCutoff)
}

func TestService_StreamMeanIsFetchedForConfirmedMarksOnlyAndCached(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)
	ctx := context.Background()
	near, far := markedFindings(t, store, emb)
	store.streamMean = make([]float32, 1024)
	store.streamMean[0] = 0.1

	topic, err := svc.CreateTopic(ctx, 1, radar.CreateTopicRequest{Description: "webauthn passkeys"})
	require.NoError(t, err)

	require.NoError(t, svc.SetTopicFeedback(ctx, 1, topic.ID, near, radar.VerdictIrrelevant))
	require.Zero(t, store.streamMeanCalls, "✗ alone does not move the topic")

	// ✓ on an unrelated finding moves the topic away from its description.
	require.NoError(t, svc.SetTopicFeedback(ctx, 1, topic.ID, far, radar.VerdictRelevant))
	require.NoError(t, svc.DeleteTopicFeedback(ctx, 1, topic.ID, near))
	require.Equal(t, 1, store.streamMeanCalls)
	require.NotZero(t, store.topicModels[topic.ID].Offset)
}

func TestService_ListTopicFeedback_checksTheTopic(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	ctx := context.Background()

	_, err := svc.ListTopicFeedback(ctx, 1, 7)
	require.ErrorIs(t, err, radar.ErrNotFound)

	store.getTopicResult = &radar.TopicWithStats{Topic: radar.Topic{ID: 7}}
	store.listFeedbackItems = []radar.TopicFeedbackView{{Verdict: radar.VerdictRelevant}}
	items, err := svc.ListTopicFeedback(ctx, 1, 7)
	require.NoError(t, err)
	require.Len(t, items, 1)
}

func TestService_ReembedTopic_KeepsWhatMarksTaught(t *testing.T) {
	store := newMockStore()
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)
	ctx := context.Background()
	near, _ := markedFindings(t, store, emb)

	topic, err := svc.CreateTopic(ctx, 1, radar.CreateTopicRequest{
		Description: "webauthn passkeys",
		Feedback:    []radar.TopicFeedback{{FindingID: near, Verdict: radar.VerdictIrrelevant}},
	})
	require.NoError(t, err)
	learned := store.topicModels[topic.ID]

	require.NoError(t, svc.ReembedTopic(ctx, topic.ID))
	require.Equal(t, learned, store.topicModels[topic.ID])
}
