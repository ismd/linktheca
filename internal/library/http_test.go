package library_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	coreauth "github.com/ismd/linktheca/internal/core/auth"
	"github.com/ismd/linktheca/internal/library"
	"github.com/stretchr/testify/require"
)

func setupHTTPTest(t *testing.T) (*chi.Mux, *coreauth.JWTIssuer) {
	t.Helper()

	store := newMockStore()
	ext := newMockExtractor()
	fetch := newMockFetcher()
	svc := library.NewService(store, ext, fetch)

	issuer := coreauth.NewJWTIssuer("test-secret-at-least-32-bytes-long-for-hmac", 15*time.Minute)
	h := library.NewHTTP(svc)

	r := chi.NewRouter()
	r.Route("/library", func(r chi.Router) {
		r.Use(coreauth.RequireUser(issuer))
		r.Post("/", h.SaveHandler())
		r.Get("/", h.ListHandler())
		r.Get("/{id}", h.GetHandler())
		r.Patch("/{id}", h.UpdateHandler())
		r.Delete("/{id}", h.DeleteHandler())
	})

	return r, issuer
}

func authHeader(t *testing.T, issuer *coreauth.JWTIssuer, userID int64) string {
	t.Helper()
	token, err := issuer.Issue(userID, false)
	require.NoError(t, err)
	return "Bearer " + token
}

func TestHTTPSaveAndGet(t *testing.T) {
	r, issuer := setupHTTPTest(t)

	body := `{"url":"https://example.com/http-test"}`
	req := httptest.NewRequest(http.MethodPost, "/library", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader(t, issuer, 1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)

	var item library.Item
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&item))
	require.Equal(t, "https://example.com/http-test", item.URL)
	require.Equal(t, "unread", item.State)

	// GET the created item
	req2 := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/library/%d", item.ID), nil)
	req2.Header.Set("Authorization", authHeader(t, issuer, 1))
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)

	require.Equal(t, http.StatusOK, rec2.Code)
}

func TestHTTPList(t *testing.T) {
	r, issuer := setupHTTPTest(t)

	// Save 2 items
	for _, url := range []string{"https://a.com", "https://b.com"} {
		body, _ := json.Marshal(library.SaveRequest{URL: url})
		req := httptest.NewRequest(http.MethodPost, "/library", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authHeader(t, issuer, 1))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		require.Equal(t, http.StatusCreated, rec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/library?limit=10", nil)
	req.Header.Set("Authorization", authHeader(t, issuer, 1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var result library.ListResult
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&result))
	require.Equal(t, 2, result.Total)
	require.Len(t, result.Items, 2)
}

func TestHTTPUpdate(t *testing.T) {
	r, issuer := setupHTTPTest(t)

	// Save
	body := `{"url":"https://example.com/update-http"}`
	req := httptest.NewRequest(http.MethodPost, "/library", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader(t, issuer, 1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var item library.Item
	json.NewDecoder(rec.Body).Decode(&item)

	// Update
	updateBody := `{"state":"read","is_favorite":true}`
	req2 := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/library/%d", item.ID), bytes.NewBufferString(updateBody))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", authHeader(t, issuer, 1))
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)

	require.Equal(t, http.StatusOK, rec2.Code)

	var updated library.Item
	json.NewDecoder(rec2.Body).Decode(&updated)
	require.Equal(t, "read", updated.State)
	require.True(t, updated.IsFavorite)
}

func TestHTTPDelete(t *testing.T) {
	r, issuer := setupHTTPTest(t)

	body := `{"url":"https://example.com/delete-http"}`
	req := httptest.NewRequest(http.MethodPost, "/library", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader(t, issuer, 1))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	var item library.Item
	json.NewDecoder(rec.Body).Decode(&item)

	// Delete
	req2 := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/library/%d", item.ID), nil)
	req2.Header.Set("Authorization", authHeader(t, issuer, 1))
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)

	require.Equal(t, http.StatusNoContent, rec2.Code)
}

func TestHTTPSaveNoAuth(t *testing.T) {
	r, _ := setupHTTPTest(t)

	body := `{"url":"https://example.com/no-auth"}`
	req := httptest.NewRequest(http.MethodPost, "/library", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestHTTPSaveDuplicate(t *testing.T) {
	r, issuer := setupHTTPTest(t)

	body := `{"url":"https://example.com/dup-http"}`
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodPost, "/library", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", authHeader(t, issuer, 1))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if i == 0 {
			require.Equal(t, http.StatusCreated, rec.Code)
		} else {
			require.Equal(t, http.StatusConflict, rec.Code)
		}
	}
}
