package radar_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/ismd/linktheca/internal/radar"
	"github.com/stretchr/testify/require"
)

// userOnlyContext attaches a user_id to ctx without parsing a JWT.
func userOnlyContext(ctx context.Context, userID int64, isAdmin bool) context.Context {
	return coreauthWithUser(ctx, userID, isAdmin)
}

// withRouteID attaches a chi.RouteContext with the named URL param to ctx.
// Use for handlers that read chi.URLParam(r, "id").
func withRouteID(ctx context.Context, id string) context.Context {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return context.WithValue(ctx, chi.RouteCtxKey, rctx)
}

func TestHTTP_CreateTopic_201(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	body, _ := json.Marshal(radar.CreateTopicRequest{
		Name: "AI", Description: "machine learning research and products",
	})
	req := httptest.NewRequest(http.MethodPost, "/radar/topics", bytes.NewReader(body))
	req = req.WithContext(userOnlyContext(req.Context(), 7, false))
	rec := httptest.NewRecorder()

	h.CreateTopicHandler()(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var got radar.Topic
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
	require.Equal(t, "AI", got.Name)
	require.True(t, got.HasEmbedding)
}

func TestHTTP_CreateTopic_400_BadJSON(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodPost, "/radar/topics",
		strings.NewReader(`{"name":}`))
	req = req.WithContext(userOnlyContext(req.Context(), 1, false))
	rec := httptest.NewRecorder()
	h.CreateTopicHandler()(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHTTP_CreateTopic_400_Validation(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	body, _ := json.Marshal(radar.CreateTopicRequest{Name: "", Description: "short"})
	req := httptest.NewRequest(http.MethodPost, "/radar/topics", bytes.NewReader(body))
	req = req.WithContext(userOnlyContext(req.Context(), 1, false))
	rec := httptest.NewRecorder()
	h.CreateTopicHandler()(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHTTP_CreateTopic_503_EmbedderDown(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &errEmbedder{err: errors.New("connection refused")})
	h := radar.NewHTTP(svc)

	body, _ := json.Marshal(radar.CreateTopicRequest{
		Name: "x", Description: "ten chars long enough",
	})
	req := httptest.NewRequest(http.MethodPost, "/radar/topics", bytes.NewReader(body))
	req = req.WithContext(userOnlyContext(req.Context(), 1, false))
	rec := httptest.NewRecorder()
	h.CreateTopicHandler()(rec, req)
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestHTTP_DisabledHandler_501(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/radar/topics", nil)
	radar.DisabledHandler(rec, req)
	require.Equal(t, http.StatusNotImplemented, rec.Code)
	require.Contains(t, rec.Body.String(), "radar_disabled")
}

func TestHTTP_AddFeed_201(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	body, _ := json.Marshal(radar.AddFeedRequest{URL: "https://news.ycombinator.com/rss"})
	req := httptest.NewRequest(http.MethodPost, "/radar/feeds", bytes.NewReader(body))
	req = req.WithContext(userOnlyContext(req.Context(), 1, true))
	rec := httptest.NewRecorder()
	h.AddFeedHandler()(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var got radar.Feed
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
	require.Equal(t, "rss", got.Kind)
	require.True(t, got.IsActive)
}

func TestHTTP_AddFeed_409_Duplicate(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	body, _ := json.Marshal(radar.AddFeedRequest{URL: "https://dup.example/f"})
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/radar/feeds", bytes.NewReader(body))
		req = req.WithContext(userOnlyContext(req.Context(), 1, true))
		rec := httptest.NewRecorder()
		h.AddFeedHandler()(rec, req)
		if i == 0 {
			require.Equal(t, http.StatusCreated, rec.Code)
		} else {
			require.Equal(t, http.StatusConflict, rec.Code)
		}
	}
}

func TestHTTP_Subscribe_201(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	feed, _ := svc.AddFeed(context.Background(), radar.AddFeedRequest{URL: "https://s.example/f"})

	body, _ := json.Marshal(radar.SubscribeRequest{FeedID: feed.ID})
	req := httptest.NewRequest(http.MethodPost, "/radar/subscriptions", bytes.NewReader(body))
	req = req.WithContext(userOnlyContext(req.Context(), 99, false))
	rec := httptest.NewRecorder()
	h.SubscribeHandler()(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var sub radar.Subscription
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&sub))
	require.Equal(t, int64(99), sub.UserID)
}

func TestHTTP_Subscribe_404_FeedMissing(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	body, _ := json.Marshal(radar.SubscribeRequest{FeedID: 12345})
	req := httptest.NewRequest(http.MethodPost, "/radar/subscriptions", bytes.NewReader(body))
	req = req.WithContext(userOnlyContext(req.Context(), 1, false))
	rec := httptest.NewRecorder()
	h.SubscribeHandler()(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHTTP_ListTopics_200(t *testing.T) {
	store := newMockStore()
	store.listTopicsResult = []radar.TopicWithStats{
		{Topic: radar.Topic{ID: 1, Name: "A"}},
	}
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodGet, "/radar/topics", nil)
	req = req.WithContext(userOnlyContext(req.Context(), 1, false))
	rec := httptest.NewRecorder()
	h.ListTopicsHandler()(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var body struct {
		Items []radar.TopicWithStats `json:"items"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	require.Len(t, body.Items, 1)
	require.Equal(t, "A", body.Items[0].Name)
}

func TestHTTP_GetTopic_200(t *testing.T) {
	store := newMockStore()
	store.getTopicResult = &radar.TopicWithStats{Topic: radar.Topic{ID: 7, Name: "X"}}
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodGet, "/radar/topics/7", nil)
	ctx := userOnlyContext(req.Context(), 1, false)
	ctx = withRouteID(ctx, "7")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	h.GetTopicHandler()(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func TestHTTP_GetTopic_404(t *testing.T) {
	store := newMockStore()
	store.getTopicErr = radar.ErrNotFound
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodGet, "/radar/topics/7", nil)
	req = req.WithContext(withRouteID(userOnlyContext(req.Context(), 1, false), "7"))
	rec := httptest.NewRecorder()
	h.GetTopicHandler()(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHTTP_GetTopic_400_badID(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodGet, "/radar/topics/abc", nil)
	req = req.WithContext(withRouteID(userOnlyContext(req.Context(), 1, false), "abc"))
	rec := httptest.NewRecorder()
	h.GetTopicHandler()(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHTTP_UpdateTopic_200(t *testing.T) {
	store := newMockStore()
	store.updateTopicResult = &radar.Topic{ID: 7, Name: "renamed", IsActive: true}
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	name := "renamed"
	body, _ := json.Marshal(radar.UpdateTopicRequest{Name: &name})
	req := httptest.NewRequest(http.MethodPatch, "/radar/topics/7", bytes.NewReader(body))
	req = req.WithContext(withRouteID(userOnlyContext(req.Context(), 1, false), "7"))
	rec := httptest.NewRecorder()
	h.UpdateTopicHandler()(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var got radar.Topic
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&got))
	require.Equal(t, "renamed", got.Name)
}

func TestHTTP_UpdateTopic_400_emptyPatch(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodPatch, "/radar/topics/7", strings.NewReader(`{}`))
	req = req.WithContext(withRouteID(userOnlyContext(req.Context(), 1, false), "7"))
	rec := httptest.NewRecorder()
	h.UpdateTopicHandler()(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHTTP_UpdateTopic_404(t *testing.T) {
	store := newMockStore()
	store.updateTopicErr = radar.ErrNotFound
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	name := "renamed"
	body, _ := json.Marshal(radar.UpdateTopicRequest{Name: &name})
	req := httptest.NewRequest(http.MethodPatch, "/radar/topics/7", bytes.NewReader(body))
	req = req.WithContext(withRouteID(userOnlyContext(req.Context(), 1, false), "7"))
	rec := httptest.NewRecorder()
	h.UpdateTopicHandler()(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHTTP_DeleteTopic_204(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodDelete, "/radar/topics/7", nil)
	req = req.WithContext(withRouteID(userOnlyContext(req.Context(), 1, false), "7"))
	rec := httptest.NewRecorder()
	h.DeleteTopicHandler()(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)
}

func TestHTTP_DeleteTopic_404(t *testing.T) {
	store := newMockStore()
	store.deleteTopicErr = radar.ErrNotFound
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodDelete, "/radar/topics/7", nil)
	req = req.WithContext(withRouteID(userOnlyContext(req.Context(), 1, false), "7"))
	rec := httptest.NewRecorder()
	h.DeleteTopicHandler()(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHTTP_ListMatches_200_filters(t *testing.T) {
	store := newMockStore()
	store.listMatchesResult = []radar.MatchView{{ID: 1, TopicID: 7, State: "new"}}
	store.listMatchesTotal = 1
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodGet, "/radar/matches?topic_id=7&state=new&limit=10", nil)
	req = req.WithContext(userOnlyContext(req.Context(), 1, false))
	rec := httptest.NewRecorder()
	h.ListMatchesHandler()(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	require.NotNil(t, store.listMatchesParams.TopicID)
	require.Equal(t, int64(7), *store.listMatchesParams.TopicID)
	require.NotNil(t, store.listMatchesParams.State)
	require.Equal(t, "new", *store.listMatchesParams.State)
	require.Equal(t, 10, store.listMatchesParams.Limit)
}

func TestHTTP_ListMatches_200_noFilters(t *testing.T) {
	store := newMockStore()
	store.listMatchesResult = []radar.MatchView{}
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodGet, "/radar/matches", nil)
	req = req.WithContext(userOnlyContext(req.Context(), 1, false))
	rec := httptest.NewRecorder()
	h.ListMatchesHandler()(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	require.Nil(t, store.listMatchesParams.TopicID)
	require.Nil(t, store.listMatchesParams.State)
	require.Equal(t, 50, store.listMatchesParams.Limit) // service default
}

func TestHTTP_ListMatches_400_badTopicID(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodGet, "/radar/matches?topic_id=abc", nil)
	req = req.WithContext(userOnlyContext(req.Context(), 1, false))
	rec := httptest.NewRecorder()
	h.ListMatchesHandler()(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHTTP_UpdateMatch_200(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	body, _ := json.Marshal(radar.UpdateMatchRequest{State: "seen"})
	req := httptest.NewRequest(http.MethodPatch, "/radar/matches/42", bytes.NewReader(body))
	req = req.WithContext(withRouteID(userOnlyContext(req.Context(), 1, false), "42"))
	rec := httptest.NewRecorder()
	h.UpdateMatchHandler()(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Equal(t, "seen", store.updateMatchState)
}

func TestHTTP_UpdateMatch_400_badEnum(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	body, _ := json.Marshal(radar.UpdateMatchRequest{State: "archived"})
	req := httptest.NewRequest(http.MethodPatch, "/radar/matches/42", bytes.NewReader(body))
	req = req.WithContext(withRouteID(userOnlyContext(req.Context(), 1, false), "42"))
	rec := httptest.NewRecorder()
	h.UpdateMatchHandler()(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHTTP_UpdateMatch_404(t *testing.T) {
	store := newMockStore()
	store.updateMatchErr = radar.ErrNotFound
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	body, _ := json.Marshal(radar.UpdateMatchRequest{State: "seen"})
	req := httptest.NewRequest(http.MethodPatch, "/radar/matches/42", bytes.NewReader(body))
	req = req.WithContext(withRouteID(userOnlyContext(req.Context(), 1, false), "42"))
	rec := httptest.NewRecorder()
	h.UpdateMatchHandler()(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHTTP_GetMatch_ok(t *testing.T) {
	store := newMockStore()
	store.getMatchResult = &radar.MatchView{
		ID: 42, TopicID: 7, TopicName: "T",
		Similarity: 0.7, State: "new", MatchedAt: time.Now(),
	}
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodGet, "/radar/matches/42", nil)
	req = req.WithContext(withRouteID(userOnlyContext(req.Context(), 11, false), "42"))
	rec := httptest.NewRecorder()
	h.GetMatchHandler()(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), `"id":42`)
}

func TestHTTP_GetMatch_badID(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodGet, "/radar/matches/abc", nil)
	req = req.WithContext(withRouteID(userOnlyContext(req.Context(), 11, false), "abc"))
	rec := httptest.NewRecorder()
	h.GetMatchHandler()(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHTTP_GetMatch_notFound(t *testing.T) {
	store := newMockStore()
	store.getMatchErr = radar.ErrNotFound
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodGet, "/radar/matches/42", nil)
	req = req.WithContext(withRouteID(userOnlyContext(req.Context(), 11, false), "42"))
	rec := httptest.NewRecorder()
	h.GetMatchHandler()(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHTTP_Status_200_withLastSweep(t *testing.T) {
	store := newMockStore()
	when := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	store.lastSweepResult = &when
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodGet, "/radar/status", nil)
	req = req.WithContext(userOnlyContext(req.Context(), 1, false))
	rec := httptest.NewRecorder()
	h.StatusHandler()(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var body radar.RadarStatus
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	require.NotNil(t, body.LastSweepAt)
}

func TestHTTP_Status_200_null(t *testing.T) {
	store := newMockStore()
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodGet, "/radar/status", nil)
	req = req.WithContext(userOnlyContext(req.Context(), 1, false))
	rec := httptest.NewRecorder()
	h.StatusHandler()(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var body radar.RadarStatus
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	require.Nil(t, body.LastSweepAt)
}

func TestHTTP_ListFeeds_200(t *testing.T) {
	store := newMockStore()
	store.listFeedsResult = []radar.Feed{{ID: 1, URL: "https://x.example/rss"}}
	store.listFeedsTotal = 1
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	req := httptest.NewRequest(http.MethodGet, "/radar/feeds?limit=10", nil)
	req = req.WithContext(userOnlyContext(req.Context(), 1, true))
	rec := httptest.NewRecorder()
	h.ListFeedsHandler()(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}
