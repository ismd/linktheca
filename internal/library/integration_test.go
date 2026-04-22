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
	"github.com/ismd/linktheca/internal/core/content"
	"github.com/ismd/linktheca/internal/library"
	"github.com/ismd/linktheca/internal/testing/testdb"
	"github.com/stretchr/testify/require"
)

func TestIntegrationFullLibraryFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)

	// Set up a fake HTTP server serving article content
	articleSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		html := `<!DOCTYPE html><html><head><title>Integration Test Article</title></head>
		<body><article><h1>Integration Test Article</h1>
		<p>This is a long enough paragraph to be recognized by the readability algorithm.
		It contains multiple sentences and enough words to make the extraction work properly.
		The content extraction should identify this as the main article text.</p>
		<p>Second paragraph with more content to ensure reliable extraction.
		We need enough text for the readability heuristics to work correctly.</p>
		</article></body></html>`
		_, _ = w.Write([]byte(html))
	}))
	defer articleSrv.Close()

	// Create test user
	userID := createTestUser(t, pool)

	// Build the library stack with real store and real extractor
	store := library.NewStore(pool)
	extractor := content.NewExtractor()
	svc := library.NewService(store, extractor)

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

	token, err := issuer.Issue(userID, false)
	require.NoError(t, err)
	auth := "Bearer " + token

	// 1. Save a URL
	body, _ := json.Marshal(library.SaveRequest{URL: articleSrv.URL})
	req := httptest.NewRequest(http.MethodPost, "/library", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code, "save response: %s", rec.Body.String())

	var saved library.Item
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&saved))
	require.Equal(t, "unread", saved.State)
	require.NotZero(t, saved.ID)

	// 2. List items
	req = httptest.NewRequest(http.MethodGet, "/library?limit=10", nil)
	req.Header.Set("Authorization", auth)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var listResult library.ListResult
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&listResult))
	require.Equal(t, 1, listResult.Total)

	// 3. Mark as read
	updateBody := `{"state":"read"}`
	req = httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/library/%d", saved.ID), bytes.NewBufferString(updateBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var updated library.Item
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&updated))
	require.Equal(t, "read", updated.State)

	// 4. Filter by state=unread — should return 0
	req = httptest.NewRequest(http.MethodGet, "/library?state=unread&limit=10", nil)
	req.Header.Set("Authorization", auth)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var filtered library.ListResult
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&filtered))
	require.Equal(t, 0, filtered.Total)

	// 5. Delete
	req = httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/library/%d", saved.ID), nil)
	req.Header.Set("Authorization", auth)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)

	// 6. Verify deleted
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/library/%d", saved.ID), nil)
	req.Header.Set("Authorization", auth)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestIntegrationSaveDuplicateURL(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	pool := testdb.New(t)

	articleSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>Dup</title></head><body><article><p>Content for duplicate test article with enough text.</p><p>More text here.</p></article></body></html>`))
	}))
	defer articleSrv.Close()

	userID := createTestUser(t, pool)
	store := library.NewStore(pool)
	extractor := content.NewExtractor()
	svc := library.NewService(store, extractor)
	issuer := coreauth.NewJWTIssuer("test-secret-at-least-32-bytes-long-for-hmac", 15*time.Minute)
	h := library.NewHTTP(svc)

	r := chi.NewRouter()
	r.Route("/library", func(r chi.Router) {
		r.Use(coreauth.RequireUser(issuer))
		r.Post("/", h.SaveHandler())
	})

	token, _ := issuer.Issue(userID, false)
	auth := "Bearer " + token

	body, _ := json.Marshal(library.SaveRequest{URL: articleSrv.URL})

	// First save
	req := httptest.NewRequest(http.MethodPost, "/library", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	// Second save — should return 409 Conflict
	req = httptest.NewRequest(http.MethodPost, "/library", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", auth)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusConflict, rec.Code)
}
