package radar_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	coreauth "github.com/ismd/linktheca/internal/core/auth"
	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/ismd/linktheca/internal/radar"
	"github.com/ismd/linktheca/internal/testing/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func seedRadarUser(t *testing.T, pool *pgxpool.Pool, isAdmin bool) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email, password_hash, display_name, is_admin)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		"u+"+t.Name()+"@example.com", "x", "Tester", isAdmin).Scan(&id)
	require.NoError(t, err)
	return id
}

func TestIntegrationRadarFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := radar.NewStore(pool)
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)
	h := radar.NewHTTP(svc)

	userID := seedRadarUser(t, pool, true)
	issuer := coreauth.NewJWTIssuer("test-secret-at-least-32-bytes-long-for-hmac", 15*time.Minute)
	token, err := issuer.Issue(userID, true)
	require.NoError(t, err)
	auth := "Bearer " + token

	r := chi.NewRouter()
	r.Route("/radar", func(r chi.Router) {
		r.Use(coreauth.RequireUser(issuer))
		r.Post("/topics", h.CreateTopicHandler())
		r.Post("/subscriptions", h.SubscribeHandler())
		r.Group(func(r chi.Router) {
			r.Use(coreauth.RequireAdmin)
			r.Post("/feeds", h.AddFeedHandler())
		})
	})

	// 1. Add a feed (admin).
	feedBody, _ := json.Marshal(radar.AddFeedRequest{URL: "https://hn.example/rss"})
	req := httptest.NewRequest(http.MethodPost, "/radar/feeds", bytes.NewReader(feedBody))
	req.Header.Set("Authorization", auth)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var feed radar.Feed
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&feed))

	// 2. Create a topic (any user).
	topicBody, _ := json.Marshal(radar.CreateTopicRequest{
		Name: "ML", Description: "machine learning and embeddings",
	})
	req = httptest.NewRequest(http.MethodPost, "/radar/topics", bytes.NewReader(topicBody))
	req.Header.Set("Authorization", auth)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var topic radar.Topic
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&topic))
	require.True(t, topic.HasEmbedding)

	// 3. Subscribe.
	subBody, _ := json.Marshal(radar.SubscribeRequest{FeedID: feed.ID})
	req = httptest.NewRequest(http.MethodPost, "/radar/subscriptions", bytes.NewReader(subBody))
	req.Header.Set("Authorization", auth)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	// 4. Verify rows in DB.
	var nFeed, nTopic, nSub int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM radar_feeds WHERE id=$1`, feed.ID).Scan(&nFeed))
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM radar_topics WHERE id=$1 AND embedding IS NOT NULL`,
		topic.ID).Scan(&nTopic))
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM radar_feed_subscriptions WHERE user_id=$1 AND feed_id=$2`,
		userID, feed.ID).Scan(&nSub))
	require.Equal(t, 1, nFeed)
	require.Equal(t, 1, nTopic)
	require.Equal(t, 1, nSub)
}

func TestIntegrationAddFeedRequiresAdmin(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := radar.NewStore(pool)
	svc := radar.NewService(store, &embeddings.FakeEmbedder{Dim: 1024})
	h := radar.NewHTTP(svc)

	userID := seedRadarUser(t, pool, false) // not admin
	issuer := coreauth.NewJWTIssuer("test-secret-at-least-32-bytes-long-for-hmac", 15*time.Minute)
	token, _ := issuer.Issue(userID, false)

	r := chi.NewRouter()
	r.Route("/radar", func(r chi.Router) {
		r.Use(coreauth.RequireUser(issuer))
		r.Group(func(r chi.Router) {
			r.Use(coreauth.RequireAdmin)
			r.Post("/feeds", h.AddFeedHandler())
		})
	})

	body, _ := json.Marshal(radar.AddFeedRequest{URL: "https://x.example/f"})
	req := httptest.NewRequest(http.MethodPost, "/radar/feeds", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestIntegrationRadarReadAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)
	store := radar.NewStore(pool)
	emb := &embeddings.FakeEmbedder{Dim: 1024}
	svc := radar.NewService(store, emb)
	h := radar.NewHTTP(svc)

	userID := seedRadarUser(t, pool, true)
	issuer := coreauth.NewJWTIssuer("test-secret-at-least-32-bytes-long-for-hmac", 15*time.Minute)
	token, err := issuer.Issue(userID, true)
	require.NoError(t, err)
	auth := "Bearer " + token

	r := chi.NewRouter()
	r.Route("/radar", func(r chi.Router) {
		r.Use(coreauth.RequireUser(issuer))
		r.Post("/topics", h.CreateTopicHandler())
		r.Get("/topics", h.ListTopicsHandler())
		r.Get("/topics/{id}", h.GetTopicHandler())
		r.Patch("/topics/{id}", h.UpdateTopicHandler())
		r.Delete("/topics/{id}", h.DeleteTopicHandler())
		r.Post("/subscriptions", h.SubscribeHandler())
		r.Get("/matches", h.ListMatchesHandler())
		r.Patch("/matches/{id}", h.UpdateMatchHandler())
		r.Get("/status", h.StatusHandler())
		r.Group(func(r chi.Router) {
			r.Use(coreauth.RequireAdmin)
			r.Post("/feeds", h.AddFeedHandler())
			r.Get("/feeds", h.ListFeedsHandler())
		})
	})

	doJSON := func(method, path string, payload any) (*httptest.ResponseRecorder, []byte) {
		t.Helper()
		var body []byte
		if payload != nil {
			body, _ = json.Marshal(payload)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Authorization", auth)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec, rec.Body.Bytes()
	}

	// 1. Admin: add feed.
	rec, _ := doJSON(http.MethodPost, "/radar/feeds",
		radar.AddFeedRequest{URL: "https://news.example/rss"})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var feed radar.Feed
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &feed))

	// 2. Create topic + subscribe.
	rec, _ = doJSON(http.MethodPost, "/radar/topics",
		radar.CreateTopicRequest{Name: "ML", Description: "machine learning research and products"})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var topic radar.Topic
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &topic))

	rec, _ = doJSON(http.MethodPost, "/radar/subscriptions",
		radar.SubscribeRequest{FeedID: feed.ID})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	// 3. Seed a finding + match directly (bypass crawler).
	var findingID int64
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO radar_findings (feed_id, url, title) VALUES ($1, $2, $3) RETURNING id`,
		feed.ID, "https://news.example/a", "title-a").Scan(&findingID))
	var matchID int64
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO radar_topic_matches (topic_id, finding_id, similarity, state)
		 VALUES ($1, $2, $3, 'new') RETURNING id`,
		topic.ID, findingID, 0.7).Scan(&matchID))

	// Also stamp last_fetched_at so /status returns it.
	_, err = pool.Exec(context.Background(),
		`UPDATE radar_feeds SET last_fetched_at = now() WHERE id = $1`, feed.ID)
	require.NoError(t, err)

	// 4. GET /radar/topics returns aggregate stats.
	rec, _ = doJSON(http.MethodGet, "/radar/topics", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var topicsResp struct {
		Items []radar.TopicWithStats `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &topicsResp))
	require.Len(t, topicsResp.Items, 1)
	require.Equal(t, 1, topicsResp.Items[0].Stats.NewCount)
	require.Equal(t, 1, topicsResp.Items[0].Stats.TotalCount)
	require.Equal(t, 1, topicsResp.Items[0].Stats.SourceCount)

	// 5. GET /radar/matches?state=new
	rec, _ = doJSON(http.MethodGet, "/radar/matches?state=new", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var matchesResp radar.MatchList
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &matchesResp))
	require.Len(t, matchesResp.Items, 1)
	require.Equal(t, "ML", matchesResp.Items[0].TopicName)
	require.Equal(t, "https://news.example/a", matchesResp.Items[0].Finding.URL)

	// 6. PATCH match state → seen.
	rec, _ = doJSON(http.MethodPatch,
		fmt.Sprintf("/radar/matches/%d", matchID),
		radar.UpdateMatchRequest{State: "seen"})
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())

	// 7. GET /radar/topics shows new_count=0 now.
	rec, _ = doJSON(http.MethodGet, "/radar/topics", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &topicsResp))
	require.Equal(t, 0, topicsResp.Items[0].Stats.NewCount)
	require.Equal(t, 1, topicsResp.Items[0].Stats.TotalCount)

	// 8. GET /radar/status returns last_sweep_at.
	rec, _ = doJSON(http.MethodGet, "/radar/status", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var status radar.RadarStatus
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &status))
	require.NotNil(t, status.LastSweepAt)

	// 9. PATCH topic (rename).
	newName := "Machine Learning"
	rec, _ = doJSON(http.MethodPatch,
		fmt.Sprintf("/radar/topics/%d", topic.ID),
		radar.UpdateTopicRequest{Name: &newName})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// 10. DELETE topic; subsequent GET /radar/matches is empty (CASCADE).
	rec, _ = doJSON(http.MethodDelete,
		fmt.Sprintf("/radar/topics/%d", topic.ID), nil)
	require.Equal(t, http.StatusNoContent, rec.Code)

	rec, _ = doJSON(http.MethodGet, "/radar/matches", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &matchesResp))
	require.Empty(t, matchesResp.Items)
}
