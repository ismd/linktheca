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

	"github.com/ismd/linktheca/internal/core/embeddings"
	"github.com/ismd/linktheca/internal/radar"
	"github.com/stretchr/testify/require"
)

// userOnlyContext attaches a user_id to ctx without parsing a JWT.
func userOnlyContext(ctx context.Context, userID int64, isAdmin bool) context.Context {
	return coreauthWithUser(ctx, userID, isAdmin)
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
