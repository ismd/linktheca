package radar_test

import (
	"bytes"
	"context"
	"encoding/json"
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
